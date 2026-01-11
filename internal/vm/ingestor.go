package vm

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
)

const (
	defaultIngestPath     = "/api/v1/import/prometheus"
	defaultTimeout        = 5 * time.Second
	defaultFlushInterval  = 2 * time.Second
	defaultBatchBytes     = 2 << 20 // 2 MiB
	defaultQueueSize      = 10000
	defaultWorkerOverride = 0
)

// Ingestor batches gRPC reports into VictoriaMetrics.
type Ingestor struct {
	ingestURL     string
	authToken     string
	username      string
	password      string
	enableGzip    bool
	maxBatchBytes int
	flushInterval time.Duration
	queue         chan reportEnvelope
	client        *http.Client
	logger        *log.Logger
	agentMeta     map[string]agentMetadata
	stopped       atomic.Bool
	wg            sync.WaitGroup
}

type reportEnvelope struct {
	agentName string
	req       *agentv1.ReportRequest
}

type agentMetadata struct {
	clusterType   string
	clusterRegion string
}

// NewIngestor creates a VictoriaMetrics ingestor. If no URL is configured, it returns nil.
func NewIngestor(cfg config.Config, logger *log.Logger) (*Ingestor, error) {
	if cfg.VictoriaMetricsURL == "" {
		return nil, nil
	}

	ingestURL, err := buildIngestURL(cfg.VictoriaMetricsURL, cfg.VictoriaMetricsIngestPath)
	if err != nil {
		return nil, err
	}

	timeout := cfg.VictoriaMetricsTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	flushInterval := cfg.VictoriaMetricsFlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}

	batchBytes := cfg.VictoriaMetricsBatchBytes
	if batchBytes <= 0 {
		batchBytes = defaultBatchBytes
	}

	queueSize := cfg.VictoriaMetricsQueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}

	workers := cfg.VictoriaMetricsWorkers
	if workers <= 0 {
		if defaultWorkerOverride > 0 {
			workers = defaultWorkerOverride
		} else {
			workers = max(2, runtime.NumCPU())
		}
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	ing := &Ingestor{
		ingestURL:     ingestURL,
		authToken:     cfg.VictoriaMetricsToken,
		username:      cfg.VictoriaMetricsUsername,
		password:      cfg.VictoriaMetricsPassword,
		enableGzip:    cfg.VictoriaMetricsGzip,
		maxBatchBytes: batchBytes,
		flushInterval: flushInterval,
		queue:         make(chan reportEnvelope, queueSize),
		client:        &http.Client{Timeout: timeout, Transport: transport},
		logger:        logger,
		agentMeta:     buildAgentMeta(cfg),
	}

	for i := 0; i < workers; i++ {
		ing.wg.Add(1)
		go ing.runWorker(i)
	}

	return ing, nil
}

// Enqueue queues a report for ingestion. It drops when the queue is full or stopped.
func (i *Ingestor) Enqueue(agentName string, req *agentv1.ReportRequest) bool {
	if i == nil || req == nil || i.stopped.Load() {
		return false
	}

	select {
	case i.queue <- reportEnvelope{agentName: agentName, req: req}:
		return true
	default:
		if i.logger != nil {
			i.logger.Printf("victoria metrics queue full; dropping report for agent %s", agentName)
		}
		return false
	}
}

// Stop flushes outstanding data and stops background workers.
func (i *Ingestor) Stop() {
	if i == nil {
		return
	}
	if i.stopped.Swap(true) {
		return
	}
	close(i.queue)
	i.wg.Wait()
}

