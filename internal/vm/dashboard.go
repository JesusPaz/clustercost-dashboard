package vm

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/store"
)

const hoursPerMonth = 24 * 30

func (c *Client) Overview(ctx context.Context, limit int) (store.OverviewPayload, error) {
	namespaces, ts, err := c.namespaceMetrics(ctx, "", "")
	if err != nil {
		return store.OverviewPayload{}, err
	}
	if len(namespaces) == 0 {
		return store.OverviewPayload{}, ErrNoData
	}

	list := make([]store.NamespaceSummary, 0, len(namespaces))
	envCost := map[string]float64{
		"production": 0,
		"nonprod":    0,
		"system":     0,
		"unknown":    0,
	}
	totalHourly := 0.0
	for _, ns := range namespaces {
		totalHourly += ns.HourlyCost
		list = append(list, *ns)
		env := normalizeEnvironment(ns.Environment)
		envCost[env] += ns.HourlyCost
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].HourlyCost > list[j].HourlyCost
	})

	topLimit := limit
	if topLimit <= 0 || topLimit > len(list) {
		topLimit = len(list)
	}
	topNamespaces := make([]store.TopNamespaceEntry, 0, topLimit)
	for idx := 0; idx < topLimit; idx++ {
		topNamespaces = append(topNamespaces, store.TopNamespaceEntry{
			Namespace:   list[idx].Namespace,
			Environment: valueOrDefault(list[idx].Environment, "unknown"),
			HourlyCost:  list[idx].HourlyCost,
		})
	}

	meta, _ := c.ClusterMetadata(ctx)
	clusterName := meta.ID
	if meta.Name != "" {
		clusterName = meta.Name
	}

	if ts.IsZero() {
		ts = meta.Timestamp
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return store.OverviewPayload{
		ClusterName:         clusterName,
		Timestamp:           ts,
		TotalHourlyCost:     totalHourly,
		TotalMonthlyCost:    totalHourly * hoursPerMonth,
		EnvCostHourly:       envCost,
		TopNamespacesByCost: topNamespaces,
		SavingsCandidates:   findSavingsCandidates(list),
	}, nil
}

func (c *Client) NamespaceList(ctx context.Context, filter store.NamespaceFilter) (store.NamespaceListResponse, error) {
	namespaces, ts, err := c.namespaceMetrics(ctx, filter.Environment, "")
	if err != nil {
		return store.NamespaceListResponse{}, err
	}

	var searchLower string
	if filter.Search != "" {
		searchLower = strings.ToLower(filter.Search)
	}

	out := make([]store.NamespaceSummary, 0, len(namespaces))
	for _, ns := range namespaces {
		if searchLower != "" && !strings.Contains(strings.ToLower(ns.Namespace), searchLower) {
			continue
		}
		out = append(out, *ns)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].HourlyCost > out[j].HourlyCost
	})

	total := len(out)
	start := clampIndex(filter.Offset, total)
	end := clampIndex(filter.Offset+filter.Limit, total)

	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return store.NamespaceListResponse{
		Items:      out[start:end],
		TotalCount: total,
		Timestamp:  ts,
	}, nil
}

func (c *Client) NamespaceDetail(ctx context.Context, name string) (store.NamespaceSummary, error) {
	namespaces, _, err := c.namespaceMetrics(ctx, "", name)
	if err != nil {
		return store.NamespaceSummary{}, err
	}
	for _, ns := range namespaces {
		if ns.Namespace == name {
			return *ns, nil
		}
	}
	return store.NamespaceSummary{}, ErrNoData
}

func (c *Client) NodeList(ctx context.Context, filter store.NodeFilter) (store.NodeListResponse, error) {
	nodes, ts, err := c.nodeMetrics(ctx, "")
	if err != nil {
		return store.NodeListResponse{}, err
	}

	var searchLower string
	if filter.Search != "" {
		searchLower = strings.ToLower(filter.Search)
	}

	out := make([]store.NodeSummary, 0, len(nodes))
	for _, node := range nodes {
		if searchLower != "" && !strings.Contains(strings.ToLower(node.NodeName), searchLower) {
			continue
		}
		out = append(out, *node)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].HourlyCost > out[j].HourlyCost
	})

	total := len(out)
	start := clampIndex(filter.Offset, total)
	end := clampIndex(filter.Offset+filter.Limit, total)

	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return store.NodeListResponse{
		Items:      out[start:end],
		TotalCount: total,
		Timestamp:  ts,
	}, nil
}

func (c *Client) NodeDetail(ctx context.Context, name string) (store.NodeSummary, error) {
	nodes, _, err := c.nodeMetrics(ctx, name)
	if err != nil {
		return store.NodeSummary{}, err
	}
	for _, node := range nodes {
		if node.NodeName == name {
			return *node, nil
		}
	}
	return store.NodeSummary{}, ErrNoData
}

