package agents

import "time"

// HealthResponse represents the payload returned by the agent health endpoint.
type HealthResponse struct {
	Status      string    `json:"status"`
	ClusterID   string    `json:"clusterId"`
	ClusterName string    `json:"clusterName,omitempty"`
	ClusterType string    `json:"clusterType,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Version     string    `json:"version,omitempty"`
	Region      string    `json:"clusterRegion,omitempty"`
}

// NamespacesResponse contains namespace level cost data plus metadata.
type NamespacesResponse struct {
	Items     []NamespaceCost `json:"items"`
	Timestamp time.Time       `json:"timestamp"`
}

// NamespaceCost contains per-namespace allocation information already aggregated by the agent.
type NamespaceCost struct {
	ClusterID          string            `json:"clusterId"`
	Namespace          string            `json:"namespace"`
	HourlyCost         float64           `json:"hourlyCost"`
	PodCount           int               `json:"podCount"`
	CPURequestMilli    int64             `json:"cpuRequestMilli"`
	MemoryRequestBytes int64             `json:"memoryRequestBytes"`
	CPUUsageMilli      int64             `json:"cpuUsageMilli"`
	MemoryUsageBytes   int64             `json:"memoryUsageBytes"`
	Labels             map[string]string `json:"labels"`
	Environment        string            `json:"environment"`
}

// NodesResponse contains node level cost and utilization data.
type NodesResponse struct {
	Items     []NodeCost `json:"items"`
	Timestamp time.Time  `json:"timestamp"`
}

// NodeCost represents node-level utilization and pricing.
type NodeCost struct {
	ClusterID              string            `json:"clusterId"`
	NodeName               string            `json:"nodeName"`
	HourlyCost             float64           `json:"hourlyCost"`
	CPUUsagePercent        float64           `json:"cpuUsagePercent"`
	MemoryUsagePercent     float64           `json:"memoryUsagePercent"`
	CPUAllocatableMilli    int64             `json:"cpuAllocatableMilli"`
	MemoryAllocatableBytes int64             `json:"memoryAllocatableBytes"`
	PodCount               int               `json:"podCount"`
	Status                 string            `json:"status"`
	IsUnderPressure        bool              `json:"isUnderPressure"`
	InstanceType           string            `json:"instanceType,omitempty"`
	Labels                 map[string]string `json:"labels"`
	Taints                 []string          `json:"taints,omitempty"`
}

// ResourcesResponse contains cluster-wide resource efficiency information.
type ResourcesResponse struct {
	Snapshot  ResourceSnapshot `json:"snapshot"`
	Timestamp time.Time        `json:"timestamp"`
}

// ResourceSnapshot aggregates CPU and memory request/usage totals plus node cost.
type ResourceSnapshot struct {
	CPUUsageMilliTotal      int64   `json:"cpuUsageMilliTotal"`
	CPURequestMilliTotal    int64   `json:"cpuRequestMilliTotal"`
	MemoryUsageBytesTotal   int64   `json:"memoryUsageBytesTotal"`
	MemoryRequestBytesTotal int64   `json:"memoryRequestBytesTotal"`
	TotalNodeHourlyCost     float64 `json:"totalNodeHourlyCost"`
}
