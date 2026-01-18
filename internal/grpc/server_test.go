package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// ... (setup code remains similar)

func TestReportMetrics_Unary(t *testing.T) {
	// Rename to TestReportMetrics_Unary
	lis := bufconn.Listen(1024 * 1024)
	// Mock ingestor
	ingestor := &fakeIngestor{}
	// Use config.Config
	cfg := config.Config{
		DefaultAgentToken: "secret",
	}
	s := NewServer(cfg, ingestor, nil)
	// NewServer registers collector internally.

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("Server exited: %v", err)
		}
	}()
	defer s.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	client := agentv1.NewCollectorClient(conn)

	// Helper to add auth token
	authCtx := func(token string) context.Context {
		return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	}

	t.Run("Valid Metrics Report", func(t *testing.T) {
		req := &agentv1.MetricsReportRequest{
			AgentId:          "agent-1",
			ClusterId:        "cluster-1",
			AvailabilityZone: "us-east-1",
			Pods: []*agentv1.PodMetric{
				{
					Namespace: "default",
					PodName:   "pod-1",
					Cpu:       &agentv1.CpuMetrics{UsageMillicores: 250},
				},
			},
		}

		resp, err := client.ReportMetrics(authCtx("secret"), req)
		if err != nil {
			t.Fatalf("ReportMetrics failed: %v", err)
		}
		if !resp.Accepted {
			t.Errorf("Expected accepted=true, got false")
		}
	})

	// Add other test cases...
}

// fakeIngestor implementation...
type fakeIngestor struct{}

func (f *fakeIngestor) EnqueueMetrics(agentName string, req *agentv1.MetricsReportRequest) bool {
	return true
}

func (f *fakeIngestor) EnqueueNetwork(agentName string, req *agentv1.NetworkReportRequest) bool {
	return true
}