func (c *Client) Resources(ctx context.Context) (store.ResourcesPayload, error) {
	cpuUsage, cpuUsageTS, err := c.scalarMetric(ctx, "clustercost_cluster_cpu_usage_milli_total")
	if err != nil && err != ErrNoData {
		return store.ResourcesPayload{}, err
	}
	cpuRequest, _, err := c.scalarMetric(ctx, "clustercost_cluster_cpu_request_milli_total")
	if err != nil && err != ErrNoData {
		return store.ResourcesPayload{}, err
	}
	memUsage, _, err := c.scalarMetric(ctx, "clustercost_cluster_memory_usage_bytes_total")
	if err != nil && err != ErrNoData {
		return store.ResourcesPayload{}, err
	}
	memRequest, _, err := c.scalarMetric(ctx, "clustercost_cluster_memory_request_bytes_total")
	if err != nil && err != ErrNoData {
		return store.ResourcesPayload{}, err
	}
	nodeHourlyCost, _, err := c.scalarMetric(ctx, "clustercost_cluster_total_node_hourly_cost")
	if err != nil && err != ErrNoData {
		return store.ResourcesPayload{}, err
	}

	// Fetch Network Metrics
	netTx, _, _ := c.scalarMetric(ctx, "clustercost_cluster_network_tx_bytes_total")
	netRx, _, _ := c.scalarMetric(ctx, "clustercost_cluster_network_rx_bytes_total")
	netEgress, _, _ := c.scalarMetric(ctx, "clustercost_cluster_network_egress_cost_total")

	namespaces, _, nsErr := c.namespaceMetrics(ctx, "", "")
	if nsErr != nil && nsErr != ErrNoData {
		return store.ResourcesPayload{}, nsErr
	}

	if cpuUsage == 0 && cpuRequest == 0 && len(namespaces) > 0 {
		for _, ns := range namespaces {
			cpuUsage += float64(ns.CPUUsageMilli)
			cpuRequest += float64(ns.CPURequestMilli)
		}
	}
	if memUsage == 0 && memRequest == 0 && len(namespaces) > 0 {
		for _, ns := range namespaces {
			memUsage += float64(ns.MemoryUsageBytes)
			memRequest += float64(ns.MemoryRequestBytes)
		}
	}
	if nodeHourlyCost == 0 {
		nodeHourlyCost = c.sumNodeHourlyCost(namespaces)
	}

	if cpuUsage == 0 && cpuRequest == 0 && memUsage == 0 && memRequest == 0 && nodeHourlyCost == 0 && len(namespaces) == 0 {
		return store.ResourcesPayload{}, ErrNoData
	}

	cpuEfficiency := percent(cpuUsage, cpuRequest)
	memEfficiency := percent(memUsage, memRequest)
	cpuWasteCost := wasteCost(nodeHourlyCost, cpuUsage, cpuRequest)
	memWasteCost := wasteCost(nodeHourlyCost, memUsage, memRequest)

	ts := cpuUsageTS
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return store.ResourcesPayload{
		Timestamp: ts,
		CPU: store.CPUResource{
			UsageMilli:               int64(cpuUsage),
			RequestMilli:             int64(cpuRequest),
			EfficiencyPercent:        cpuEfficiency,
			EstimatedHourlyWasteCost: cpuWasteCost,
		},
		Memory: store.MemoryResource{
			UsageBytes:               int64(memUsage),
			RequestBytes:             int64(memRequest),
			EfficiencyPercent:        memEfficiency,
			EstimatedHourlyWasteCost: memWasteCost,
		},
		Network: store.NetworkResource{
			TxBytesTotal:     int64(netTx),
			RxBytesTotal:     int64(netRx),
			EgressCostHourly: netEgress,
		},
		NamespaceWaste: buildNamespaceWaste(namespaces),
	}, nil
}

