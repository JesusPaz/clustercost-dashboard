package vm

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
	"github.com/clustercost/clustercost-dashboard/internal/store"
)

func safeInt64(u uint64) int64 {
	if u > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(u)
}

const (
	defaultIngestPath     = "/api/v1/import/prometheus"
	defaultTimeout        = 5 * time.Second
	defaultFlushInterval  = 2 * time.Second
	defaultBatchBytes     = 2 << 20 // 2 MiB
	defaultQueueSize      = 10000
	defaultWorkerOverride = 0
)

var labelReplacer = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)

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
	logLevel      string
	gzipPool      sync.Pool
}

type reportEnvelope struct {
	agentName  string
	metricsReq *agentv1.MetricsReportRequest
	networkReq *agentv1.NetworkReportRequest
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
		logLevel:      cfg.LogLevel,
		gzipPool: sync.Pool{
			New: func() interface{} {
				return gzip.NewWriter(io.Discard)
			},
		},
	}

	for i := 0; i < workers; i++ {
		ing.wg.Add(1)
		go ing.runWorker(i)
	}

	return ing, nil
}

// EnqueueMetrics queues a metrics report for ingestion.
func (i *Ingestor) EnqueueMetrics(agentName string, req *agentv1.MetricsReportRequest) bool {
	if i == nil || req == nil || i.stopped.Load() {
		return false
	}

	select {
	case i.queue <- reportEnvelope{agentName: agentName, metricsReq: req}:
		return true
	default:
		if i.logger != nil {
			i.logger.Printf("victoria metrics queue full; dropping metrics report for agent %s", agentName)
		}
		return false
	}
}

