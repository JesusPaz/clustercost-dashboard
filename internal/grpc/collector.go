package grpc

import (
	"fmt"
	"io"
	"log"

	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
)

type Collector struct {
	agentv1.UnimplementedAgentServiceServer
	ingestor ReportIngestor
}

type ReportIngestor interface {
	Enqueue(agentName string, req *agentv1.ReportRequest) bool
}

func NewCollector(ingestor ReportIngestor) *Collector {
	return &Collector{ingestor: ingestor}
}

func (c *Collector) Report(stream agentv1.AgentService_ReportServer) error {
	// ctx := stream.Context()

	// Optional: Check auth from context once here if needed?
	// But we might want to check agent_id in each message matches auth?

	count := 0
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// Done reading
			return stream.SendAndClose(&agentv1.ReportResponse{Accepted: true})
		}
		if err != nil {
			return err
		}

		if err := c.processReport(req); err != nil {
			log.Printf("Failed to process report from agent %s: %v", req.AgentId, err)
			// Decide: return error and close stream, or just log and continue?
			// Usually strict error handling for ingestion.
			return err
		}
		count++

		// If we processed 100 messages, maybe we valid?
		// We just stream until EOF.
	}
}

func (c *Collector) processReport(req *agentv1.ReportRequest) error {
	// Identify agent.
	agentName := req.AgentId
	if agentName == "" {
		return fmt.Errorf("missing agent_id")
	}

	if c.ingestor == nil {
		return fmt.Errorf("ingestor not configured")
	}

	if ok := c.ingestor.Enqueue(agentName, req); !ok {
		return fmt.Errorf("ingest queue full")
	}
	return nil
}