func (c *Client) NetworkTopology(ctx context.Context, opts store.NetworkTopologyOptions) ([]store.NetworkEdge, error) {
	clusterID := opts.ClusterID
	if clusterID == "" {
		clusterID = c.resolveClusterID(ctx)
	}
	if clusterID == "" {
		return nil, ErrNoData
	}
	ctx = WithClusterID(ctx, clusterID)

	if opts.End.IsZero() {
		opts.End = time.Now().UTC()
	}
	if opts.Start.IsZero() {
		opts.Start = opts.End.Add(-c.lookback)
	}

	window := opts.End.Sub(opts.Start)
	if window <= 0 {
		window = c.lookback
	}
	windowStr := formatDuration(window)

	labels := map[string]string{"cluster_id": clusterID}
	queryNamespace := ""
	if len(opts.Namespaces) == 1 {
		queryNamespace = opts.Namespaces[0]
	}
	groupLabels := []string{
		"src_namespace",
		"src_pod",
		"src_node",
		"src_ip",
		"src_dns_name",
		"src_availability_zone",
		"dst_namespace",
		"dst_pod",
		"dst_node",
		"dst_ip",
		"dst_dns_name",
		"dst_availability_zone",
		"dst_kind",
		"service_match",
		"dst_services",
		"protocol",
	}
	groupBy := strings.Join(groupLabels, ",")

	endSeconds := opts.End.UTC().Unix()

	bytesSentExpr := connectionMetricExpr("clustercost_connection_bytes_sent_total", labels, queryNamespace, windowStr, endSeconds, groupBy, "increase")
	bytesRecvExpr := connectionMetricExpr("clustercost_connection_bytes_received_total", labels, queryNamespace, windowStr, endSeconds, groupBy, "increase")
	countExpr := connectionMetricExpr("clustercost_connection_bytes_sent_total", labels, queryNamespace, windowStr, endSeconds, groupBy, "count")

	sentSamples, err := c.query(ctx, bytesSentExpr)
	if err != nil {
		return nil, err
	}
	recvSamples, err := c.query(ctx, bytesRecvExpr)
	if err != nil {
		return nil, err
	}

	startSeconds := opts.Start.UTC().Unix()
	edges := make(map[string]*store.NetworkEdge)

	namespaceSet := make(map[string]struct{}, len(opts.Namespaces))
	for _, namespace := range opts.Namespaces {
		if namespace == "" {
			continue
		}
		namespaceSet[namespace] = struct{}{}
	}

	applySample := func(sample sample, assign func(*store.NetworkEdge, float64)) {
		edge := edgeFromLabels(sample.labels, groupLabels)
		if edge == nil {
			return
		}
		if len(namespaceSet) > 0 {
			if _, ok := namespaceSet[edge.SrcNamespace]; !ok {
				if _, ok := namespaceSet[edge.DstNamespace]; !ok {
					return
				}
			}
		}
		key := edgeKey(sample.labels, groupLabels)
		current := edges[key]
		if current == nil {
			edge.FirstSeen = startSeconds
			edge.LastSeen = endSeconds
			edges[key] = edge
			current = edge
		}
		assign(current, sample.value)
	}

	for _, sample := range sentSamples {
		applySample(sample, func(edge *store.NetworkEdge, value float64) {
			edge.BytesSent = int64(value)
		})
	}
	for _, sample := range recvSamples {
		applySample(sample, func(edge *store.NetworkEdge, value float64) {
			edge.BytesReceived = int64(value)
		})
	}
	countSamples, err := c.query(ctx, countExpr)
	if err != nil {
		return nil, err
	}
	for _, sample := range countSamples {
		applySample(sample, func(edge *store.NetworkEdge, value float64) {
			edge.ConnectionCount = int64(value)
		})
	}

	if len(edges) == 0 {
		return nil, ErrNoData
	}

	list := make([]store.NetworkEdge, 0, len(edges))
	for _, edge := range edges {
		if opts.MinCostUSD > 0 && edge.EgressCostUSD <= opts.MinCostUSD {
			continue
		}
		if opts.MinBytes > 0 && (edge.BytesSent+edge.BytesReceived) < opts.MinBytes {
			continue
		}
		if opts.MinConnections > 0 && edge.ConnectionCount < opts.MinConnections {
			continue
		}
		list = append(list, *edge)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].EgressCostUSD != list[j].EgressCostUSD {
			return list[i].EgressCostUSD > list[j].EgressCostUSD
		}
		totalI := list[i].BytesSent + list[i].BytesReceived
		totalJ := list[j].BytesSent + list[j].BytesReceived
		return totalI > totalJ
	})

	if opts.Limit > 0 && len(list) > opts.Limit {
		list = list[:opts.Limit]
	}
	return list, nil
}

func (c *Client) AgentStatus(ctx context.Context) (store.AgentStatusPayload, error) {
	clusterID := c.resolveClusterID(ctx)
	ctx = WithClusterID(ctx, clusterID)
	agentSamples, err := c.seriesTimestamp(ctx, "clustercost_agent_up", nil)
	if err != nil {
		return store.AgentStatusPayload{}, err
	}
	if len(agentSamples) == 0 {
		return store.AgentStatusPayload{}, ErrNoData
	}

	lastSync := latestTimestamp(agentSamples)
	if lastSync.IsZero() {
		return store.AgentStatusPayload{}, ErrNoData
	}

	nsTS := c.seriesTimestampSafe(ctx, "clustercost_namespace_hourly_cost")
	nodeTS := c.seriesTimestampSafe(ctx, "clustercost_node_hourly_cost")
	resTS := c.seriesTimestampSafe(ctx, "clustercost_cluster_cpu_usage_milli_total")

	datasets := store.AgentDatasetHealth{
		Namespaces: datasetStatus(!nsTS.IsZero(), nsTS, lastSync),
		Nodes:      datasetStatus(!nodeTS.IsZero(), nodeTS, lastSync),
		Resources:  datasetStatus(!resTS.IsZero(), resTS, lastSync),
	}

	var status string
	allOK := datasets.Namespaces == "ok" && datasets.Nodes == "ok" && datasets.Resources == "ok"
	if time.Since(lastSync) > agentOfflineThreshold {
		status = "offline"
	} else if allOK {
		status = "connected"
	} else {
		status = "partial"
	}

	meta, _ := c.ClusterMetadata(ctx)
	version := meta.Version
	updateAvailable := c.recommendedAgentVersion != "" && version != "" && version != c.recommendedAgentVersion

	nodeCount := len(c.nodeNames(ctx))

	return store.AgentStatusPayload{
		Status:          status,
		LastSync:        lastSync,
		Datasets:        datasets,
		Version:         version,
		UpdateAvailable: updateAvailable,
		ClusterName:     meta.Name,
		ClusterType:     meta.Type,
		ClusterRegion:   meta.Region,
		NodeCount:       nodeCount,
	}, nil
}

