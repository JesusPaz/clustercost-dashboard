package grpc

import (
	"context"
	"fmt"
	"log"

	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
	"github.com/clustercost/clustercost-dashboard/internal/store"
)

type Collector struct {
	agentv1.UnimplementedCollectorServer
	ingestor ReportIngestor
	store    *store.Store
}

type ReportIngestor interface {
	Enqueue(agentName string, req *agentv1.ReportRequest) bool
}

func NewCollector(ingestor ReportIngestor, st *store.Store) *Collector {
	return &Collector{
		ingestor: ingestor,
		store:    st,
	}
}

func (c *Collector) Report(ctx context.Context, req *agentv1.ReportRequest) (*agentv1.ReportResponse, error) {
	if err := c.processReport(req); err != nil {
		log.Printf("Failed to process report from agent %s: %v", req.AgentId, err)
		return &agentv1.ReportResponse{
			Accepted:     false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &agentv1.ReportResponse{Accepted: true}, nil
}

func (c *Collector) processReport(req *agentv1.ReportRequest) error {
	// Identify agent.
	agentName := req.AgentId
	if agentName == "" {
		return fmt.Errorf("missing agent_id")
	}

	if c.ingestor != nil {
		if ok := c.ingestor.Enqueue(agentName, req); !ok {
			// Log warning but don't fail RPC if queue is full?
			// failure to ingest to VM is bad but maybe transient.
			// Ideally we signal backpressure.
			return fmt.Errorf("ingest queue full")
		}
	}

	if c.store != nil {
		c.store.Update(agentName, req)
	}

	return nil
}
