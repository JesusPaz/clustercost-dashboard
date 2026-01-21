package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	"github.com/clustercost/clustercost-dashboard/internal/store"
)

// ErrNoData indicates that VictoriaMetrics returned no usable data.
var ErrNoData = errors.New("no data available")

const (
	queryPath             = "/api/v1/query"
	datasetFreshThreshold = 2 * time.Minute
	agentOfflineThreshold = 5 * time.Minute
	defaultQueryTimeout   = 5 * time.Second
	defaultQueryCacheTTL  = 5 * time.Second
)

// Client queries VictoriaMetrics for dashboard data.
type Client struct {
	baseURL                 string
	lookback                time.Duration
	recommendedAgentVersion string
	agents                  []config.AgentConfig
	httpClient              *http.Client
	authToken               string
	username                string
	password                string
	cacheTTL                time.Duration
	cacheMu                 sync.Mutex
	cache                   map[string]cachedQuery
	pricing                 *store.PricingCatalog
}

// SetPricingCatalog allows injecting the pricing catalog.
func (c *Client) SetPricingCatalog(p *store.PricingCatalog) {
	c.pricing = p
}

type cachedQuery struct {
	expires time.Time
	samples []sample
}

// NewClient creates a VictoriaMetrics query client.
func NewClient(cfg config.Config) (*Client, error) {
	if cfg.VictoriaMetricsURL == "" {
		return nil, fmt.Errorf("victoria metrics url is required")
	}

	base, err := buildQueryURL(cfg.VictoriaMetricsURL)
	if err != nil {
		return nil, err
	}

	timeout := cfg.VictoriaMetricsTimeout
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}

	lookback := cfg.VictoriaMetricsLookback
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}

	c := &Client{
		baseURL:                 base,
		lookback:                lookback,
		recommendedAgentVersion: cfg.RecommendedAgentVersion,
		agents:                  cfg.Agents,
		httpClient:              &http.Client{Timeout: timeout},
		authToken:               cfg.VictoriaMetricsToken,
		username:                cfg.VictoriaMetricsUsername,
		password:                cfg.VictoriaMetricsPassword,
		cacheTTL:                defaultQueryCacheTTL,
		cache:                   make(map[string]cachedQuery),
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := c.Ping(ctx); err != nil {
			log.Printf("[VictoriaMetrics] WARN: Failed initial connection check to %s: %v", base, err)
		} else {
			log.Printf("[VictoriaMetrics] Successfully connected to %s", base)
		}
	}()

	return c, nil
}

// Ping checks connectivity to VictoriaMetrics.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.query(ctx, "1")
	return err
}

func buildQueryURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid victoria metrics url: %w", err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("victoria metrics url missing scheme: %s", base)
	}
	parsed.Path = path.Join(parsed.Path, queryPath)
	return parsed.String(), nil
}

type sample struct {
	labels    map[string]string
	value     float64
	timestamp time.Time
}

type vmResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

func (c *Client) query(ctx context.Context, expr string) ([]sample, error) {
	if cached, ok := c.loadCached(expr); ok {
		return cached, nil
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse query url: %w", err)
	}
	q := u.Query()
	q.Set("query", expr)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	} else if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query victoria metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("victoria metrics responded with status %d", resp.StatusCode)
	}

	var payload vmResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode victoria metrics response: %w", err)
	}
	if payload.Status != "success" {
		if payload.Error != "" {
			return nil, fmt.Errorf("victoria metrics error: %s", payload.Error)
		}
		return nil, fmt.Errorf("victoria metrics status %s", payload.Status)
	}

	out := make([]sample, 0, len(payload.Data.Result))
	for _, item := range payload.Data.Result {
		if len(item.Value) != 2 {
			continue
		}
		ts, ok := parseFloat(item.Value[0])
		if !ok {
			continue
		}
		val, ok := parseFloat(item.Value[1])
		if !ok {
			continue
		}
		out = append(out, sample{
			labels:    item.Metric,
			value:     val,
			timestamp: time.Unix(int64(ts), 0),
		})
	}
	c.storeCached(expr, out)
	return out, nil
}

func (c *Client) loadCached(expr string) ([]sample, bool) {
	if c.cacheTTL <= 0 {
		return nil, false
	}
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	entry, ok := c.cache[expr]
	if !ok || time.Now().After(entry.expires) {
		delete(c.cache, expr)
		return nil, false
	}
	out := make([]sample, len(entry.samples))
	copy(out, entry.samples)
	return out, true
}

func (c *Client) storeCached(expr string, samples []sample) {
	if c.cacheTTL <= 0 {
		return
	}
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	out := make([]sample, len(samples))
	copy(out, samples)
	c.cache[expr] = cachedQuery{
		expires: time.Now().Add(c.cacheTTL),
		samples: out,
	}
}

func parseFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		val, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		return val, true
	default:
		return 0, false
	}
}

func (c *Client) lookbackExpr(metric string, labels map[string]string, clusterID string) string {
	selector := metricSelector(metric, c.scopedLabels(labels, clusterID))
	return fmt.Sprintf("last_over_time(%s[%s])", selector, c.lookback.String())
}

