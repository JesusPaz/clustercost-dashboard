package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	ccgrpc "github.com/clustercost/clustercost-dashboard/internal/grpc"
	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeIngestor struct {
	received chan *agentv1.ReportRequest
}

func (f *fakeIngestor) Enqueue(agentName string, req *agentv1.ReportRequest) bool {
	if req == nil {
		return false
	}
	f.received <- req
	return true
}

func TestGRPCServer_Integration(t *testing.T) {
	// 1. Setup
	token := "valid-token-123"
	defaultToken := "global-default"
	agentName := "test-agent"
	unknownAgent := "new-agent-using-default"

	cfg := config.Config{
		GrpcAddr:          ":0", // Random port
		DefaultAgentToken: defaultToken,
		Agents: []config.AgentConfig{
			{Name: agentName, Token: token, Type: "k8s"},
		},
	}

	// Start Server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	ingestor := &fakeIngestor{received: make(chan *agentv1.ReportRequest, 10)}
	srv := ccgrpc.NewServer(cfg, ingestor)

	// Run server in goroutine
	go func() {
		if err := srv.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			// t.Logf("server stopped: %v", err)
		}
	}()
	defer srv.Stop()

	// 2. Client Setup
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := agentv1.NewCollectorClient(conn)

	// 3. Test Case: Unauthenticated
	ctx := context.Background()
	_, err = c.Report(ctx, &agentv1.ReportRequest{AgentId: agentName})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %v", err)
	}

	// 4. Test Case: Invalid Token
	ctx = metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer invalid"))
	_, err = c.Report(ctx, &agentv1.ReportRequest{AgentId: agentName})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated for invalid token, got: %v", err)
	}

	// 5. Test Case: Valid Report
	ctx = metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))

	ts := time.Now().Unix()
	req := &agentv1.ReportRequest{
		AgentId:          agentName,
		ClusterId:        "cluster-1",
		TimestampSeconds: ts,
		Snapshot: &agentv1.Snapshot{
			TimestampSeconds: ts,
			Namespaces: []*agentv1.NamespaceCostRecord{
				{Namespace: "default", HourlyCost: 1.5},
			},
		},
	}

	resp, err := c.Report(ctx, req)
	if err != nil {
		t.Fatalf("Report failed: %v", err)
	}
	if !resp.Accepted {
		t.Errorf("Report not accepted: %s", resp.ErrorMessage)
	}

	// 5b. Test Case: Valid Report with Default Token
	ctx = metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+defaultToken))
	reqDefault := &agentv1.ReportRequest{
		AgentId:          unknownAgent,
		ClusterId:        "cluster-2",
		TimestampSeconds: ts,
		Snapshot: &agentv1.Snapshot{
			TimestampSeconds: ts,
		},
	}
	resp, err = c.Report(ctx, reqDefault)
	if err != nil {
		t.Fatalf("Default token Report failed: %v", err)
	}
	if !resp.Accepted {
		t.Errorf("Default token Report not accepted: %s", resp.ErrorMessage)
	}

	// 6. Verify Ingestor received reports
	expectReports := 2
	for i := 0; i < expectReports; i++ {
		select {
		case <-ingestor.received:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected report %d/%d to be ingested", i+1, expectReports)
		}
	}
}
