package vm

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
)

func TestAppendReportConnectionsEmitsMetrics(t *testing.T) {
	req := &agentv1.ReportRequest{
		AgentId:          "agent-1",
		ClusterId:        "cluster-1",
		TimestampSeconds: 1700000000,
		Connections: []*agentv1.NetworkConnection{
			{
				Src: &agentv1.NetworkEndpoint{
					Ip:               "10.0.0.1",
					Namespace:        "default",
					PodName:          "pod-a",
					NodeName:         "node-a",
					AvailabilityZone: "us-east-1a",
				},
				Dst: &agentv1.NetworkEndpoint{
					Ip:               "1.1.1.1",
					AvailabilityZone: "us-east-1a",
					Services: []*agentv1.ServiceRef{
						{Namespace: "default", Name: "api"},
					},
				},
				Protocol:      6,
				BytesSent:     100,
				BytesReceived: 200,
				EgressClass:   "public_internet",
				EgressCostUsd: 0.25,
				DstKind:       "external",
				ServiceMatch:  "none",
				IsEgress:      true,
			},
			{
				Protocol:      17,
				BytesSent:     300,
				BytesReceived: 400,
				EgressCostUsd: 0.75,
			},
		},
	}

	ing := &Ingestor{}
	var buf bytes.Buffer
	ing.appendReport(&buf, reportEnvelope{agentName: "agent-1", req: req})

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
	assertLabel(t, labels, "dst_ip", "1.1.1.1")
	assertLabel(t, labels, "dst_availability_zone", "us-east-1a")
	assertLabel(t, labels, "dst_services", "default/api")

	txLine := findMetricLine(lines, "clustercost_cluster_network_tx_bytes_total")
	if txLine == "" {
		t.Fatalf("expected cluster tx bytes metric in output")
	}
	_, _, txValue, ts := parseMetricLine(t, txLine)
	if txValue != "400" {
		t.Fatalf("expected cluster tx bytes 400, got %s", txValue)
	}
	if ts != 1700000000000 {
		t.Fatalf("expected timestamp 1700000000000, got %d", ts)
	}

	rxLine := findMetricLine(lines, "clustercost_cluster_network_rx_bytes_total")
	if rxLine == "" {
		t.Fatalf("expected cluster rx bytes metric in output")
	}
	_, _, rxValue, _ := parseMetricLine(t, rxLine)
	if rxValue != "600" {
		t.Fatalf("expected cluster rx bytes 600, got %s", rxValue)
	}

	egressLine := findMetricLine(lines, "clustercost_cluster_network_egress_cost_total")
	if egressLine == "" {
		t.Fatalf("expected cluster egress cost metric in output")
	}
	_, _, egressValue, _ := parseMetricLine(t, egressLine)
	egressFloat, err := strconv.ParseFloat(egressValue, 64)
	if err != nil {
		t.Fatalf("expected egress cost float, got %s", egressValue)
	}
	if egressFloat != 1.0 {
		t.Fatalf("expected cluster egress cost 1.0, got %v", egressFloat)
	}
}

func TestReportTimestampMillisUsesReportTimestamp(t *testing.T) {
	req := &agentv1.ReportRequest{TimestampSeconds: 1700001234}
	got := reportTimestampMillis(req)
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
