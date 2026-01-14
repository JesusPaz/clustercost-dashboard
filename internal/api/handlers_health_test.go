package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/store"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

type fakeMetricsProvider struct {
	meta   store.ClusterMetadata
	status store.AgentStatusPayload
}

func (f *fakeMetricsProvider) Overview(context.Context, int) (store.OverviewPayload, error) {
	return store.OverviewPayload{}, vm.ErrNoData
}
func (f *fakeMetricsProvider) NamespaceList(context.Context, store.NamespaceFilter) (store.NamespaceListResponse, error) {
	return store.NamespaceListResponse{}, vm.ErrNoData
}
func (f *fakeMetricsProvider) NamespaceDetail(context.Context, string) (store.NamespaceSummary, error) {
	return store.NamespaceSummary{}, vm.ErrNoData
}
func (f *fakeMetricsProvider) NodeList(context.Context, store.NodeFilter) (store.NodeListResponse, error) {
	return store.NodeListResponse{}, vm.ErrNoData
}
func (f *fakeMetricsProvider) NodeDetail(context.Context, string) (store.NodeSummary, error) {
	return store.NodeSummary{}, vm.ErrNoData
}
func (f *fakeMetricsProvider) Resources(context.Context) (store.ResourcesPayload, error) {
	return store.ResourcesPayload{}, vm.ErrNoData
}
func (f *fakeMetricsProvider) AgentStatus(context.Context) (store.AgentStatusPayload, error) {
	return f.status, nil
}
func (f *fakeMetricsProvider) Agents(context.Context) ([]store.AgentInfo, error) {
	return nil, vm.ErrNoData
}
func (f *fakeMetricsProvider) ClusterMetadata(context.Context) (store.ClusterMetadata, error) {
	return f.meta, nil
}
func (f *fakeMetricsProvider) NetworkTopology(context.Context, store.NetworkTopologyOptions) ([]store.NetworkEdge, error) {
	return nil, vm.ErrNoData
}

func newTestHandler(meta store.ClusterMetadata, status store.AgentStatusPayload) *Handler {
	return &Handler{vm: &fakeMetricsProvider{meta: meta, status: status}}
}

func TestHealthHandlerReturnsClusterMetadata(t *testing.T) {
	now := time.Now().UTC()
	h := newTestHandler(store.ClusterMetadata{
		ID:        "cluster-123",
		Name:      "Test Cluster",
		Type:      "k8s",
		Region:    "us-east-2",
		Version:   "dev",
		Timestamp: now,
	}, store.AgentStatusPayload{Status: "connected"})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	h.Health(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status OK, got %d", res.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["clusterName"] != "Test Cluster" {
		t.Fatalf("expected clusterName Test Cluster, got %v", payload["clusterName"])
	}
	if payload["clusterRegion"] != "us-east-2" {
		t.Fatalf("expected clusterRegion us-east-2, got %v", payload["clusterRegion"])
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", payload["status"])
	}
}
