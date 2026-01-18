package vm

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
)

func TestAppendReportConnectionsEmitsMetrics(t *testing.T) {
	endpoints := []*agentv1.NetworkEndpoint{
		{
			Ip:               "10.0.0.1",
			DnsName:          "api.internal.local",
			Namespace:        "default",
			PodName:          "pod-a",
			NodeName:         "node-a",
			AvailabilityZone: "us-east-1a",
		},
		{
			Ip:               "1.1.1.1",
			DnsName:          "api.example.com",
			AvailabilityZone: "us-east-1a",
			Services: []*agentv1.ServiceRef{
				{Namespace: "default", Name: "api"},
			},
		},
	}

	req := &agentv1.NetworkReportRequest{
		AgentId:          "agent-1",
		ClusterId:        "cluster-1",
		TimestampSeconds: 1700000000,
		Endpoints:        endpoints,
		CompactConnections: []*agentv1.CompactNetworkConnection{
			{
				SrcIndex:      0,
				DstIndex:      1,
				Protocol:      6,
				BytesSent:     100,
				BytesReceived: 200,
				EgressClass:   "public_internet",
				DstKind:       "external",
				ServiceMatch:  "none",
				IsEgress:      true,
			},
			{
				SrcIndex:      0,
				DstIndex:      1, // reusing for simplicity
				Protocol:      17,
				BytesSent:     300,
				BytesReceived: 400,
			},
		},
	}

	ing := &Ingestor{}
	var buf bytes.Buffer
	ing.appendReport(&buf, &bytes.Buffer{}, make([]byte, 64), reportEnvelope{agentName: "agent-1", networkReq: req})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	connLine := findMetricLine(lines, "clustercost_connection_bytes_sent_total")
	if connLine == "" {
		t.Fatalf("expected connection bytes metric in output")
	}

	_, labels, value, ts := parseMetricLine(t, connLine)
	if value != "100" {
		t.Fatalf("expected bytes sent 100, got %s", value)
	}
	if ts != 1700000000000 {
		t.Fatalf("expected timestamp 1700000000000, got %d", ts)
	}

	assertLabel(t, labels, "cluster_id", "cluster-1")
	assertLabel(t, labels, "agent_id", "agent-1")
	assertLabel(t, labels, "protocol", "6")
	assertLabel(t, labels, "egress_class", "public_internet")
	assertLabel(t, labels, "dst_kind", "external")
	assertLabel(t, labels, "service_match", "none")
	assertLabel(t, labels, "is_egress", "true")
	assertLabel(t, labels, "src_ip", "10.0.0.1")
	assertLabel(t, labels, "src_namespace", "default")
	assertLabel(t, labels, "src_pod", "pod-a")
	assertLabel(t, labels, "src_node", "node-a")
	assertLabel(t, labels, "src_availability_zone", "us-east-1a")
	assertLabel(t, labels, "src_dns_name", "api.internal.local")
	assertLabel(t, labels, "dst_ip", "1.1.1.1")
	assertLabel(t, labels, "dst_availability_zone", "us-east-1a")
	assertLabel(t, labels, "dst_dns_name", "api.example.com")
	assertLabel(t, labels, "dst_services", "default/api")

	// Cluster metrics now aggregated in MetricsReport, not NetworkReport

	// Egress cost metrics removed from the proto; bytes-only aggregation remains.
}

func TestAppendReportEmitsNodeMetrics(t *testing.T) {
	req := &agentv1.MetricsReportRequest{
		AgentId:          "agent-1",
		ClusterId:        "cluster-1",
		TimestampSeconds: 1700000000,
		Nodes: []*agentv1.NodeMetric{
			{
				NodeName:                 "node-a",
				CpuUsageMillicores:       1500,
				MemoryUsageBytes:         2147483648,
				CapacityCpuMillicores:    4000,
				CapacityMemoryBytes:      8589934592,
				AllocatableCpuMillicores: 3500,
				AllocatableMemoryBytes:   7516192768,
				RequestedCpuMillicores:   2000,
				RequestedMemoryBytes:     3221225472,
				ThrottlingNs:             123456789,
			},
		},
	}

	ing := &Ingestor{}
	var buf bytes.Buffer
	ing.appendReport(&buf, &bytes.Buffer{}, make([]byte, 64), reportEnvelope{agentName: "agent-1", metricsReq: req})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	cpuLine := findMetricLine(lines, "clustercost_node_cpu_usage_milli")
	if cpuLine == "" {
		t.Fatalf("expected node cpu usage metric in output")
	}
	_, labels, value, _ := parseMetricLine(t, cpuLine)
	if value != "1500" {
		t.Fatalf("expected node cpu usage 1500, got %s", value)
	}
	assertLabel(t, labels, "node", "node-a")

	memLine := findMetricLine(lines, "clustercost_node_memory_allocatable_bytes")
	if memLine == "" {
		t.Fatalf("expected node allocatable memory metric in output")
	}
	_, labels, value, _ = parseMetricLine(t, memLine)
	if value != "7516192768" {
		t.Fatalf("expected allocatable memory 7516192768, got %s", value)
	}
	assertLabel(t, labels, "node", "node-a")

	throttleLine := findMetricLine(lines, "clustercost_node_cpu_throttling_ns_total")
	if throttleLine == "" {
		t.Fatalf("expected node throttling metric in output")
	}
	_, labels, value, _ = parseMetricLine(t, throttleLine)
	if value != "123456789" {
		t.Fatalf("expected node throttling 123456789, got %s", value)
	}
	assertLabel(t, labels, "node", "node-a")
}