func (c *Client) Agents(ctx context.Context) ([]store.AgentInfo, error) {
	clusterID := c.resolveClusterID(ctx)
	ctx = WithClusterID(ctx, clusterID)

	// Explicitly query for agents active in the last 24 hours
	expr := "max_over_time(timestamp(clustercost_agent_up)[24h])"
	samples, err := c.query(ctx, expr)
	if err != nil && err != ErrNoData {
		return nil, err
	}

	for idx := range samples {
		samples[idx].timestamp = time.Unix(int64(samples[idx].value), 0)
	}
	agentSamples := samples

	configuredNames := make(map[string]bool)
	agentMap := make(map[string]store.AgentInfo)
	for _, cfg := range c.agents {
		configuredNames[cfg.Name] = true
		agentMap[cfg.Name] = store.AgentInfo{
			Name:    cfg.Name,
			BaseURL: cfg.BaseURL,
			Status:  "unknown",
		}
	}

	now := time.Now()
	for _, sample := range agentSamples {
		// Try agent_id first, fallback to cluster_id or just "unknown"
		name := sample.labels["agent_id"]
		if name == "" {
			name = sample.labels["cluster_id"]
		}
		if name == "" {
			continue // skip samples without identification
		}

		info := agentMap[name]
		info.Name = name
		info.LastScrapeTime = sample.timestamp
		info.ClusterID = sample.labels["cluster_id"]
		info.NodeName = sample.labels["node"]

		if sample.timestamp.IsZero() {
			info.Status = "unknown"
		} else if now.Sub(sample.timestamp) > agentOfflineThreshold {
			info.Status = "offline"
		} else {
			info.Status = "connected"
		}
		agentMap[name] = info
	}

	result := make([]store.AgentInfo, 0, len(agentMap))
	for _, info := range agentMap {
		// Filter: only include agents seen in the last 24 hours or configured statically
		if time.Since(info.LastScrapeTime) > 24*time.Hour && info.Status == "unknown" {
			// If it's a static agent that we haven't seen, deciding whether to keep it.
			// The user asked for "show only agents connected in last 24h".
			// So, if it's static ("unknown") and no data, maybe exclude?
			// But for now, let's keep static configs if they exist, but definitely filter out dynamic ones that are too old.
			if !configuredNames[info.Name] {
				continue
			}
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (c *Client) ClusterMetadata(ctx context.Context) (store.ClusterMetadata, error) {
	clusterID := c.resolveClusterID(ctx)
	ctx = WithClusterID(ctx, clusterID)
	agentSamples, err := c.seriesTimestamp(ctx, "clustercost_agent_up", nil)
	if err != nil {
		return store.ClusterMetadata{}, err
	}
	if len(agentSamples) == 0 {
		return store.ClusterMetadata{}, ErrNoData
	}

	latest := pickLatestSample(agentSamples)
	if latest == nil {
		return store.ClusterMetadata{}, ErrNoData
	}

	clusterIDLabel := latest.labels["cluster_id"]
	clusterName := latest.labels["cluster_name"]
	if clusterName == "" {
		clusterName = clusterIDLabel
	}

	return store.ClusterMetadata{
		ID:        clusterIDLabel,
		Name:      clusterName,
		Type:      latest.labels["cluster_type"],
		Region:    latest.labels["cluster_region"],
		Version:   latest.labels["version"],
		Timestamp: latest.timestamp,
	}, nil
}

func (c *Client) namespaceMetrics(ctx context.Context, environment, namespace string) (map[string]*store.NamespaceSummary, time.Time, error) {
	clusterID := c.resolveClusterID(ctx)
	ctx = WithClusterID(ctx, clusterID)
	labels := map[string]string{}
	if environment != "" {
		labels["environment"] = environment
	}
	if namespace != "" {
		labels["namespace"] = namespace
	}

	// We use pod metrics and aggregate them on the fly
	metrics := []struct {
		name     string
		agg      string // "sum" or "count"
		assign   func(entry *store.NamespaceSummary, value float64)
		expr     func(clusterID string, labels map[string]string) string
		fallback string
	}{
		{"clustercost_namespace_pod_count", "sum", func(e *store.NamespaceSummary, v float64) { e.PodCount = int(v) }, nil, ""},
		{
			"clustercost_namespace_cpu_request_millicores",
			"sum",
			func(e *store.NamespaceSummary, v float64) { e.CPURequestMilli = int64(v) },
			nil,
			"clustercost_pod_cpu_request_millicores",
		},
		{
			"clustercost_namespace_cpu_usage_milli",
			"sum",
			func(e *store.NamespaceSummary, v float64) { e.CPUUsageMilli = int64(v) },
			func(clusterID string, labels map[string]string) string {
				return fmt.Sprintf("sum by (namespace, environment) (%s)", c.lookbackExpr("clustercost_namespace_cpu_usage_milli", labels, clusterID))
			},
			"clustercost_pod_cpu_usage_milli",
		},
		{
			"clustercost_namespace_memory_request_bytes",
			"sum",
			func(e *store.NamespaceSummary, v float64) { e.MemoryRequestBytes = int64(v) },
			nil,
			"clustercost_pod_memory_request_bytes",
		},
		{"clustercost_namespace_memory_rss_bytes_total", "sum", func(e *store.NamespaceSummary, v float64) { e.MemoryUsageBytes = int64(v) }, nil, ""},
	}

	out := make(map[string]*store.NamespaceSummary)
	var latest time.Time

	// Regex to identify UUID-like strings (which are likely garbage/incorrect namespaces)
	uuidPattern := regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

	for _, metric := range metrics {
		// e.g. sum by (namespace, environment) (last_over_time(clustercost_pod_hourly_cost{...}[1h]))
		// Note: lookbackExpr returns "last_over_time(metric{...}[lookback])"
		// We wrap that in the aggregation.
		queryExpr := func(metricName string) (string, error) {
			if metric.expr != nil && metricName == metric.name {
				return metric.expr(clusterID, labels), nil
			}
			return fmt.Sprintf("%s by (namespace, environment) (%s)", metric.agg, c.lookbackExpr(metricName, labels, clusterID)), nil
		}

		expr, _ := queryExpr(metric.name)
		samples, err := c.query(ctx, expr)
		if err != nil {
			return nil, time.Time{}, err
		}
		if len(samples) == 0 && metric.fallback != "" {
			fallbackExpr, _ := queryExpr(metric.fallback)
			samples, err = c.query(ctx, fallbackExpr)
			if err != nil {
				return nil, time.Time{}, err
			}
		}
		for _, sample := range samples {
			ns := sample.labels["namespace"]
			if ns == "" {
				continue
			}
			// Filter out UUID-like namespaces as they are likely misreported or noise
			if uuidPattern.MatchString(ns) {
				continue
			}

			env := sample.labels["environment"]
			key := namespaceKey(ns, env)
			entry := out[key]
			if entry == nil {
				entry = &store.NamespaceSummary{
					Namespace:   ns,
					Environment: env,
					Labels:      map[string]string{},
				}
				out[key] = entry
			}
			metric.assign(entry, sample.value)
		}
	}

	queryScalar := func(expr string) (float64, error) {
		samples, err := c.query(ctx, expr)
		if err != nil {
			return 0, err
		}
		if len(samples) == 0 {
			return 0, ErrNoData
		}
		return samples[0].value, nil
	}

	nodeCostExpr := fmt.Sprintf("sum(max by (node) (%s))", c.lookbackExpr("clustercost_node_hourly_cost", nil, clusterID))
	cpuAllocExpr := fmt.Sprintf("sum(max by (node) (%s))", c.lookbackExpr("clustercost_node_cpu_allocatable_milli", nil, clusterID))
	memAllocExpr := fmt.Sprintf("sum(max by (node) (%s))", c.lookbackExpr("clustercost_node_memory_allocatable_bytes", nil, clusterID))

	nodeCost, err := queryScalar(nodeCostExpr)
	if err == nil && nodeCost > 0 {
		cpuAllocMilli, errCPU := queryScalar(cpuAllocExpr)
		memAllocBytes, errMem := queryScalar(memAllocExpr)
		if errCPU == nil && errMem == nil && cpuAllocMilli > 0 && memAllocBytes > 0 {
			cpuPrice := (nodeCost * 0.5) / (cpuAllocMilli / 1000.0)
			memPrice := (nodeCost * 0.5) / (memAllocBytes / (1024.0 * 1024.0 * 1024.0))
			for _, entry := range out {
				cpuUsageCores := float64(entry.CPUUsageMilli) / 1000.0
				memUsageGB := float64(entry.MemoryUsageBytes) / (1024.0 * 1024.0 * 1024.0)
				entry.HourlyCost = (cpuUsageCores * cpuPrice) + (memUsageGB * memPrice)
			}
		}
	}

	latest = c.seriesTimestampSafe(ctx, "clustercost_namespace_memory_rss_bytes_total")

	type nodeAlloc struct {
		instanceType string
		region       string
		cpuMilli     float64
		memBytes     float64
	}
	nodes := make(map[string]*nodeAlloc)
	loadNodeAlloc := func(metric string, assign func(entry *nodeAlloc, value float64)) error {
		expr := fmt.Sprintf("max by (node,instance_type,cluster_region) (%s)", c.lookbackExpr(metric, nil, clusterID))
		samples, err := c.query(ctx, expr)
		if err != nil {
			return err
		}
		for _, sample := range samples {
			node := sample.labels["node"]
			if node == "" {
				continue
			}
			entry := nodes[node]
			if entry == nil {
				entry = &nodeAlloc{
					instanceType: sample.labels["instance_type"],
					region:       sample.labels["cluster_region"],
				}
				nodes[node] = entry
			}
			if entry.instanceType == "" {
				entry.instanceType = sample.labels["instance_type"]
			}
			if entry.region == "" {
				entry.region = sample.labels["cluster_region"]
			}
			assign(entry, sample.value)
		}
		return nil
	}

	if err := loadNodeAlloc("clustercost_node_cpu_allocatable_milli", func(entry *nodeAlloc, value float64) {
		entry.cpuMilli = value
	}); err == nil {
		_ = loadNodeAlloc("clustercost_node_memory_allocatable_bytes", func(entry *nodeAlloc, value float64) {
			entry.memBytes = value
		})
	}

	pricing := store.NewPricingCatalog(nil)
	totalNodeCost := 0.0
	totalCpuCores := 0.0
	totalMemGB := 0.0
	for _, entry := range nodes {
		if entry.cpuMilli > 0 {
			totalCpuCores += entry.cpuMilli / 1000.0
		}
		if entry.memBytes > 0 {
			totalMemGB += entry.memBytes / (1024.0 * 1024.0 * 1024.0)
		}
		instanceType := entry.instanceType
		if instanceType == "" {
			instanceType = "default"
		}
		totalNodeCost += pricing.GetTotalNodePrice(context.Background(), entry.region, instanceType)
	}

	if totalNodeCost > 0 && totalCpuCores > 0 && totalMemGB > 0 {
		cpuPrice := (totalNodeCost * 0.5) / totalCpuCores
		memPrice := (totalNodeCost * 0.5) / totalMemGB
		for _, entry := range out {
			cpuUsageCores := float64(entry.CPUUsageMilli) / 1000.0
			memUsageGB := float64(entry.MemoryUsageBytes) / (1024.0 * 1024.0 * 1024.0)
			entry.HourlyCost = (cpuUsageCores * cpuPrice) + (memUsageGB * memPrice)
		}
	}
	return out, latest, nil
}

func (c *Client) nodeMetrics(ctx context.Context, nodeName string) (map[string]*store.NodeSummary, time.Time, error) {
	clusterID := c.resolveClusterID(ctx)
	ctx = WithClusterID(ctx, clusterID)
	labels := map[string]string{}
	if nodeName != "" {
		labels["node"] = nodeName
	}

	metrics := []struct {
		name   string
		assign func(entry *store.NodeSummary, value float64, labels map[string]string)
	}{
		{"clustercost_node_hourly_cost", func(e *store.NodeSummary, v float64, l map[string]string) {
			e.HourlyCost = v
			if e.InstanceType == "" {
				e.InstanceType = l["instance_type"]
			}
		}},
		{"clustercost_node_cpu_usage_percent", func(e *store.NodeSummary, v float64, _ map[string]string) { e.CPUUsagePercent = v }},
		{"clustercost_node_memory_usage_percent", func(e *store.NodeSummary, v float64, _ map[string]string) { e.MemoryUsagePercent = v }},
		{"clustercost_node_cpu_allocatable_milli", func(e *store.NodeSummary, v float64, _ map[string]string) { e.CPUAllocatableMilli = int64(v) }},
		{"clustercost_node_memory_allocatable_bytes", func(e *store.NodeSummary, v float64, _ map[string]string) { e.MemoryAllocatableBytes = int64(v) }},
		{"clustercost_node_pod_count", func(e *store.NodeSummary, v float64, _ map[string]string) { e.PodCount = int(v) }},
		{"clustercost_node_under_pressure", func(e *store.NodeSummary, v float64, _ map[string]string) { e.IsUnderPressure = v > 0.5 }},
	}

	out := make(map[string]*store.NodeSummary)
	for _, metric := range metrics {
		by := "node"
		if metric.name == "clustercost_node_hourly_cost" {
			by = "node,instance_type"
		}
		expr := fmt.Sprintf("max by (%s) (%s)", by, c.lookbackExpr(metric.name, labels, clusterID))
		samples, err := c.query(ctx, expr)
		if err != nil {
			return nil, time.Time{}, err
		}
		for _, sample := range samples {
			node := sample.labels["node"]
			if node == "" {
				continue
			}
			entry := out[node]
			if entry == nil {
				entry = &store.NodeSummary{
					NodeName: node,
					Labels:   map[string]string{},
					Taints:   []string{},
				}
				out[node] = entry
			}
			metric.assign(entry, sample.value, sample.labels)
		}
	}

	statusSamples, err := c.seriesTimestamp(ctx, "clustercost_node_status", labels)
	if err != nil && err != ErrNoData {
		return nil, time.Time{}, err
	}
	for node, status := range pickLatestStatus(statusSamples) {
		entry := out[node]
		if entry != nil {
			entry.Status = status
		}
	}

	latest := c.seriesTimestampSafe(ctx, "clustercost_node_hourly_cost")
	return out, latest, nil
}

func (c *Client) scalarMetric(ctx context.Context, metric string) (float64, time.Time, error) {
	clusterID := c.resolveClusterID(ctx)
	ctx = WithClusterID(ctx, clusterID)
	expr := fmt.Sprintf("sum(%s)", c.lookbackExpr(metric, nil, clusterID))
	samples, err := c.query(ctx, expr)
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(samples) == 0 {
		return 0, time.Time{}, ErrNoData
	}
	latest := c.seriesTimestampSafe(ctx, metric)
	return samples[0].value, latest, nil
}

func formatDuration(value time.Duration) string {
	seconds := int64(value.Seconds())
	if seconds <= 0 {
		return "0s"
	}
	if seconds%86400 == 0 {
		return fmt.Sprintf("%dd", seconds/86400)
	}
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}

func edgeKey(labels map[string]string, keys []string) string {
	if len(labels) == 0 {
		return ""
	}
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, labels[key])
	}
	return strings.Join(values, "|")
}

func edgeFromLabels(labels map[string]string, keys []string) *store.NetworkEdge {
	if len(labels) == 0 {
		return nil
	}
	protocol := int64(0)
	if raw := labels["protocol"]; raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			protocol = parsed
		}
	}

	return &store.NetworkEdge{
		SrcNamespace: labels["src_namespace"],
		SrcPodName:   labels["src_pod"],
		SrcNodeName:  labels["src_node"],
		SrcIP:        labels["src_ip"],
		SrcDNSName:   labels["src_dns_name"],
		SrcAZ:        labels["src_availability_zone"],
		DstNamespace: labels["dst_namespace"],
		DstPodName:   labels["dst_pod"],
		DstNodeName:  labels["dst_node"],
		DstIP:        labels["dst_ip"],
		DstDNSName:   labels["dst_dns_name"],
		DstAZ:        labels["dst_availability_zone"],
		DstKind:      labels["dst_kind"],
		ServiceMatch: labels["service_match"],
		DstServices:  labels["dst_services"],
		Protocol:     protocol,
	}
}