func metricSelector(metric string, labels map[string]string) string {
	if len(labels) == 0 {
		return metric
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(metric)
	b.WriteByte('{')
	for idx, key := range keys {
		if idx > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteString(`="`)
		b.WriteString(escapeLabelValue(labels[key]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return replacer.Replace(value)
}

type clusterIDKey struct{}

// WithClusterID sets the preferred cluster id for subsequent queries.
func WithClusterID(ctx context.Context, clusterID string) context.Context {
	if clusterID == "" {
		return ctx
	}
	return context.WithValue(ctx, clusterIDKey{}, clusterID)
}

func clusterIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(clusterIDKey{}).(string); ok {
		return val
	}
	return ""
}

func (c *Client) resolveClusterID(ctx context.Context) string {
	if clusterID := clusterIDFromContext(ctx); clusterID != "" {
		return clusterID
	}
	clusterID, _ := c.latestClusterID(ctx)
	return clusterID
}

func (c *Client) latestClusterID(ctx context.Context) (string, error) {
	samples, err := c.seriesTimestamp(ctx, "clustercost_agent_up", nil)
	if err != nil {
		return "", err
	}
	latest := pickLatestSample(samples)
	if latest == nil {
		return "", ErrNoData
	}
	return latest.labels["cluster_id"], nil
}

func (c *Client) scopedLabels(labels map[string]string, clusterID string) map[string]string {
	if clusterID == "" {
		return labels
	}
	if labels == nil {
		labels = map[string]string{}
	}
	if _, ok := labels["cluster_id"]; !ok {
		labels["cluster_id"] = clusterID
	}
	return labels
}

// GetPodP95Usage returns the 95th percentile of CPU and Memory usage for a specific pod over the lookback period.
// cpu is in cores, memory is in bytes.
func (c *Client) GetPodP95Usage(ctx context.Context, clusterID, namespace, podName string) (cpuCores float64, memoryBytes float64, err error) {
	// Construct labels for specific pod
	labels := map[string]string{
		"namespace": namespace,
		"pod":       podName,
	}
	if clusterID != "" {
		labels["cluster_id"] = clusterID
	}

	// CPU Query: quantile_over_time(0.95, clustercost_pod_cpu_usage_milli{...}[1h]) / 1000
	// Pod CPU usage is reported as millicores (gauge).
	cpuQuery := fmt.Sprintf("quantile_over_time(0.95, clustercost_pod_cpu_usage_milli%s[%s]) / 1000",
		formatLabels(labels), c.lookback.String())

	// Memory Query: quantile_over_time(0.95, clustercost_pod_memory_rss_bytes{...}[1h])
	memQuery := fmt.Sprintf("quantile_over_time(0.95, clustercost_pod_memory_rss_bytes%s[%s])",
		formatLabels(labels), c.lookback.String())

	// Execute CPU query
	cpuSamples, err := c.query(ctx, cpuQuery)
	if err != nil {
		return 0, 0, fmt.Errorf("query cpu p95: %w", err)
	}
	if len(cpuSamples) > 0 {
		cpuCores = cpuSamples[0].value
	}

	// Execute Memory query
	memSamples, err := c.query(ctx, memQuery)
	if err != nil {
		return 0, 0, fmt.Errorf("query memory p95: %w", err)
	}
	if len(memSamples) > 0 {
		memoryBytes = memSamples[0].value
	}

	return cpuCores, memoryBytes, nil
}

// GetNodeStats calculates the average usage and real cost of a node over a time window.
func (c *Client) GetNodeStats(ctx context.Context, clusterID, nodeName string, window time.Duration) (store.NodeStats, error) {
	if nodeName == "" {
		return store.NodeStats{}, fmt.Errorf("node name is required")
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	windowStr := formatDuration(window)

	labels := map[string]string{
		"node": nodeName,
	}
	if clusterID != "" {
		labels["cluster_id"] = clusterID
	}

	// 1. Get P95 Usage %
	// quantile_over_time(0.95, clustercost_node_cpu_usage_percent{node="name"}[window])
	cpuQuery := fmt.Sprintf("quantile_over_time(0.95, clustercost_node_cpu_usage_percent%s[%s])",
		formatLabels(labels), windowStr)
	memQuery := fmt.Sprintf("quantile_over_time(0.95, clustercost_node_memory_usage_percent%s[%s])",
		formatLabels(labels), windowStr)

	// 2. Get Average Hourly Cost (to account for potential spot price fluctuations or just stability)
	costQuery := fmt.Sprintf("avg_over_time(clustercost_node_hourly_cost%s[%s])",
		formatLabels(labels), windowStr)

	cpuSamples, err := c.query(ctx, cpuQuery)
	if err != nil {
		return store.NodeStats{}, fmt.Errorf("query cpu stats: %w", err)
	}
	memSamples, err := c.query(ctx, memQuery)
	if err != nil {
		return store.NodeStats{}, fmt.Errorf("query mem stats: %w", err)
	}
	costSamples, err := c.query(ctx, costQuery)
	if err != nil {
		return store.NodeStats{}, fmt.Errorf("query cost stats: %w", err)
	}

	p95Cpu := 0.0
	if len(cpuSamples) > 0 {
		p95Cpu = cpuSamples[0].value
	}
	p95Mem := 0.0
	if len(memSamples) > 0 {
		p95Mem = memSamples[0].value
	}
	avgHourlyCost := 0.0
	if len(costSamples) > 0 {
		avgHourlyCost = costSamples[0].value
	}

	// Calculate Costs
	totalMonthly := avgHourlyCost * hoursPerMonth // 720 hours
	// "Real Usage" = (Total * 0.5 * Cpu%) + (Total * 0.5 * Mem%)
	// Percentages are 0-100 in VM usually (based on ingestion code: `writeFloatSample(..., cpuPct, ...)` where `cpuPct` was * 100)

	realUsageMonthly := (totalMonthly * 0.5 * (p95Cpu / 100.0)) + (totalMonthly * 0.5 * (p95Mem / 100.0))

	return store.NodeStats{
		NodeName:              nodeName,
		P95CPUUsagePercent:    p95Cpu,
		P95MemoryUsagePercent: p95Mem,
		TotalMonthlyCost:      totalMonthly,
		RealUsageMonthlyCost:  realUsageMonthly,
		Window:                windowStr,
	}, nil
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(labels[k]) // Simple escape for now, assume safe chars
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// GetNodePods returns 24h P95 and Request metrics for all pods on a specific node.
func (c *Client) GetNodePods(ctx context.Context, clusterID, nodeName string, window time.Duration) ([]store.PodMetrics, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	windowStr := formatDuration(window)

	labels := map[string]string{
		"node": nodeName,
	}
	if clusterID != "" {
		labels["cluster_id"] = clusterID
	}
	labelStr := formatLabels(labels)

	// We need 5 metrics per pod:
	// 1. CPU Request (Max)
	// 2. CPU Limit (Max) - to determine QoS
	// 3. Mem Request (Max)
	// 4. CPU Usage (P95)
	// 5. Mem Usage (P95)

	queries := map[string]string{
		"cpu_req_max": fmt.Sprintf("max_over_time(clustercost_pod_cpu_request_millicores%s[%s])", labelStr, windowStr),
		"cpu_lim_max": fmt.Sprintf("max_over_time(clustercost_pod_cpu_limit_millicores%s[%s])", labelStr, windowStr),
		"mem_req_max": fmt.Sprintf("max_over_time(clustercost_pod_memory_request_bytes%s[%s])", labelStr, windowStr),
		"cpu_add_p95": fmt.Sprintf("quantile_over_time(0.95, clustercost_pod_cpu_usage_milli%s[%s])", labelStr, windowStr),
		"mem_add_p95": fmt.Sprintf("quantile_over_time(0.95, clustercost_pod_memory_rss_bytes%s[%s])", labelStr, windowStr),
	}

	// Helper struct to aggregate data
	type podData struct {
		Namespace string
		PodName   string
		CPUReq    float64
		CPULim    float64
		MemReq    float64
		CPUP95    float64
		MemP95    float64
	}
	podMap := make(map[string]*podData)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for key, query := range queries {
		wg.Add(1)
		go func(k, q string) {
			defer wg.Done()
			samples, err := c.query(ctx, q)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			for _, s := range samples {
				ns := s.labels["namespace"]
				pod := s.labels["pod"]
				if ns == "" || pod == "" {
					continue
				}
				id := ns + "|" + pod
				if _, exists := podMap[id]; !exists {
					podMap[id] = &podData{Namespace: ns, PodName: pod}
				}
				p := podMap[id]

				switch k {
				case "cpu_req_max":
					p.CPUReq = s.value
				case "cpu_lim_max":
					p.CPULim = s.value
				case "mem_req_max":
					p.MemReq = s.value
				case "cpu_add_p95":
					p.CPUP95 = s.value
				case "mem_add_p95":
					p.MemP95 = s.value
				}
			}
			mu.Unlock()
		}(key, query)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, fmt.Errorf("failed to query pod metrics: %w", firstErr)
	}

	results := make([]store.PodMetrics, 0, len(podMap))
	for _, p := range podMap {
		// QoS Logic
		qos := "Burstable"
		if p.CPUReq == 0 && p.MemReq == 0 {
			qos = "BestEffort"
		} else if p.CPUReq == p.CPULim && p.CPULim > 0 {
			qos = "Guaranteed" // Simplified, strictly checking CPU for now
		}

		results = append(results, store.PodMetrics{
			PodName:            p.PodName,
			Namespace:          p.Namespace,
			QoSClass:           qos,
			CPURequestMilli:    int64(p.CPUReq),
			CPUP95Milli:        p.CPUP95,
			MemoryRequestBytes: int64(p.MemReq),
			MemoryP95Bytes:     p.MemP95,
		})
	}

	// Sort by Waste Amount (heuristic: max diff)
	sort.Slice(results, func(i, j int) bool {
		// Just sorting by name for stability for now, frontend handles logic sort
		if results[i].Namespace != results[j].Namespace {
			return results[i].Namespace < results[j].Namespace
		}
		return results[i].PodName < results[j].PodName
	})

	return results, nil
}
