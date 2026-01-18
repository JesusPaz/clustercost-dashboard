package store

import (
	"testing"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
)

func newTestStore() *Store {
	cfgs := []config.AgentConfig{
		{Name: "test-agent", BaseURL: "http://example.com", Type: "k8s"},
	}
	s := New(cfgs, "v1.0.0")
	// Inject Mock Pricing
	s.pricing = NewPricingCatalog(&MockPricing{})
	return s
}

func TestClusterMetadataReturnsLatestSnapshot(t *testing.T) {
	s := newTestStore()
	s.UpdateMetrics("test-agent", &agentv1.MetricsReportRequest{
		AgentId:   "test-agent",
		ClusterId: "cluster-1",
		// ClusterName removed
		AvailabilityZone: "us-east-1",            // Using AZ as Region proxy
		Pods:             []*agentv1.PodMetric{}, // Changed from Snapshot
	})

	meta, err := s.ClusterMetadata()
	if err != nil {
		t.Fatalf("ClusterMetadata returned error: %v", err)
	}
	if meta.Name != "Cluster" {
		t.Fatalf("expected cluster name Cluster, got %q", meta.Name)
	}
	if meta.Type != "k8s" {
		t.Fatalf("expected cluster type k8s, got %q", meta.Type)
	}
	if meta.Region != "us-east-1" {
		t.Fatalf("expected region us-east-1, got %q", meta.Region)
	}
	if meta.Version != "v2.0" {
		t.Fatalf("expected version v2.0, got %q", meta.Version)
	}
	if meta.Timestamp.IsZero() {
		t.Fatal("expected metadata timestamp to be set")
	}
}

func TestAgentStatusConnectedWhenDataFresh(t *testing.T) {
	s := newTestStore()

	s.UpdateMetrics("test-agent", &agentv1.MetricsReportRequest{
		AgentId:          "test-agent",
		ClusterId:        "cluster-2",
		NodeName:         "node-1",
		AvailabilityZone: "us-west-2",
		Pods: []*agentv1.PodMetric{
			{
				Namespace: "default",
				PodName:   "pod-1",
				Memory:    &agentv1.MemoryMetrics{RssBytes: 1024 * 1024 * 100},
			},
		},
	})

	status, err := s.AgentStatus()
	if err != nil {
		t.Fatalf("AgentStatus returned error: %v", err)
	}
	if status.Status != "connected" {
		t.Fatalf("expected status connected, got %q", status.Status)
	}
	if status.ClusterName != "Cluster" {
		t.Fatalf("expected cluster name Cluster, got %q", status.ClusterName)
	}
	if status.ClusterType != "k8s" {
		t.Fatalf("expected cluster type k8s, got %q", status.ClusterType)
	}
	if status.ClusterRegion != "us-west-2" {
		t.Fatalf("expected region us-west-2, got %q", status.ClusterRegion)
	}
	if status.NodeCount != 1 {
		t.Fatalf("expected node count 1, got %d", status.NodeCount)
	}
	if status.Datasets.Namespaces != "ok" || status.Datasets.Nodes != "ok" || status.Datasets.Resources != "ok" {
		t.Fatalf("expected all datasets to be ok, got %+v", status.Datasets)
	}
}

func TestCPUUsageCalculation(t *testing.T) {
	s := newTestStore()
	agentID := "test-agent"

	// 1. Initial Report (Baseline)
	s.UpdateMetrics(agentID, &agentv1.MetricsReportRequest{
		AgentId:          agentID,
		ClusterId:        "cluster-1",
		NodeName:         "node-1",
		TimestampSeconds: 1000,
		Nodes: []*agentv1.NodeMetric{
			{NodeName: "node-1", CapacityCpuMillicores: 2000}, // 2 Cores
		},
		Pods: []*agentv1.PodMetric{
			{
				PodUid:    "pod-1",
				Namespace: "default",
				Cpu:       &agentv1.CpuMetrics{UsageMillicores: 250}, // 0.25 cores
			},
		},
	})

	// 2. Second Report (usage 500m)
	// % = 0.5 / 2.0 = 25%
	s.UpdateMetrics(agentID, &agentv1.MetricsReportRequest{
		AgentId:          agentID,
		ClusterId:        "cluster-1",
		NodeName:         "node-1",
		TimestampSeconds: 1010, // +10s
		Nodes: []*agentv1.NodeMetric{
			{NodeName: "node-1", CapacityCpuMillicores: 2000},
		},
		Pods: []*agentv1.PodMetric{
			{
				PodUid:    "pod-1",
				Namespace: "default",
				Cpu: &agentv1.CpuMetrics{
					UsageMillicores: 500,
					LimitMillicores: 2000, // 2 Cores
				},
			},
		},
	})

	summary, err := s.NamespaceDetail("default")
	if err != nil {
		t.Fatalf("NamespaceDetail failed: %v", err)
	}

	// Milli = 500
	if summary.CPUUsageMilli != 500 {
		t.Errorf("expected 500m CPU usage, got %d", summary.CPUUsageMilli)
	}

	// Percent = (500 / 2000) * 100 = 25%
	if summary.CPUUsagePercent != 25.0 {
		t.Errorf("expected 25.0 percent CPU usage, got %f", summary.CPUUsagePercent)
	}
}