func connectionMetricExpr(metric string, baseLabels map[string]string, namespace, window string, endSeconds int64, groupBy, op string) string {
	rangeExpr := func(selector string) string {
		switch op {
		case "count":
			return fmt.Sprintf("(count_over_time(%s[%s] @ %d) > 0)", selector, window, endSeconds)
		default:
			return fmt.Sprintf("increase(%s[%s] @ %d)", selector, window, endSeconds)
		}
	}

	selector := metricSelector(metric, baseLabels)
	if namespace == "" {
		return fmt.Sprintf("sum by (%s) (%s)", groupBy, rangeExpr(selector))
	}

	srcLabels := copyLabels(baseLabels)
	dstLabels := copyLabels(baseLabels)
	srcLabels["src_namespace"] = namespace
	dstLabels["dst_namespace"] = namespace

	srcSelector := metricSelector(metric, srcLabels)
	dstSelector := metricSelector(metric, dstLabels)
	return fmt.Sprintf("sum by (%s) ((%s) or (%s))", groupBy, rangeExpr(srcSelector), rangeExpr(dstSelector))
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (c *Client) seriesTimestamp(ctx context.Context, metric string, labels map[string]string) ([]sample, error) {
	clusterID := clusterIDFromContext(ctx)
	scoped := c.scopedLabels(labels, clusterID)
	expr := fmt.Sprintf("max_over_time(timestamp(%s)[%s])", metricSelector(metric, scoped), c.lookback.String())
	samples, err := c.query(ctx, expr)
	if err != nil {
		return nil, err
	}
	for idx := range samples {
		samples[idx].timestamp = time.Unix(int64(samples[idx].value), 0)
	}
	return samples, nil
}

func (c *Client) seriesTimestampSafe(ctx context.Context, metric string) time.Time {
	samples, err := c.seriesTimestamp(ctx, metric, nil)
	if err != nil {
		return time.Time{}
	}
	return latestTimestamp(samples)
}

func latestTimestamp(samples []sample) time.Time {
	var latest time.Time
	for _, s := range samples {
		if s.timestamp.After(latest) {
			latest = s.timestamp
		}
	}
	return latest
}

func pickLatestSample(samples []sample) *sample {
	var latest *sample
	for idx := range samples {
		current := &samples[idx]
		if latest == nil || current.timestamp.After(latest.timestamp) {
			latest = current
		}
	}
	return latest
}

func pickLatestStatus(samples []sample) map[string]string {
	latest := make(map[string]sample)
	for _, s := range samples {
		node := s.labels["node"]
		status := s.labels["status"]
		if node == "" || status == "" {
			continue
		}
		if existing, ok := latest[node]; !ok || s.timestamp.After(existing.timestamp) {
			latest[node] = s
		}
	}
	out := make(map[string]string, len(latest))
	for node, sample := range latest {
		out[node] = sample.labels["status"]
	}
	return out
}

func (c *Client) nodeNames(ctx context.Context) []string {
	clusterID := c.resolveClusterID(ctx)
	expr := fmt.Sprintf("max by (node) (%s)", c.lookbackExpr("clustercost_node_hourly_cost", nil, clusterID))
	samples, err := c.query(ctx, expr)
	if err != nil {
		return nil
	}
	nodes := make([]string, 0, len(samples))
	for _, sample := range samples {
		if node := sample.labels["node"]; node != "" {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func namespaceKey(ns, env string) string {
	return fmt.Sprintf("%s|%s", ns, env)
}

func normalizeEnvironment(env string) string {
	switch strings.ToLower(env) {
	case "prod", "production":
		return "production"
	case "nonprod", "dev", "development":
		return "nonprod"
	case "system":
		return "system"
	default:
		return "unknown"
	}
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func datasetStatus(hasData bool, timestamp, fallback time.Time) string {
	if !hasData {
		return "missing"
	}
	effective := timestamp
	if effective.IsZero() {
		effective = fallback
	}
	if effective.IsZero() {
		return "partial"
	}
	if time.Since(effective) > datasetFreshThreshold {
		return "partial"
	}
	return "ok"
}

func clampIndex(idx, max int) int {
	if idx < 0 {
		return 0
	}
	if idx > max {
		return max
	}
	return idx
}

func findSavingsCandidates(namespaces []store.NamespaceSummary) []store.SavingsCandidate {
	const utilizationThreshold = 0.4
	const costThreshold = 0.05

	candidates := make([]store.SavingsCandidate, 0)
	for _, ns := range namespaces {
		if ns.HourlyCost < costThreshold {
			continue
		}
		cpuRatio := usageRatio(float64(ns.CPUUsageMilli), float64(ns.CPURequestMilli))
		memRatio := usageRatio(float64(ns.MemoryUsageBytes), float64(ns.MemoryRequestBytes))
		if (ns.CPURequestMilli > 0 && cpuRatio <= utilizationThreshold) ||
			(ns.MemoryRequestBytes > 0 && memRatio <= utilizationThreshold) {
			candidates = append(candidates, store.SavingsCandidate{
				Namespace:          ns.Namespace,
				Environment:        valueOrDefault(ns.Environment, "unknown"),
				HourlyCost:         ns.HourlyCost,
				CPURequestMilli:    ns.CPURequestMilli,
				CPUUsageMilli:      ns.CPUUsageMilli,
				MemoryRequestBytes: ns.MemoryRequestBytes,
				MemoryUsageBytes:   ns.MemoryUsageBytes,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].HourlyCost > candidates[j].HourlyCost
	})

	if len(candidates) > 5 {
		return candidates[:5]
	}
	return candidates
}

func buildNamespaceWaste(data map[string]*store.NamespaceSummary) []store.NamespaceWasteEntry {
	if len(data) == 0 {
		return []store.NamespaceWasteEntry{}
	}

	out := make([]store.NamespaceWasteEntry, 0, len(data))
	for _, ns := range data {
		cpuWaste := wastePercent(float64(ns.CPUUsageMilli), float64(ns.CPURequestMilli))
		memWaste := wastePercent(float64(ns.MemoryUsageBytes), float64(ns.MemoryRequestBytes))
		if cpuWaste == 0 && memWaste == 0 {
			continue
		}
		wasteRatio := maxFloat(
			unusedRatio(float64(ns.CPUUsageMilli), float64(ns.CPURequestMilli)),
			unusedRatio(float64(ns.MemoryUsageBytes), float64(ns.MemoryRequestBytes)),
		)
		out = append(out, store.NamespaceWasteEntry{
			Namespace:                ns.Namespace,
			Environment:              valueOrDefault(ns.Environment, "unknown"),
			CPUWastePercent:          cpuWaste,
			MemoryWastePercent:       memWaste,
			EstimatedHourlyWasteCost: ns.HourlyCost * wasteRatio,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].EstimatedHourlyWasteCost > out[j].EstimatedHourlyWasteCost
	})

	if len(out) > 5 {
		return out[:5]
	}
	return out
}

func usageRatio(usage, request float64) float64 {
	if request <= 0 {
		return 0
	}
	return usage / request
}

func percent(num, denom float64) float64 {
	if denom <= 0 {
		return 0
	}
	val := (num / denom) * 100
	if val < 0 {
		return 0
	}
	return val
}

func wastePercent(usage, request float64) float64 {
	if request <= 0 {
		return 0
	}
	waste := (1 - (usage / request)) * 100
	return clampFloat(waste, 0, 100)
}

func unusedRatio(usage, request float64) float64 {
	if request <= 0 {
		return 0
	}
	return clampFloat(1-(usage/request), 0, 1)
}

func wasteCost(nodeCost, usage, request float64) float64 {
	if request <= 0 || nodeCost <= 0 {
		return 0
	}
	return nodeCost * clampFloat(1-(usage/request), 0, 1)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (c *Client) sumNodeHourlyCost(namespaces map[string]*store.NamespaceSummary) float64 {
	total := 0.0
	for _, ns := range namespaces {
		total += ns.HourlyCost
	}
	return total
}