// EnqueueNetwork queues a network report for ingestion.
func (i *Ingestor) EnqueueNetwork(agentName string, req *agentv1.NetworkReportRequest) bool {
	if i == nil || req == nil || i.stopped.Load() {
		return false
	}

	select {
	case i.queue <- reportEnvelope{agentName: agentName, networkReq: req}:
		return true
	default:
		if i.logger != nil {
			i.logger.Printf("victoria metrics queue full; dropping network report for agent %s", agentName)
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
	var labelBuf bytes.Buffer   // Reusable buffer for label caching
	scratch := make([]byte, 64) // Reusable scratch for number formatting

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
			i.appendReport(&buf, &labelBuf, scratch, env)
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
	req, err := http.NewRequest(http.MethodPost, i.ingestURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if i.enableGzip {
		var buf bytes.Buffer
		gz := i.gzipPool.Get().(*gzip.Writer)
		gz.Reset(&buf)

		if _, err := gz.Write(payload); err != nil {
			return fmt.Errorf("compress payload: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("finalize payload: %w", err)
		}

		i.gzipPool.Put(gz) // Return to pool
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("victoria metrics responded with status %d", resp.StatusCode)
	}
	return nil
}

func (i *Ingestor) appendReport(buf, labelBuf *bytes.Buffer, scratch []byte, env reportEnvelope) {
	if env.metricsReq != nil {
		i.appendMetricsReport(buf, labelBuf, scratch, env.agentName, env.metricsReq)
	} else if env.networkReq != nil {
		i.appendNetworkReport(buf, labelBuf, scratch, env.agentName, env.networkReq)
	}
}

func (i *Ingestor) appendMetricsReport(buf, labelBuf *bytes.Buffer, scratch []byte, agentName string, req *agentv1.MetricsReportRequest) {
	if req == nil {
		return
	}

	tsMillis := reportTimestampMillis(req.TimestampSeconds)
	if agentName == "" {
		agentName = req.AgentId
	}
	// Fallback
	if agentName == "" {
		agentName = "unknown"
	}

	meta := i.agentMeta[agentName]
	base := baseLabels(agentName, req.ClusterId, "", meta) // ClusterName is not in V2 proto?
	// Wait, user proto: cluster_id = 2. No cluster_name.
	// We'll leave clusterName empty or rely on agentMeta?
	// The proto provided has: agent_id, cluster_id, node_name, availability_zone, timestamp_seconds, pods.
	// No cluster_name.

	// Report agent up
	labelBuf.Reset()
	writeLabels(labelBuf, base)
	baseLabelsBlob := labelBuf.Bytes() // Safe to use until labelBuf reset

	writeFlagSample(buf, scratch, "clustercost_agent_up", baseLabelsBlob, 1, tsMillis)

	// 2. Process Pods & Aggregate Namespace Data
	type nsAgg struct {
		hourlyCost     float64
		podCount       int64
		cpuUsageMilli  int64
		memoryRssBytes int64
		cpuReqMilli    int64
		memReqBytes    int64
		netTxBytes     int64
		netRxBytes     int64
		egressPublic   int64
		egressCrossAZ  int64
		egressInternal int64
	}
	// map[namespace]*nsAgg
	nsMap := make(map[string]*nsAgg)
	pricing := store.NewPricingCatalog(nil)
	region := req.Region
	if region == "" {
		region = req.AvailabilityZone
	}
	if region == "" {
		region = "us-east-1"
	}
	instanceType := req.InstanceType
	if instanceType == "" {
		instanceType = "default"
	}
	vcpus := safeInt64(0)
	ramBytes := safeInt64(0)
	if req.NodeName != "" {
		for _, node := range req.Nodes {
			if node == nil || node.NodeName != req.NodeName {
				continue
			}
			if node.CapacityCpuMillicores > 0 {
				vcpus = safeInt64(node.CapacityCpuMillicores / 1000)
			} else if node.AllocatableCpuMillicores > 0 {
				vcpus = safeInt64(node.AllocatableCpuMillicores / 1000)
			}
			if node.CapacityMemoryBytes > 0 {
				ramBytes = safeInt64(node.CapacityMemoryBytes)
			} else if node.AllocatableMemoryBytes > 0 {
				ramBytes = safeInt64(node.AllocatableMemoryBytes)
			}
			break
		}
	}
	cpuPrice, memPrice := pricing.GetNodeResourcePrices(context.Background(), region, instanceType, vcpus, ramBytes)

	for _, pod := range req.Pods {
		if pod == nil {
			continue
		}

		// Labels are removed in V2 proto provided by user.
		// We can't determine environment or owners from labels anymore.
		// We will set environment to "unknown" or "production" default?
		environment := "production" // Default assumption without labels

		nodeName := req.NodeName

		region := req.Region
		if region == "" {
			region = req.AvailabilityZone
		}

		// Calculate Totals and Costs
		// CPU
		cpuUsageMilli := safeInt64(0)
		cpuReq := safeInt64(0)
		cpuLim := safeInt64(0)
		if pod.Cpu != nil {
			cpuUsageMilli = safeInt64(pod.Cpu.UsageMillicores)
			if i.logLevel == "debug" {
				if i.logger != nil && cpuUsageMilli == 0 {
					i.logger.Printf("[DEBUG-CPU] Pod %s/%s has 0 CPU usage. Raw Proto: %+v", pod.Namespace, pod.PodName, pod.Cpu)
				} else if i.logger != nil {
					i.logger.Printf("[DEBUG-CPU] Pod %s/%s CPU Usage: %d", pod.Namespace, pod.PodName, cpuUsageMilli)
				}
			}
			cpuReq = safeInt64(pod.Cpu.RequestMillicores)
			cpuLim = safeInt64(pod.Cpu.LimitMillicores)
		}

		// Memory
		memBytes := safeInt64(0)
		memReq := safeInt64(0)
		memLim := safeInt64(0)
		if pod.Memory != nil {
			memBytes = safeInt64(pod.Memory.RssBytes)
			memReq = safeInt64(pod.Memory.RequestBytes)
			memLim = safeInt64(pod.Memory.LimitBytes)
		}

		// Network
		netTx := safeInt64(0)
		netRx := safeInt64(0)
		egressPublic := safeInt64(0)
		egressCrossAZ := safeInt64(0)
		egressInternal := safeInt64(0)
		if pod.Network != nil {
			netTx = safeInt64(pod.Network.BytesSent)
			netRx = safeInt64(pod.Network.BytesReceived)
			egressPublic = safeInt64(pod.Network.EgressPublicBytes)
			egressCrossAZ = safeInt64(pod.Network.EgressCrossAzBytes)
			egressInternal = safeInt64(pod.Network.EgressInternalBytes)
		}

		// Prepare cached labels for this pod
		labelBuf.Reset()
		writeLabels(labelBuf, base,
			label{"namespace", pod.Namespace},
			label{"pod", pod.PodName},
			label{"node", nodeName},
			label{"availability_zone", req.AvailabilityZone},
			label{"region", region},
			label{"instance_type", req.InstanceType},
			label{"environment", environment},
		)
		podLabelsBlob := labelBuf.Bytes()

		writeIntSample(buf, scratch, "clustercost_pod_cpu_usage_milli", podLabelsBlob, cpuUsageMilli, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_cpu_request_millicores", podLabelsBlob, cpuReq, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_cpu_limit_millicores", podLabelsBlob, cpuLim, tsMillis)

		writeIntSample(buf, scratch, "clustercost_pod_memory_rss_bytes", podLabelsBlob, memBytes, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_memory_request_bytes", podLabelsBlob, memReq, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_memory_limit_bytes", podLabelsBlob, memLim, tsMillis)

		writeIntSample(buf, scratch, "clustercost_pod_network_tx_bytes_total", podLabelsBlob, netTx, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_network_rx_bytes_total", podLabelsBlob, netRx, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_network_egress_public_bytes_total", podLabelsBlob, egressPublic, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_network_egress_cross_az_bytes_total", podLabelsBlob, egressCrossAZ, tsMillis)
		writeIntSample(buf, scratch, "clustercost_pod_network_egress_internal_bytes_total", podLabelsBlob, egressInternal, tsMillis)

		cpuReqCores := float64(cpuReq) / 1000.0
		memReqGB := float64(memReq) / (1024 * 1024 * 1024)
		hourlyCost := (cpuReqCores * cpuPrice) + (memReqGB * memPrice)

		writeFloatSample(buf, scratch, "clustercost_pod_hourly_cost", podLabelsBlob, hourlyCost, tsMillis)

		// Aggregate for Namespace
		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &nsAgg{}
		}
		agg := nsMap[pod.Namespace]
		agg.podCount++
		agg.cpuUsageMilli += cpuUsageMilli
		agg.memoryRssBytes += memBytes
		agg.hourlyCost += hourlyCost
		agg.cpuReqMilli += cpuReq
		agg.netTxBytes += netTx
		agg.netRxBytes += netRx
		agg.egressPublic += egressPublic
		agg.egressCrossAZ += egressCrossAZ
		agg.egressInternal += egressInternal
	}

	// 3. Emit Aggregated Namespace Metrics & Calculate Cluster Totals
	clusterTx := safeInt64(0)
	clusterRx := safeInt64(0)
	clusterEgressPublic := safeInt64(0)
	clusterEgressCrossAZ := safeInt64(0)
	clusterEgressInternal := safeInt64(0)
	for ns, agg := range nsMap {
		clusterTx += agg.netTxBytes
		clusterRx += agg.netRxBytes
		clusterEgressPublic += agg.egressPublic
		clusterEgressCrossAZ += agg.egressCrossAZ
		clusterEgressInternal += agg.egressInternal

		labelBuf.Reset()
		writeLabels(labelBuf, base,
			label{"namespace", ns},
			label{"environment", "production"},
		)
		nsLabelsBlob := labelBuf.Bytes()

		writeIntSample(buf, scratch, "clustercost_namespace_pod_count", nsLabelsBlob, agg.podCount, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_cpu_usage_milli", nsLabelsBlob, agg.cpuUsageMilli, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_memory_rss_bytes_total", nsLabelsBlob, agg.memoryRssBytes, tsMillis)
		writeFloatSample(buf, scratch, "clustercost_namespace_hourly_cost", nsLabelsBlob, agg.hourlyCost, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_cpu_request_millicores", nsLabelsBlob, agg.cpuReqMilli, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_memory_request_bytes", nsLabelsBlob, agg.memReqBytes, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_network_tx_bytes_total", nsLabelsBlob, agg.netTxBytes, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_network_rx_bytes_total", nsLabelsBlob, agg.netRxBytes, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_network_egress_public_bytes_total", nsLabelsBlob, agg.egressPublic, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_network_egress_cross_az_bytes_total", nsLabelsBlob, agg.egressCrossAZ, tsMillis)
		writeIntSample(buf, scratch, "clustercost_namespace_network_egress_internal_bytes_total", nsLabelsBlob, agg.egressInternal, tsMillis)
	}

	// Cluster totals will be emitted after Node processing

	// 3b. Emit Node Metrics
	for _, node := range req.Nodes {
		if node == nil || node.NodeName == "" {
			continue
		}
		labelBuf.Reset()
		writeLabels(labelBuf, base, label{"node", node.NodeName})
		if node.NodeName == req.NodeName && req.InstanceType != "" {
			writeLabel(labelBuf, label{"instance_type", req.InstanceType})
		}
		nodeLabelsBlob := labelBuf.Bytes()

		writeIntSample(buf, scratch, "clustercost_node_cpu_usage_milli", nodeLabelsBlob, safeInt64(node.CpuUsageMillicores), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_memory_usage_bytes", nodeLabelsBlob, safeInt64(node.MemoryUsageBytes), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_cpu_capacity_milli", nodeLabelsBlob, safeInt64(node.CapacityCpuMillicores), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_memory_capacity_bytes", nodeLabelsBlob, safeInt64(node.CapacityMemoryBytes), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_cpu_allocatable_milli", nodeLabelsBlob, safeInt64(node.AllocatableCpuMillicores), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_memory_allocatable_bytes", nodeLabelsBlob, safeInt64(node.AllocatableMemoryBytes), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_cpu_requested_milli", nodeLabelsBlob, safeInt64(node.RequestedCpuMillicores), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_memory_requested_bytes", nodeLabelsBlob, safeInt64(node.RequestedMemoryBytes), tsMillis)
		writeIntSample(buf, scratch, "clustercost_node_cpu_throttling_ns_total", nodeLabelsBlob, safeInt64(node.ThrottlingNs), tsMillis)

		if node.AllocatableCpuMillicores > 0 {
			cpuPct := (float64(node.CpuUsageMillicores) / float64(node.AllocatableCpuMillicores)) * 100
			writeFloatSample(buf, scratch, "clustercost_node_cpu_usage_percent", nodeLabelsBlob, cpuPct, tsMillis)
		}
		if node.AllocatableMemoryBytes > 0 {
			memPct := (float64(node.MemoryUsageBytes) / float64(node.AllocatableMemoryBytes)) * 100
			writeFloatSample(buf, scratch, "clustercost_node_memory_usage_percent", nodeLabelsBlob, memPct, tsMillis)
		}

		// Node Network Metrics (Host Traffic)
		if node.Network != nil {
			nodeTx := safeInt64(node.Network.BytesSent)
			nodeRx := safeInt64(node.Network.BytesReceived)
			nodeEgressPublic := safeInt64(node.Network.EgressPublicBytes)
			nodeEgressCrossAZ := safeInt64(node.Network.EgressCrossAzBytes)
			nodeEgressInternal := safeInt64(node.Network.EgressInternalBytes)

			writeIntSample(buf, scratch, "clustercost_node_network_tx_bytes_total", nodeLabelsBlob, nodeTx, tsMillis)
			writeIntSample(buf, scratch, "clustercost_node_network_rx_bytes_total", nodeLabelsBlob, nodeRx, tsMillis)
			writeIntSample(buf, scratch, "clustercost_node_network_egress_public_bytes_total", nodeLabelsBlob, nodeEgressPublic, tsMillis)
			writeIntSample(buf, scratch, "clustercost_node_network_egress_cross_az_bytes_total", nodeLabelsBlob, nodeEgressCrossAZ, tsMillis)
			writeIntSample(buf, scratch, "clustercost_node_network_egress_internal_bytes_total", nodeLabelsBlob, nodeEgressInternal, tsMillis)

			// Aggregate Node Traffic to Cluster Totals
			clusterTx += nodeTx
			clusterRx += nodeRx
			clusterEgressPublic += nodeEgressPublic
			clusterEgressCrossAZ += nodeEgressCrossAZ
			clusterEgressInternal += nodeEgressInternal
		}
	}

	// 4. Emit Cluster Totals (Pod + Node)
	if clusterTx > 0 || clusterRx > 0 {
		writeIntSample(buf, scratch, "clustercost_cluster_network_tx_bytes_total", baseLabelsBlob, clusterTx, tsMillis)
		writeIntSample(buf, scratch, "clustercost_cluster_network_rx_bytes_total", baseLabelsBlob, clusterRx, tsMillis)
		writeIntSample(buf, scratch, "clustercost_cluster_network_egress_public_bytes_total", baseLabelsBlob, clusterEgressPublic, tsMillis)
		writeIntSample(buf, scratch, "clustercost_cluster_network_egress_cross_az_bytes_total", baseLabelsBlob, clusterEgressCrossAZ, tsMillis)
		writeIntSample(buf, scratch, "clustercost_cluster_network_egress_internal_bytes_total", baseLabelsBlob, clusterEgressInternal, tsMillis)
	}

}

func (i *Ingestor) appendNetworkReport(buf, labelBuf *bytes.Buffer, scratch []byte, agentName string, req *agentv1.NetworkReportRequest) {
	if req == nil {
		return
	}
	tsMillis := reportTimestampMillis(req.TimestampSeconds)
	if agentName == "" {
		agentName = req.AgentId
	}
	if agentName == "" {
		agentName = "unknown"
	}
	meta := i.agentMeta[agentName]
	base := baseLabels(agentName, req.ClusterId, "", meta)

	// Decode Compact Connections
	endpoints := req.Endpoints
	for _, compact := range req.CompactConnections {
		if compact == nil {
			continue
		}
		if int(compact.SrcIndex) >= len(endpoints) || int(compact.DstIndex) >= len(endpoints) {
			continue // Invalid index
		}
		src := endpoints[compact.SrcIndex]
		dst := endpoints[compact.DstIndex]

		// Build full connection object for label generation
		labelBuf.Reset()
		// Use specialized compact writer to avoid allocs
		writeCompactLabels(labelBuf, base, compact, src, dst)
		labelsBlob := labelBuf.Bytes()

		writeIntSample(buf, scratch, "clustercost_connection_bytes_sent_total", labelsBlob, safeInt64(compact.BytesSent), tsMillis)
		writeIntSample(buf, scratch, "clustercost_connection_bytes_received_total", labelsBlob, safeInt64(compact.BytesReceived), tsMillis)
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

func writeIntSample(buf *bytes.Buffer, scratch []byte, name string, labels []byte, value int64, tsMillis int64) {
	buf.WriteString(name)
	if len(labels) > 0 {
		buf.WriteByte('{')
		buf.Write(labels)
		buf.WriteByte('}')
	}
	buf.WriteByte(' ')
	buf.Write(strconv.AppendInt(scratch[:0], value, 10))
	buf.WriteByte(' ')
	buf.Write(strconv.AppendInt(scratch[:0], tsMillis, 10))
	buf.WriteByte('\n')
}

func writeFlagSample(buf *bytes.Buffer, scratch []byte, name string, labels []byte, value int64, tsMillis int64) {
	writeIntSample(buf, scratch, name, labels, value, tsMillis)
}

func writeFloatSample(buf *bytes.Buffer, scratch []byte, name string, labels []byte, value float64, tsMillis int64) {
	buf.WriteString(name)
	if len(labels) > 0 {
		buf.WriteByte('{')
		buf.Write(labels)
		buf.WriteByte('}')
	}
	buf.WriteByte(' ')
	buf.Write(strconv.AppendFloat(scratch[:0], value, 'f', -1, 64))
	buf.WriteByte(' ')
	buf.Write(strconv.AppendInt(scratch[:0], tsMillis, 10))
	buf.WriteByte('\n')
}

func writeLabels(buf *bytes.Buffer, base []label, extra ...label) {
	first := true
	for _, l := range base {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		writeLabelKV(buf, l.key, l.value)
	}
	for _, l := range extra {
		if strings.TrimSpace(l.value) != "" {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			writeLabelKV(buf, l.key, l.value)
		}
	}
}

func writeLabel(buf *bytes.Buffer, l label) {
	if buf.Len() > 0 {
		buf.WriteByte(',')
	}
	writeLabelKV(buf, l.key, l.value)
}

func writeLabelKV(buf *bytes.Buffer, k, v string) {
	buf.WriteString(k)
	buf.WriteString(`="`)
	_, _ = labelReplacer.WriteString(buf, v) // Uses global replacer and writes directly to buffer
	buf.WriteByte('"')
}

func writeCompactLabels(buf *bytes.Buffer, base []label, compact *agentv1.CompactNetworkConnection, src, dst *agentv1.NetworkEndpoint) {
	writeLabels(buf, base) // Writes base labels

	// Write extra manually
	writeLabel(buf, label{"protocol", strconv.FormatUint(uint64(compact.Protocol), 10)})
	writeLabel(buf, label{"egress_class", compact.EgressClass})
	writeLabel(buf, label{"dst_kind", compact.DstKind})
	writeLabel(buf, label{"service_match", compact.ServiceMatch})
	writeLabel(buf, label{"is_egress", strconv.FormatBool(compact.IsEgress)})

	if src != nil {
		writeEndpointLabels(buf, "src", src)
	}
	if dst != nil {
		writeEndpointLabels(buf, "dst", dst)
		services := joinServiceRefs(dst.Services)
		if services != "" {
			writeLabel(buf, label{"dst_services", services})
		}
	}
}

func writeEndpointLabels(buf *bytes.Buffer, prefix string, ep *agentv1.NetworkEndpoint) {
	writeLabel(buf, label{prefix + "_ip", ep.Ip})
	writeLabel(buf, label{prefix + "_namespace", ep.Namespace})
	writeLabel(buf, label{prefix + "_pod", ep.PodName})
	writeLabel(buf, label{prefix + "_node", ep.NodeName})
	writeLabel(buf, label{prefix + "_availability_zone", ep.AvailabilityZone})
	writeLabel(buf, label{prefix + "_dns_name", ep.DnsName})
}

func reportTimestampMillis(tsSeconds int64) int64 {
	if tsSeconds > 0 {
		return tsSeconds * 1000
	}
	return time.Now().UnixMilli()
}

func joinServiceRefs(services []*agentv1.ServiceRef) string {
	if len(services) == 0 {
		return ""
	}
	parts := make([]string, 0, len(services))
	for _, svc := range services {
		if svc == nil {
			continue
		}
		if svc.Namespace == "" && svc.Name == "" {
			continue
		}
		if svc.Namespace == "" {
			parts = append(parts, svc.Name)
			continue
		}
		parts = append(parts, svc.Namespace+"/"+svc.Name)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
