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
	return New(cfgs, "v1.0.0")
}

func TestClusterMetadataReturnsLatestSnapshot(t *testing.T) {
	s := newTestStore()
	s.Update("test-agent", &agentv1.ReportRequest{
		AgentId:          "test-agent",
		ClusterId:        "cluster-1",
		ClusterName:      "Cluster One",
		AvailabilityZone: "us-east-1", // Using AZ as Region proxy
		Snapshot: &agentv1.Snapshot{
			Pods: []*agentv1.PodMetric{},
		},
	})

	meta, err := s.ClusterMetadata()
	if err != nil {
		t.Fatalf("ClusterMetadata returned error: %v", err)
	}
	if meta.Name != "Cluster One" {
		t.Fatalf("expected cluster name to be preserved, got %q", meta.Name)
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

	s.Update("test-agent", &agentv1.ReportRequest{
		AgentId:          "test-agent",
		ClusterId:        "cluster-2",
		ClusterName:      "Cluster Two",
		NodeName:         "node-1",
		AvailabilityZone: "us-west-2",
		Snapshot: &agentv1.Snapshot{
			Pods: []*agentv1.PodMetric{
				{
					Namespace:     "default",
					Pod:           "pod-1",
					MemoryMetrics: &agentv1.MemoryMetrics{RssBytes: 1024 * 1024 * 100},
				},
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
	if status.ClusterName != "Cluster Two" {
		t.Fatalf("expected cluster name Cluster Two, got %q", status.ClusterName)
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
