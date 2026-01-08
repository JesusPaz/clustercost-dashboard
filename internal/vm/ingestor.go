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
		agentName = req.AgentId
	}

	meta := i.agentMeta[agentName]
	base := baseLabels(agentName, req.ClusterId, req.ClusterName, meta)

	version := strings.TrimSpace(req.Version)
	labels := appendLabels(base, label{"version", version})
	writeSample(buf, "clustercost_agent_up", labels, "1", tsMillis)

	if req.Snapshot == nil {
		return
	}

	for _, item := range req.Snapshot.Namespaces {
		nsLabels := appendLabels(base,
			label{"namespace", item.Namespace},
			label{"environment", item.Environment},
		)
		writeSample(buf, "clustercost_namespace_hourly_cost", nsLabels, formatFloat(item.HourlyCost), tsMillis)
		writeSample(buf, "clustercost_namespace_pod_count", nsLabels, formatInt(int64(item.PodCount)), tsMillis)
		writeSample(buf, "clustercost_namespace_cpu_request_milli", nsLabels, formatInt(item.CpuRequestMilli), tsMillis)
		writeSample(buf, "clustercost_namespace_cpu_usage_milli", nsLabels, formatInt(item.CpuUsageMilli), tsMillis)
		writeSample(buf, "clustercost_namespace_memory_request_bytes", nsLabels, formatInt(item.MemoryRequestBytes), tsMillis)
		writeSample(buf, "clustercost_namespace_memory_usage_bytes", nsLabels, formatInt(item.MemoryUsageBytes), tsMillis)
	}

	for _, item := range req.Snapshot.Nodes {
		nodeLabels := appendLabels(base,
			label{"node", item.NodeName},
		)
		writeSample(buf, "clustercost_node_hourly_cost", appendLabels(nodeLabels, label{"instance_type", item.InstanceType}), formatFloat(item.HourlyCost), tsMillis)
		writeSample(buf, "clustercost_node_cpu_usage_percent", nodeLabels, formatFloat(item.CpuUsagePercent), tsMillis)
		writeSample(buf, "clustercost_node_memory_usage_percent", nodeLabels, formatFloat(item.MemoryUsagePercent), tsMillis)
		writeSample(buf, "clustercost_node_cpu_allocatable_milli", nodeLabels, formatInt(item.CpuAllocatableMilli), tsMillis)
		writeSample(buf, "clustercost_node_memory_allocatable_bytes", nodeLabels, formatInt(item.MemoryAllocatableBytes), tsMillis)
		writeSample(buf, "clustercost_node_pod_count", nodeLabels, formatInt(int64(item.PodCount)), tsMillis)
		writeSample(buf, "clustercost_node_under_pressure", nodeLabels, formatBool(item.IsUnderPressure), tsMillis)
		if item.Status != "" {
			statusLabels := appendLabels(nodeLabels, label{"status", item.Status})
			writeSample(buf, "clustercost_node_status", statusLabels, "1", tsMillis)
		}
	}

	if req.Snapshot.Resources != nil {
		resLabels := base
		writeSample(buf, "clustercost_cluster_cpu_usage_milli_total", resLabels, formatInt(req.Snapshot.Resources.CpuUsageMilliTotal), tsMillis)
		writeSample(buf, "clustercost_cluster_cpu_request_milli_total", resLabels, formatInt(req.Snapshot.Resources.CpuRequestMilliTotal), tsMillis)
		writeSample(buf, "clustercost_cluster_memory_usage_bytes_total", resLabels, formatInt(req.Snapshot.Resources.MemoryUsageBytesTotal), tsMillis)
		writeSample(buf, "clustercost_cluster_memory_request_bytes_total", resLabels, formatInt(req.Snapshot.Resources.MemoryRequestBytesTotal), tsMillis)
		writeSample(buf, "clustercost_cluster_total_node_hourly_cost", resLabels, formatFloat(req.Snapshot.Resources.TotalNodeHourlyCost), tsMillis)
	}
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
	if req == nil {
		return time.Now().UnixMilli()
	}
	ts := req.TimestampSeconds
	if req.Snapshot != nil && req.Snapshot.TimestampSeconds > 0 {
		ts = req.Snapshot.TimestampSeconds
	}
	if ts <= 0 {
		return time.Now().UnixMilli()
	}
	return ts * 1000
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