func TestAppendReportEmitsPodAndNamespaceHourlyCost(t *testing.T) {
	req := &agentv1.MetricsReportRequest{
		AgentId:          "agent-1",
		ClusterId:        "cluster-1",
		NodeName:         "node-a",
		InstanceType:     "t3.medium",
		TimestampSeconds: 1700000000,
		Nodes: []*agentv1.NodeMetric{
			{
				NodeName:              "node-a",
				CpuUsageMillicores:    1200,
				CapacityCpuMillicores: 2000,
				CapacityMemoryBytes:   8 * 1024 * 1024 * 1024,
			},
		},
		Pods: []*agentv1.PodMetric{
			{
				Namespace: "payments",
				PodName:   "api-1",
				Cpu: &agentv1.CpuMetrics{
					RequestMillicores: 500,
				},
				Memory: &agentv1.MemoryMetrics{
					RequestBytes: 1024 * 1024 * 1024,
				},
				Network: &agentv1.NetworkMetrics{
					BytesSent:     1000,
					BytesReceived: 2000,
				},
			},
		},
	}

	ing := &Ingestor{}
	var buf bytes.Buffer
	ing.appendReport(&buf, &bytes.Buffer{}, make([]byte, 64), reportEnvelope{agentName: "agent-1", metricsReq: req})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	podCostLine := findMetricLine(lines, "clustercost_pod_hourly_cost")
	if podCostLine == "" {
		t.Fatalf("expected pod hourly cost metric in output")
	}
	_, _, podCostValue, _ := parseMetricLine(t, podCostLine)
	podCost, err := strconv.ParseFloat(podCostValue, 64)
	if err != nil {
		t.Fatalf("expected pod hourly cost float, got %s", podCostValue)
	}
	if diff := podCost - 0.0078; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("expected pod hourly cost ~0.0078, got %v", podCost)
	}

	nsCostLine := findMetricLine(lines, "clustercost_namespace_hourly_cost")
	if nsCostLine == "" {
		t.Fatalf("expected namespace hourly cost metric in output")
	}
	_, labels, nsCostValue, _ := parseMetricLine(t, nsCostLine)
	if labels["namespace"] != "payments" {
		t.Fatalf("expected namespace payments, got %s", labels["namespace"])
	}
	nsCost, err := strconv.ParseFloat(nsCostValue, 64)
	if err != nil {
		t.Fatalf("expected namespace hourly cost float, got %s", nsCostValue)
	}
	if diff := nsCost - 0.0078; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("expected namespace hourly cost ~0.0078, got %v", nsCost)
	}

	nodeAllocLine := findMetricLine(lines, "clustercost_node_cpu_allocatable_milli")
	if nodeAllocLine == "" {
		t.Fatalf("expected node allocatable cpu metric in output")
	}
	_, labels, _, _ = parseMetricLine(t, nodeAllocLine)
	if labels["instance_type"] != "t3.medium" {
		t.Fatalf("expected instance_type t3.medium, got %s", labels["instance_type"])
	}
}

func TestReportTimestampMillisUsesReportTimestamp(t *testing.T) {
	got := reportTimestampMillis(1700001234)
	if got != 1700001234000 {
		t.Fatalf("expected timestamp seconds converted to ms, got %d", got)
	}
}

func findMetricLine(lines []string, metric string) string {
	for _, line := range lines {
		if strings.HasPrefix(line, metric+"{") || strings.HasPrefix(line, metric+" ") {
			return line
		}
	}
	return ""
}

