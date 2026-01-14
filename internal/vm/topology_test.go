package vm

import (
	"strings"
	"testing"
)

func TestConnectionMetricExpr_NoNamespace(t *testing.T) {
	expr := connectionMetricExpr(
		"clustercost_connection_bytes_sent_total",
		map[string]string{"cluster_id": "cluster-1"},
		"",
		"1h",
		1700000000,
		"src_namespace,dst_namespace,protocol",
		"increase",
	)

	if !strings.Contains(expr, `sum by (src_namespace,dst_namespace,protocol)`) {
		t.Fatalf("expected sum by clause, got %s", expr)
	}
	if !strings.Contains(expr, `increase(clustercost_connection_bytes_sent_total{cluster_id="cluster-1"}[1h] @ 1700000000)`) {
		t.Fatalf("expected increase selector with @ end, got %s", expr)
	}
	if strings.Contains(expr, "or") {
		t.Fatalf("did not expect namespace filter OR, got %s", expr)
	}
}

func TestConnectionMetricExpr_WithNamespace(t *testing.T) {
	expr := connectionMetricExpr(
		"clustercost_connection_bytes_sent_total",
		map[string]string{"cluster_id": "cluster-1"},
		"payments",
		"30m",
		1700001234,
		"src_namespace,dst_namespace,protocol",
		"increase",
	)

	if !strings.Contains(expr, `or`) {
		t.Fatalf("expected namespace filter OR, got %s", expr)
	}
	if !strings.Contains(expr, `src_namespace="payments"`) || !strings.Contains(expr, `dst_namespace="payments"`) {
		t.Fatalf("expected namespace label filters, got %s", expr)
	}
	if !strings.Contains(expr, `@ 1700001234`) {
		t.Fatalf("expected explicit end timestamp, got %s", expr)
	}
}

func TestEdgeFromLabelsParsesProtocol(t *testing.T) {
	edge := edgeFromLabels(map[string]string{
		"src_namespace":         "payments",
		"src_pod":               "api-1",
		"src_node":              "node-a",
		"src_ip":                "10.0.0.1",
		"src_availability_zone": "us-east-1a",
		"dst_namespace":         "payments",
		"dst_pod":               "db-1",
		"dst_node":              "node-b",
		"dst_ip":                "10.0.0.2",
		"dst_availability_zone": "us-east-1b",
		"dst_kind":              "pod",
		"service_match":         "endpoint",
		"dst_services":          "payments/db",
		"protocol":              "6",
	}, nil)

	if edge == nil {
		t.Fatalf("expected edge from labels")
	}
	if edge.Protocol != 6 {
		t.Fatalf("expected protocol 6, got %d", edge.Protocol)
	}
	if edge.SrcAZ != "us-east-1a" || edge.DstAZ != "us-east-1b" {
		t.Fatalf("unexpected AZs: %s %s", edge.SrcAZ, edge.DstAZ)
	}
}
