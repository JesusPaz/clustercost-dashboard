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
	logLevel string
}

type ReportIngestor interface {
	EnqueueMetrics(agentName string, req *agentv1.MetricsReportRequest) bool
	EnqueueNetwork(agentName string, req *agentv1.NetworkReportRequest) bool
}

func NewCollector(ingestor ReportIngestor, st *store.Store, logLevel string) *Collector {
	return &Collector{
		ingestor: ingestor,
		store:    st,
		logLevel: logLevel,
	}
}

func (c *Collector) ReportMetrics(ctx context.Context, req *agentv1.MetricsReportRequest) (*agentv1.ReportResponse, error) {
	if c.logLevel == "debug" {
		log.Printf("[DEBUG-GRPC] Received Metrics Report from agent %s. Pods: %d, Nodes: %d", req.AgentId, len(req.Pods), len(req.Nodes))
	}

	if err := c.processMetricsReport(req); err != nil {
		log.Printf("Failed to process metrics report from agent %s: %v", req.AgentId, err)
		return &agentv1.ReportResponse{
			Accepted:     false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &agentv1.ReportResponse{Accepted: true}, nil
}

func (c *Collector) ReportNetwork(ctx context.Context, req *agentv1.NetworkReportRequest) (*agentv1.ReportResponse, error) {
	if c.logLevel == "debug" {
		log.Printf("[DEBUG-GRPC] Received Network Report from agent %s. Endpoints: %d, Connections: %d", req.AgentId, len(req.Endpoints), len(req.CompactConnections))
	}

	if err := c.processNetworkReport(req); err != nil {
		log.Printf("Failed to process network report from agent %s: %v", req.AgentId, err)
		return &agentv1.ReportResponse{
			Accepted:     false,
			ErrorMessage: err.Error(),
		}, nil
	}
	return &agentv1.ReportResponse{Accepted: true}, nil
}

func (c *Collector) processMetricsReport(req *agentv1.MetricsReportRequest) error {
	agentName := req.AgentId
	if agentName == "" {
		return fmt.Errorf("missing agent_id")
	}

	if c.ingestor != nil {
		if ok := c.ingestor.EnqueueMetrics(agentName, req); !ok {
			return fmt.Errorf("ingest queue full")
		}
	}

	if c.store != nil {
		c.store.UpdateMetrics(agentName, req)
	}

	return nil
}

func (c *Collector) processNetworkReport(req *agentv1.NetworkReportRequest) error {
	agentName := req.AgentId
	if agentName == "" {
		return fmt.Errorf("missing agent_id")
	}

	if c.ingestor != nil {
		if ok := c.ingestor.EnqueueNetwork(agentName, req); !ok {
			return fmt.Errorf("ingest queue full")
		}
	}

	if c.store != nil {
		c.store.UpdateNetwork(agentName, req)
	}

	return nil
}
