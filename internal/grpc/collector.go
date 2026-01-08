package grpc

import (
	"context"
	"fmt"

	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
)

type Collector struct {
	agentv1.UnimplementedCollectorServer
	ingestor ReportIngestor
}

type ReportIngestor interface {
	Enqueue(agentName string, req *agentv1.ReportRequest) bool
}

func NewCollector(ingestor ReportIngestor) *Collector {
	return &Collector{ingestor: ingestor}
}

func (c *Collector) Report(ctx context.Context, req *agentv1.ReportRequest) (*agentv1.ReportResponse, error) {
	if err := c.processReport(ctx, req); err != nil {
		return &agentv1.ReportResponse{
			Accepted:     false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &agentv1.ReportResponse{Accepted: true}, nil
}

func (c *Collector) ReportBatch(ctx context.Context, req *agentv1.ReportBatchRequest) (*agentv1.ReportResponse, error) {
	var lastErr error
	for _, report := range req.Reports {
		if err := c.processReport(ctx, report); err != nil {
			lastErr = err
			// Continue processing other reports?
			// For now, we'll try to process all and return error if any failed.
			// Ideally we should return partial success status, but the proto has simple boolean.
		}
	}

	if lastErr != nil {
		return &agentv1.ReportResponse{
			Accepted:     false,
			ErrorMessage: fmt.Sprintf("some reports failed, last error: %v", lastErr),
		}, nil
	}

	return &agentv1.ReportResponse{Accepted: true}, nil
}

func (c *Collector) processReport(ctx context.Context, req *agentv1.ReportRequest) error {
	// Identify agent.
	// Ideally we get agent name from context (AuthInterceptor), or we trust the agent_id in request.
	// We'll use the agent_id from the request as the key for the store updates.
	// If the auth interceptor put the "agent_name" in context, we could verify it matches or use it.

	agentName := req.AgentId
	if agentName == "" {
		// Fallback to retrieving from context if available, or error
		if name, ok := ctx.Value("agent_name").(string); ok {
			agentName = name
		} else {
			return fmt.Errorf("missing agent_id")
		}
	}

	if c.ingestor == nil {
		return fmt.Errorf("victoria metrics ingestor not configured")
	}
	if ok := c.ingestor.Enqueue(agentName, req); !ok {
		return fmt.Errorf("victoria metrics ingest queue full")
	}
	return nil
}