func parseMetricLine(t *testing.T, line string) (string, map[string]string, string, int64) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		t.Fatalf("expected 3 fields in metric line, got %q", line)
	}

	nameAndLabels := fields[0]
	value := fields[1]
	ts, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp %q", fields[2])
	}

	name := nameAndLabels
	labels := map[string]string{}
	if idx := strings.Index(nameAndLabels, "{"); idx != -1 {
		name = nameAndLabels[:idx]
		labelsStr := strings.TrimSuffix(nameAndLabels[idx+1:], "}")
		if labelsStr != "" {
			for _, part := range strings.Split(labelsStr, ",") {
				kv := strings.SplitN(part, "=", 2)
				if len(kv) != 2 {
					continue
				}
				key := kv[0]
				val := strings.Trim(kv[1], "\"")
				labels[key] = val
			}
		}
	}

	return name, labels, value, ts
}

func assertLabel(t *testing.T, labels map[string]string, key, expected string) {
	t.Helper()
	if labels[key] != expected {
		t.Fatalf("expected label %s=%s, got %s", key, expected, labels[key])
	}
}

func TestReportEmitsEgressBreakdownMetrics(t *testing.T) {
	req := &agentv1.MetricsReportRequest{
		AgentId:          "agent-1",
		TimestampSeconds: 1700000000,
		Pods: []*agentv1.PodMetric{
			{
				Namespace: "backend",
				PodName:   "api-1",
				Network: &agentv1.NetworkMetrics{
					BytesSent:           1000,
					BytesReceived:       2000,
					EgressPublicBytes:   500,
					EgressCrossAzBytes:  300,
					EgressInternalBytes: 200,
				},
			},
		},
	}

	ing := &Ingestor{}
	var buf bytes.Buffer
	ing.appendReport(&buf, &bytes.Buffer{}, make([]byte, 64), reportEnvelope{agentName: "agent-1", metricsReq: req})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	// Verify Pod Metrics
	checkMetric(t, lines, "clustercost_pod_network_egress_public_bytes_total", "500")
	checkMetric(t, lines, "clustercost_pod_network_egress_cross_az_bytes_total", "300")
	checkMetric(t, lines, "clustercost_pod_network_egress_internal_bytes_total", "200")

	// Verify Namespace Metrics
	checkMetric(t, lines, "clustercost_namespace_network_egress_public_bytes_total", "500")
	checkMetric(t, lines, "clustercost_namespace_network_egress_cross_az_bytes_total", "300")
	checkMetric(t, lines, "clustercost_namespace_network_egress_internal_bytes_total", "200")

	// Verify Cluster Metrics
	checkMetric(t, lines, "clustercost_cluster_network_egress_public_bytes_total", "500")
	checkMetric(t, lines, "clustercost_cluster_network_egress_cross_az_bytes_total", "300")
	checkMetric(t, lines, "clustercost_cluster_network_egress_internal_bytes_total", "200")
}

func TestReportEmitsNodeNetworkMetrics(t *testing.T) {
	req := &agentv1.MetricsReportRequest{
		AgentId:          "agent-1",
		TimestampSeconds: 1700000000,
		Nodes: []*agentv1.NodeMetric{
			{
				NodeName: "node-1",
				Network: &agentv1.NetworkMetrics{
					BytesSent:           100,
					BytesReceived:       200,
					EgressPublicBytes:   50,
					EgressCrossAzBytes:  30,
					EgressInternalBytes: 20,
				},
			},
		},
	}

	ing := &Ingestor{}
	var buf bytes.Buffer
	ing.appendReport(&buf, &bytes.Buffer{}, make([]byte, 64), reportEnvelope{agentName: "agent-1", metricsReq: req})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	// Verify Node Metrics
	checkMetric(t, lines, "clustercost_node_network_tx_bytes_total", "100")
	checkMetric(t, lines, "clustercost_node_network_rx_bytes_total", "200")
	checkMetric(t, lines, "clustercost_node_network_egress_public_bytes_total", "50")

	// Verify Cluster Aggregation (Should contain Node metrics)
	checkMetric(t, lines, "clustercost_cluster_network_tx_bytes_total", "100")
	checkMetric(t, lines, "clustercost_cluster_network_egress_public_bytes_total", "50")
}

func checkMetric(t *testing.T, lines []string, name, expectedVal string) {
	t.Helper()
	line := findMetricLine(lines, name)
	if line == "" {
		t.Fatalf("metric %s not found", name)
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		t.Fatalf("invalid metric line: %s", line)
	}
	if fields[1] != expectedVal {
		t.Errorf("metric %s: expected %s, got %s", name, expectedVal, fields[1])
	}
}