func (i *Ingestor) runWorker(id int) {
	defer i.wg.Done()
	ticker := time.NewTicker(i.flushInterval)
	defer ticker.Stop()

	var buf bytes.Buffer
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		if err := i.post(buf.Bytes()); err != nil && i.logger != nil {
			i.logger.Printf("victoria metrics ingest error: %v", err)
		}
		buf.Reset()
	}

	for {
		select {
		case env, ok := <-i.queue:
			if !ok {
				flush()
				return
			}
			i.appendReport(&buf, env)
			if buf.Len() >= i.maxBatchBytes {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (i *Ingestor) post(payload []byte) error {
	var body io.Reader = bytes.NewReader(payload)
	var gz *gzip.Writer
	req, err := http.NewRequest(http.MethodPost, i.ingestURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if i.enableGzip {
		var buf bytes.Buffer
		gz = gzip.NewWriter(&buf)
		if _, err := gz.Write(payload); err != nil {
			return fmt.Errorf("compress payload: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("finalize payload: %w", err)
		}
		body = &buf
		req.Header.Set("Content-Encoding", "gzip")
	}

	req.Body = io.NopCloser(body)
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")

	if i.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+i.authToken)
	} else if i.username != "" || i.password != "" {
		req.SetBasicAuth(i.username, i.password)
	}

	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("send payload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("victoria metrics responded with status %d", resp.StatusCode)
	}
	return nil
}

func (i *Ingestor) appendReport(buf *bytes.Buffer, env reportEnvelope) {
	req := env.req
	if req == nil {
		return
	}

	tsMillis := reportTimestampMillis(req)
	agentName := env.agentName
	if agentName == "" {
		agentName = env.req.AgentId
	}
	// Fallback
	if agentName == "" {
		agentName = "unknown"
	}

	meta := i.agentMeta[agentName]
	base := baseLabels(agentName, env.req.ClusterId, env.req.ClusterName, meta)

	// Report agent up
	writeSample(buf, "clustercost_agent_up", base, "1", tsMillis)

	if req.Snapshot == nil {
		return
	}

	// 2. Process Pods & Aggregate Namespace Data
	type nsAgg struct {
		hourlyCost         float64
		podCount           int64
		cpuRequestMilli    int64
		cpuUsageMilli      int64
		memoryRequestBytes int64
		memoryUsageBytes   int64
	}
	// map[namespace]map[environment]*nsAgg
	nsMap := make(map[string]map[string]*nsAgg)

	for _, pod := range req.Snapshot.Pods {
		if pod == nil {
			continue
		}

		environment := pod.Labels["environment"]
		if environment == "" {
			environment = "unknown"
		}
		// v2 PodMetric has simple labels map.

		nodeName := req.NodeName

		podLabels := appendLabels(base,
			label{"namespace", pod.Namespace},
			label{"pod", pod.Pod},
			label{"node", nodeName},
			label{"environment", environment},
		)
		// Owner references are not in v2 top-level, assumingly maybe in labels or missing.
		// We skip them for now.

		// Add custom labels
		for k, v := range pod.Labels {
			if k == "app" || k == "component" || k == "service" {
				podLabels = append(podLabels, label{k, v})
			}
		}

		// Calculate Totals and Costs
		// CPU
		cpuSeconds := float64(0)
		if pod.CpuMetrics != nil {
			// v2: usage_user_ns + usage_kernel_ns
			ns := pod.CpuMetrics.UsageUserNs + pod.CpuMetrics.UsageKernelNs
			cpuSeconds = float64(ns) / 1e9
		}

		// Memory
		memBytes := int64(0)
		if pod.MemoryMetrics != nil {
			memBytes = int64(pod.MemoryMetrics.RssBytes)
		}

		// Network
		netTx := int64(0)
		netRx := int64(0)
		egressPublic := int64(0)
		if pod.NetworkMetrics != nil {
			netTx = int64(pod.NetworkMetrics.BytesSent)
			netRx = int64(pod.NetworkMetrics.BytesRecv)
			egressPublic = int64(pod.NetworkMetrics.EgressPublicBytes)
		}

		writeSample(buf, "clustercost_pod_cpu_usage_seconds_total", podLabels, formatFloat(cpuSeconds), tsMillis)
		writeSample(buf, "clustercost_pod_memory_rss_bytes", podLabels, formatInt(memBytes), tsMillis)
		writeSample(buf, "clustercost_pod_network_tx_bytes_total", podLabels, formatInt(netTx), tsMillis)
		writeSample(buf, "clustercost_pod_network_rx_bytes_total", podLabels, formatInt(netRx), tsMillis)
		writeSample(buf, "clustercost_pod_network_egress_public_bytes_total", podLabels, formatInt(egressPublic), tsMillis)

		// Aggregate for Namespace
		// We can still aggregate usage for namespace if we want, but "Hourly Cost" is hard without rate.
		// We'll skip aggregated cost for now and focus on raw metrics.
		// Or we can just sum the raw counters for namespace.
		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = make(map[string]*nsAgg)
		}
		if nsMap[pod.Namespace][environment] == nil {
			nsMap[pod.Namespace][environment] = &nsAgg{}
		}
		agg := nsMap[pod.Namespace][environment]
		agg.podCount++
		// agg.cpuUsageMilli += ??? We have seconds total.
	}

	// 3. Emit Aggregated Namespace Metrics
	for ns, envs := range nsMap {
		for env, agg := range envs {
			nsLabels := appendLabels(base,
				label{"namespace", ns},
				label{"environment", env},
			)
			writeSample(buf, "clustercost_namespace_pod_count", nsLabels, formatInt(agg.podCount), tsMillis)
		}
	}

	// 4. Resources Snapshot - Removed in V2.

}

func buildIngestURL(baseURL, ingestPath string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid victoria metrics url: %w", err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("victoria metrics url missing scheme: %s", baseURL)
	}
	if ingestPath == "" {
		ingestPath = defaultIngestPath
	}
	parsed.Path = path.Join(parsed.Path, ingestPath)
	return parsed.String(), nil
}

func buildAgentMeta(cfg config.Config) map[string]agentMetadata {
	out := make(map[string]agentMetadata, len(cfg.Agents))
	for _, agent := range cfg.Agents {
		out[agent.Name] = agentMetadata{
			clusterType:   agent.Type,
			clusterRegion: agent.Region,
		}
	}
	return out
}

type label struct {
	key   string
	value string
}

func baseLabels(agentName, clusterID, clusterName string, meta agentMetadata) []label {
	labels := make([]label, 0, 5)
	if clusterID != "" {
		labels = append(labels, label{"cluster_id", clusterID})
	}
	if clusterName != "" {
		labels = append(labels, label{"cluster_name", clusterName})
	}
	if meta.clusterType != "" {
		labels = append(labels, label{"cluster_type", meta.clusterType})
	}
	if meta.clusterRegion != "" {
		labels = append(labels, label{"cluster_region", meta.clusterRegion})
	}
	if agentName != "" {
		labels = append(labels, label{"agent_id", agentName})
	}
	return labels
}

func appendLabels(base []label, extra ...label) []label {
	if len(extra) == 0 {
		return base
	}
	labels := make([]label, 0, len(base)+len(extra))
	labels = append(labels, base...)
	for _, item := range extra {
		if strings.TrimSpace(item.value) == "" {
			continue
		}
		labels = append(labels, item)
	}
	return labels
}

func writeSample(buf *bytes.Buffer, name string, labels []label, value string, tsMillis int64) {
	if name == "" || value == "" {
		return
	}
	buf.WriteString(name)
	if len(labels) > 0 {
		buf.WriteByte('{')
		for idx, item := range labels {
			if idx > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(item.key)
			buf.WriteString(`="`)
			buf.WriteString(escapeLabelValue(item.value))
			buf.WriteByte('"')
		}
		buf.WriteByte('}')
	}
	buf.WriteByte(' ')
	buf.WriteString(value)
	buf.WriteByte(' ')
	buf.WriteString(strconv.FormatInt(tsMillis, 10))
	buf.WriteByte('\n')
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func formatBool(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func reportTimestampMillis(req *agentv1.ReportRequest) int64 {
	// ReportRequest v2 does not have a timestamp field.
	// We use ingestion time.
	return time.Now().UnixMilli()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
