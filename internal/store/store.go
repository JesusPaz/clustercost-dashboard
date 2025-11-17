package store

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/agents"
	"github.com/clustercost/clustercost-dashboard/internal/config"
)

// ErrNoData indicates that the store has not ingested any data yet.
var ErrNoData = errors.New("no data available")

const hoursPerMonth = 24 * 30
const datasetFreshThreshold = 2 * time.Minute
const agentOfflineThreshold = 5 * time.Minute

var environments = []string{"production", "nonprod", "system", "unknown"}

// Store keeps the latest snapshots retrieved from agents.
type Store struct {
	mu                      sync.RWMutex
	agentConfigs            map[string]config.AgentConfig
	snapshots               map[string]*AgentSnapshot
	recommendedAgentVersion string
}

// AgentSnapshot contains the most recent data fetched for an agent.
type AgentSnapshot struct {
	Health     *agents.HealthResponse
	Namespaces *agents.NamespacesResponse
	Nodes      *agents.NodesResponse
	Resources  *agents.ResourcesResponse
	LastScrape time.Time
	LastError  string
}

// OverviewPayload matches the payload served by /api/cost/overview.
type OverviewPayload struct {
	ClusterName         string              `json:"clusterName"`
	Timestamp           time.Time           `json:"timestamp"`
	TotalHourlyCost     float64             `json:"totalHourlyCost"`
	TotalMonthlyCost    float64             `json:"totalMonthlyCost"`
	EnvCostHourly       map[string]float64  `json:"envCostHourly"`
	TopNamespacesByCost []TopNamespaceEntry `json:"topNamespacesByCost"`
	SavingsCandidates   []SavingsCandidate  `json:"savingsCandidates"`
}

// TopNamespaceEntry highlights the most expensive namespaces.
type TopNamespaceEntry struct {
	Namespace   string  `json:"namespace"`
	Environment string  `json:"environment"`
	HourlyCost  float64 `json:"hourlyCost"`
}

// SavingsCandidate contains high cost / low usage namespace information.
type SavingsCandidate struct {
	Namespace          string  `json:"namespace"`
	Environment        string  `json:"environment"`
	HourlyCost         float64 `json:"hourlyCost"`
	CPURequestMilli    int64   `json:"cpuRequestMilli"`
	CPUUsageMilli      int64   `json:"cpuUsageMilli"`
	MemoryRequestBytes int64   `json:"memoryRequestBytes"`
	MemoryUsageBytes   int64   `json:"memoryUsageBytes"`
}

// NamespaceSummary contains the normalized view returned by /api/cost/namespaces.
type NamespaceSummary struct {
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

// NamespaceListResponse wraps paginated namespaces results.
type NamespaceListResponse struct {
	Items      []NamespaceSummary `json:"items"`
	TotalCount int                `json:"totalCount"`
	Timestamp  time.Time          `json:"timestamp"`
}

// NodeSummary mirrors the nodes API output.
type NodeSummary struct {
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
	Taints                 []string          `json:"taints"`
}

// NodeListResponse wraps paginated node results.
type NodeListResponse struct {
	Items      []NodeSummary `json:"items"`
	TotalCount int           `json:"totalCount"`
	Timestamp  time.Time     `json:"timestamp"`
}

// CPUResource describes CPU efficiency metrics.
type CPUResource struct {
	UsageMilli               int64   `json:"usageMilli"`
	RequestMilli             int64   `json:"requestMilli"`
	EfficiencyPercent        float64 `json:"efficiencyPercent"`
	EstimatedHourlyWasteCost float64 `json:"estimatedHourlyWasteCost"`
}

// MemoryResource describes memory efficiency metrics.
type MemoryResource struct {
	UsageBytes               int64   `json:"usageBytes"`
	RequestBytes             int64   `json:"requestBytes"`
	EfficiencyPercent        float64 `json:"efficiencyPercent"`
	EstimatedHourlyWasteCost float64 `json:"estimatedHourlyWasteCost"`
}

// NamespaceWasteEntry highlights inefficient namespaces.
type NamespaceWasteEntry struct {
	Namespace                string  `json:"namespace"`
	Environment              string  `json:"environment"`
	CPUWastePercent          float64 `json:"cpuWastePercent"`
	MemoryWastePercent       float64 `json:"memoryWastePercent"`
	EstimatedHourlyWasteCost float64 `json:"estimatedHourlyWasteCost"`
}

// ResourcesPayload powers the /api/cost/resources endpoint.
type ResourcesPayload struct {
	Timestamp      time.Time             `json:"timestamp"`
	CPU            CPUResource           `json:"cpu"`
	Memory         MemoryResource        `json:"memory"`
	NamespaceWaste []NamespaceWasteEntry `json:"namespaceWaste"`
}

// AgentDatasetHealth summarizes data availability per dataset.
type AgentDatasetHealth struct {
	Namespaces string `json:"namespaces"`
	Nodes      string `json:"nodes"`
	Resources  string `json:"resources"`
}

// AgentStatusPayload powers the agent status endpoint.
type AgentStatusPayload struct {
	Status          string             `json:"status"`
	LastSync        time.Time          `json:"lastSync"`
	Datasets        AgentDatasetHealth `json:"datasets"`
	Version         string             `json:"version,omitempty"`
	UpdateAvailable bool               `json:"updateAvailable"`
	ClusterName     string             `json:"clusterName,omitempty"`
	ClusterType     string             `json:"clusterType,omitempty"`
	ClusterRegion   string             `json:"clusterRegion,omitempty"`
	NodeCount       int                `json:"nodeCount"`
}

// ClusterMetadata captures the latest cluster identity details known to the dashboard.
type ClusterMetadata struct {
	ID        string
	Name      string
	Type      string
	Region    string
	Version   string
	Timestamp time.Time
}

// AgentInfo is exposed on /api/agents and referenced by health.
type AgentInfo struct {
	Name           string    `json:"name"`
	BaseURL        string    `json:"baseUrl"`
	Status         string    `json:"status"`
	LastScrapeTime time.Time `json:"lastScrapeTime"`
	Error          string    `json:"error,omitempty"`
}

// NamespaceFilter controls namespaces list filtering.
type NamespaceFilter struct {
	Environment string
	Search      string
	Limit       int
	Offset      int
}

// NodeFilter controls nodes list filtering.
type NodeFilter struct {
	Search string
	Limit  int
	Offset int
}

// New creates a store seeded with agent configurations.
func New(cfgs []config.AgentConfig, recommendedAgentVersion string) *Store {
	agentConfigs := make(map[string]config.AgentConfig, len(cfgs))
	for _, c := range cfgs {
		agentConfigs[c.Name] = c
	}
	return &Store{
		agentConfigs:            agentConfigs,
		snapshots:               make(map[string]*AgentSnapshot, len(cfgs)),
		recommendedAgentVersion: recommendedAgentVersion,
	}
}

// Update stores the latest snapshot for a given agent.
func (s *Store) Update(name string, snapshot AgentSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copySnapshot := snapshot
	s.snapshots[name] = &copySnapshot
}

// Overview aggregates cluster level information for the overview dashboard.
func (s *Store) Overview(limit int) (OverviewPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	namespaces, err := s.aggregateNamespacesLocked()
	if err != nil {
		return OverviewPayload{}, err
	}

	timestamp := s.latestDataTimestampLocked()
	if timestamp.IsZero() {
		timestamp = s.latestScrapeLocked()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	clusterMeta := s.latestAgentMetadataLocked()
	clusterID := clusterMeta.ClusterID

	envCost := make(map[string]float64, len(environments))
	for _, env := range environments {
		envCost[env] = 0
	}

	totalHourly := 0.0
	list := make([]NamespaceSummary, 0, len(namespaces))
	for _, ns := range namespaces {
		totalHourly += ns.HourlyCost
		list = append(list, *ns)
		if _, ok := envCost[ns.Environment]; ok {
			envCost[ns.Environment] += ns.HourlyCost
		} else {
			envCost["unknown"] += ns.HourlyCost
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].HourlyCost > list[j].HourlyCost
	})

	topLimit := limit
	if topLimit <= 0 || topLimit > len(list) {
		topLimit = len(list)
	}
	topNamespaces := make([]TopNamespaceEntry, 0, topLimit)
	for idx := 0; idx < topLimit; idx++ {
		topNamespaces = append(topNamespaces, TopNamespaceEntry{
			Namespace:   list[idx].Namespace,
			Environment: valueOrDefault(list[idx].Environment, "unknown"),
			HourlyCost:  list[idx].HourlyCost,
		})
	}

	candidates := findSavingsCandidates(list)

	return OverviewPayload{
		ClusterName:         clusterID,
		Timestamp:           timestamp,
		TotalHourlyCost:     totalHourly,
		TotalMonthlyCost:    totalHourly * hoursPerMonth,
		EnvCostHourly:       envCost,
		TopNamespacesByCost: topNamespaces,
		SavingsCandidates:   candidates,
	}, nil
}

// NamespaceList returns filtered namespaces with pagination.
func (s *Store) NamespaceList(filter NamespaceFilter) (NamespaceListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	namespaces, err := s.aggregateNamespacesLocked()
	if err != nil {
		return NamespaceListResponse{}, err
	}

	var searchLower string
	if filter.Search != "" {
		searchLower = strings.ToLower(filter.Search)
	}

	out := make([]NamespaceSummary, 0, len(namespaces))
	for _, ns := range namespaces {
		if !environmentMatches(filter.Environment, ns.Environment) {
			continue
		}
		if searchLower != "" && !strings.Contains(strings.ToLower(ns.Namespace), searchLower) {
			continue
		}
		out = append(out, *ns)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].HourlyCost > out[j].HourlyCost
	})

	total := len(out)
	start := clampIndex(filter.Offset, total)
	end := clampIndex(filter.Offset+filter.Limit, total)

	timestamp := s.latestNamespacesTimestampLocked()
	if timestamp.IsZero() {
		timestamp = s.latestScrapeLocked()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	return NamespaceListResponse{
		Items:      out[start:end],
		TotalCount: total,
		Timestamp:  timestamp,
	}, nil
}

// NamespaceDetail returns a single namespace entry if present.
func (s *Store) NamespaceDetail(name string) (NamespaceSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	namespaces, err := s.aggregateNamespacesLocked()
	if err != nil {
		return NamespaceSummary{}, err
	}
	ns, ok := namespaces[name]
	if !ok {
		return NamespaceSummary{}, ErrNoData
	}
	return *ns, nil
}

// NodeList returns filtered nodes with pagination.
func (s *Store) NodeList(filter NodeFilter) (NodeListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes, err := s.aggregateNodesLocked()
	if err != nil {
		return NodeListResponse{}, err
	}

	var searchLower string
	if filter.Search != "" {
		searchLower = strings.ToLower(filter.Search)
	}

	out := make([]NodeSummary, 0, len(nodes))
	for _, node := range nodes {
		if searchLower != "" && !strings.Contains(strings.ToLower(node.NodeName), searchLower) {
			continue
		}
		out = append(out, *node)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].HourlyCost > out[j].HourlyCost
	})

	total := len(out)
	start := clampIndex(filter.Offset, total)
	end := clampIndex(filter.Offset+filter.Limit, total)

	timestamp := s.latestNodesTimestampLocked()
	if timestamp.IsZero() {
		timestamp = s.latestScrapeLocked()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	return NodeListResponse{
		Items:      out[start:end],
		TotalCount: total,
		Timestamp:  timestamp,
	}, nil
}

// NodeDetail returns a single node entry if present.
func (s *Store) NodeDetail(name string) (NodeSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes, err := s.aggregateNodesLocked()
	if err != nil {
		return NodeSummary{}, err
	}
	node, ok := nodes[name]
	if !ok {
		return NodeSummary{}, ErrNoData
	}
	return *node, nil
}

// Resources aggregates cluster efficiency data.
func (s *Store) Resources() (ResourcesPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources, resourcesTime := s.latestResourcesLocked()

	namespaces, nsErr := s.aggregateNamespacesLocked()
	if nsErr != nil && nsErr != ErrNoData {
		return ResourcesPayload{}, nsErr
	}
	if nsErr == ErrNoData {
		namespaces = nil
	}

	if resources == nil && len(namespaces) == 0 {
		return ResourcesPayload{}, ErrNoData
	}

	var cpuUsage, cpuRequest, memUsage, memRequest int64
	var estimatedNodeCost float64

	if resources != nil {
		cpuUsage = resources.Snapshot.CPUUsageMilliTotal
		cpuRequest = resources.Snapshot.CPURequestMilliTotal
		memUsage = resources.Snapshot.MemoryUsageBytesTotal
		memRequest = resources.Snapshot.MemoryRequestBytesTotal
		estimatedNodeCost = resources.Snapshot.TotalNodeHourlyCost
	}

	if cpuUsage == 0 && cpuRequest == 0 && len(namespaces) > 0 {
		for _, ns := range namespaces {
			cpuUsage += ns.CPUUsageMilli
			cpuRequest += ns.CPURequestMilli
		}
	}
	if memUsage == 0 && memRequest == 0 && len(namespaces) > 0 {
		for _, ns := range namespaces {
			memUsage += ns.MemoryUsageBytes
			memRequest += ns.MemoryRequestBytes
		}
	}

	if estimatedNodeCost == 0 {
		estimatedNodeCost = s.sumNodeHourlyCostLocked()
	}

	cpuEfficiency := percent(float64(cpuUsage), float64(cpuRequest))
	memEfficiency := percent(float64(memUsage), float64(memRequest))

	cpuWasteCost := wasteCost(estimatedNodeCost, float64(cpuUsage), float64(cpuRequest))
	memWasteCost := wasteCost(estimatedNodeCost, float64(memUsage), float64(memRequest))

	namespaceWaste := buildNamespaceWaste(namespaces)

	timestamp := resourcesTime
	if timestamp.IsZero() {
		timestamp = s.latestScrapeLocked()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	return ResourcesPayload{
		Timestamp: timestamp,
		CPU: CPUResource{
			UsageMilli:               cpuUsage,
			RequestMilli:             cpuRequest,
			EfficiencyPercent:        cpuEfficiency,
			EstimatedHourlyWasteCost: cpuWasteCost,
		},
		Memory: MemoryResource{
			UsageBytes:               memUsage,
			RequestBytes:             memRequest,
			EfficiencyPercent:        memEfficiency,
			EstimatedHourlyWasteCost: memWasteCost,
		},
		NamespaceWaste: namespaceWaste,
	}, nil
}

// AgentStatus returns a simplified view of agent health and data completeness.
func (s *Store) AgentStatus() (AgentStatusPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lastSync := s.latestScrapeLocked()
	if lastSync.IsZero() {
		return AgentStatusPayload{}, ErrNoData
	}

	nsTimestamp := s.latestNamespacesTimestampLocked()
	nodeTimestamp := s.latestNodesTimestampLocked()
	resourcesSnapshot, resourcesTimestamp := s.latestResourcesLocked()

	datasets := AgentDatasetHealth{
		Namespaces: datasetStatus(!nsTimestamp.IsZero(), nsTimestamp, lastSync),
		Nodes:      datasetStatus(!nodeTimestamp.IsZero(), nodeTimestamp, lastSync),
		Resources:  datasetStatus(resourcesSnapshot != nil, resourcesTimestamp, lastSync),
	}

	status := "offline"
	allOK := datasets.Namespaces == "ok" && datasets.Nodes == "ok" && datasets.Resources == "ok"
	if !lastSync.IsZero() {
		if time.Since(lastSync) > agentOfflineThreshold {
			status = "offline"
		} else if allOK {
			status = "connected"
		} else {
			status = "partial"
		}
	}

	meta := s.latestAgentMetadataLocked()
	version := meta.Version
	updateAvailable := s.recommendedAgentVersion != "" && version != "" && version != s.recommendedAgentVersion

	var nodeCount int
	if nodes, err := s.aggregateNodesLocked(); err == nil {
		nodeCount = len(nodes)
	}

	return AgentStatusPayload{
		Status:          status,
		LastSync:        lastSync,
		Datasets:        datasets,
		Version:         version,
		UpdateAvailable: updateAvailable,
		ClusterName:     meta.ClusterName,
		ClusterType:     meta.ClusterType,
		ClusterRegion:   meta.Region,
		NodeCount:       nodeCount,
	}, nil
}

// Agents returns metadata about configured agents and their latest status.
func (s *Store) Agents() []AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]AgentInfo, 0, len(s.agentConfigs))
	for name, cfg := range s.agentConfigs {
		snapshot := s.snapshots[name]
		info := AgentInfo{
			Name:    name,
			BaseURL: cfg.BaseURL,
			Status:  "unknown",
		}
		if snapshot != nil {
			if snapshot.LastError != "" {
				info.Status = "error"
				info.Error = snapshot.LastError
			} else if snapshot.Health != nil {
				info.Status = snapshot.Health.Status
			} else {
				info.Status = "stale"
			}
			info.LastScrapeTime = snapshot.LastScrape
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ClusterMetadata returns the known cluster metadata and latest timestamp.
func (s *Store) ClusterMetadata() (ClusterMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	meta := s.latestAgentMetadataLocked()
	ts := s.latestDataTimestampLocked()
	if ts.IsZero() {
		ts = meta.Timestamp
	}
	if ts.IsZero() {
		ts = s.latestScrapeLocked()
	}

	if meta.ClusterName == "" {
		meta.ClusterName = meta.ClusterID
	}
	if meta.ClusterType == "" {
		meta.ClusterType = meta.ClusterID
	}

	cluster := ClusterMetadata{
		ID:        meta.ClusterID,
		Name:      meta.ClusterName,
		Type:      meta.ClusterType,
		Region:    meta.Region,
		Version:   meta.Version,
		Timestamp: ts,
	}

	if ts.IsZero() {
		return cluster, ErrNoData
	}
	return cluster, nil
}

func (s *Store) latestScrapeLocked() time.Time {
	var latest time.Time
	for _, snap := range s.snapshots {
		if snap == nil {
			continue
		}
		if snap.LastScrape.After(latest) {
			latest = snap.LastScrape
		}
	}
	return latest
}

type agentMetadata struct {
	ClusterID   string
	ClusterName string
	ClusterType string
	Region      string
	Version     string
	Timestamp   time.Time
}

func (s *Store) latestAgentMetadataLocked() agentMetadata {
	var meta agentMetadata
	for _, snap := range s.snapshots {
		if snap == nil || snap.Health == nil {
			continue
		}
		ts := snap.Health.Timestamp
		if ts.IsZero() {
			ts = snap.LastScrape
		}
		if meta.Timestamp.IsZero() || ts.After(meta.Timestamp) {
			meta = agentMetadata{
				ClusterID:   snap.Health.ClusterID,
				ClusterName: snap.Health.ClusterName,
				ClusterType: snap.Health.ClusterType,
				Region:      snap.Health.Region,
				Version:     snap.Health.Version,
				Timestamp:   ts,
			}
		}
	}
	if meta.ClusterName == "" {
		meta.ClusterName = meta.ClusterID
	}
	if meta.ClusterType == "" {
		meta.ClusterType = meta.ClusterID
	}
	return meta
}

func (s *Store) latestResourcesLocked() (*agents.ResourcesResponse, time.Time) {
	var latest *agents.ResourcesResponse
	var ts time.Time
	for _, snap := range s.snapshots {
		if snap == nil || snap.Resources == nil {
			continue
		}
		cur := snap.Resources.Timestamp
		if cur.IsZero() {
			cur = snap.LastScrape
		}
		if cur.After(ts) {
			latest = snap.Resources
			ts = cur
		}
	}
	return latest, ts
}

func (s *Store) latestDataTimestampLocked() time.Time {
	var ts time.Time
	for _, snap := range s.snapshots {
		if snap == nil {
			continue
		}
		if snap.Namespaces != nil {
			current := snap.Namespaces.Timestamp
			if current.IsZero() {
				current = snap.LastScrape
			}
			if current.After(ts) {
				ts = current
			}
		}
		if snap.Nodes != nil {
			current := snap.Nodes.Timestamp
			if current.IsZero() {
				current = snap.LastScrape
			}
			if current.After(ts) {
				ts = current
			}
		}
		if snap.Resources != nil {
			current := snap.Resources.Timestamp
			if current.IsZero() {
				current = snap.LastScrape
			}
			if current.After(ts) {
				ts = current
			}
		}
	}
	return ts
}

func (s *Store) latestNamespacesTimestampLocked() time.Time {
	var ts time.Time
	for _, snap := range s.snapshots {
		if snap == nil || snap.Namespaces == nil {
			continue
		}
		current := snap.Namespaces.Timestamp
		if current.IsZero() {
			current = snap.LastScrape
		}
		if current.After(ts) {
			ts = current
		}
	}
	return ts
}

func (s *Store) latestNodesTimestampLocked() time.Time {
	var ts time.Time
	for _, snap := range s.snapshots {
		if snap == nil || snap.Nodes == nil {
			continue
		}
		current := snap.Nodes.Timestamp
		if current.IsZero() {
			current = snap.LastScrape
		}
		if current.After(ts) {
			ts = current
		}
	}
	return ts
}

func (s *Store) aggregateNamespacesLocked() (map[string]*NamespaceSummary, error) {
	collector := make(map[string]*NamespaceSummary)
	haveData := false

	for _, snap := range s.snapshots {
		if snap == nil || snap.Namespaces == nil || len(snap.Namespaces.Items) == 0 {
			continue
		}
		haveData = true
		for _, ns := range snap.Namespaces.Items {
			entry, ok := collector[ns.Namespace]
			if !ok {
				entry = &NamespaceSummary{
					Namespace:   ns.Namespace,
					Environment: valueOrDefault(ns.Environment, "unknown"),
					Labels:      copyLabels(ns.Labels),
				}
				collector[ns.Namespace] = entry
			}
			entry.HourlyCost += ns.HourlyCost
			entry.PodCount += ns.PodCount
			entry.CPURequestMilli += ns.CPURequestMilli
			entry.MemoryRequestBytes += ns.MemoryRequestBytes
			entry.CPUUsageMilli += ns.CPUUsageMilli
			entry.MemoryUsageBytes += ns.MemoryUsageBytes
			if len(entry.Labels) == 0 {
				entry.Labels = copyLabels(ns.Labels)
			}
			if entry.Environment == "" {
				entry.Environment = valueOrDefault(ns.Environment, "unknown")
			}
		}
	}

	if !haveData {
		return nil, ErrNoData
	}

	return collector, nil
}

func (s *Store) aggregateNodesLocked() (map[string]*NodeSummary, error) {
	type nodeEntry struct {
		node        *NodeSummary
		lastUpdated time.Time
	}

	collector := make(map[string]nodeEntry)
	haveData := false

	for _, snap := range s.snapshots {
		if snap == nil || snap.Nodes == nil || len(snap.Nodes.Items) == 0 {
			continue
		}
		haveData = true
		for _, node := range snap.Nodes.Items {
			current := nodeEntry{}
			existing, ok := collector[node.NodeName]
			if ok {
				current = existing
			}
			if !ok || snap.LastScrape.After(current.lastUpdated) {
				current.node = &NodeSummary{
					NodeName:               node.NodeName,
					HourlyCost:             node.HourlyCost,
					CPUUsagePercent:        node.CPUUsagePercent,
					MemoryUsagePercent:     node.MemoryUsagePercent,
					CPUAllocatableMilli:    node.CPUAllocatableMilli,
					MemoryAllocatableBytes: node.MemoryAllocatableBytes,
					PodCount:               node.PodCount,
					Status:                 node.Status,
					IsUnderPressure:        node.IsUnderPressure,
					InstanceType:           node.InstanceType,
					Labels:                 copyLabels(node.Labels),
					Taints:                 copyStrings(node.Taints),
				}
				current.lastUpdated = snap.LastScrape
				collector[node.NodeName] = current
			}
		}
	}

	if !haveData {
		return nil, ErrNoData
	}

	result := make(map[string]*NodeSummary, len(collector))
	for name, entry := range collector {
		result[name] = entry.node
	}
	return result, nil
}

func (s *Store) sumNodeHourlyCostLocked() float64 {
	total := 0.0
	for _, snap := range s.snapshots {
		if snap == nil || snap.Nodes == nil {
			continue
		}
		for _, node := range snap.Nodes.Items {
			total += node.HourlyCost
		}
	}
	return total
}

func findSavingsCandidates(namespaces []NamespaceSummary) []SavingsCandidate {
	const utilizationThreshold = 0.4
	const costThreshold = 0.05

	candidates := make([]SavingsCandidate, 0)
	for _, ns := range namespaces {
		if ns.HourlyCost < costThreshold {
			continue
		}
		cpuRatio := usageRatio(float64(ns.CPUUsageMilli), float64(ns.CPURequestMilli))
		memRatio := usageRatio(float64(ns.MemoryUsageBytes), float64(ns.MemoryRequestBytes))
		if (ns.CPURequestMilli > 0 && cpuRatio <= utilizationThreshold) ||
			(ns.MemoryRequestBytes > 0 && memRatio <= utilizationThreshold) {
			candidates = append(candidates, SavingsCandidate{
				Namespace:          ns.Namespace,
				Environment:        valueOrDefault(ns.Environment, "unknown"),
				HourlyCost:         ns.HourlyCost,
				CPURequestMilli:    ns.CPURequestMilli,
				CPUUsageMilli:      ns.CPUUsageMilli,
				MemoryRequestBytes: ns.MemoryRequestBytes,
				MemoryUsageBytes:   ns.MemoryUsageBytes,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].HourlyCost > candidates[j].HourlyCost
	})

	if len(candidates) > 5 {
		return candidates[:5]
	}
	return candidates
}

func buildNamespaceWaste(data map[string]*NamespaceSummary) []NamespaceWasteEntry {
	if len(data) == 0 {
		return []NamespaceWasteEntry{}
	}

	out := make([]NamespaceWasteEntry, 0, len(data))
	for _, ns := range data {
		cpuWaste := wastePercent(float64(ns.CPUUsageMilli), float64(ns.CPURequestMilli))
		memWaste := wastePercent(float64(ns.MemoryUsageBytes), float64(ns.MemoryRequestBytes))
		if cpuWaste == 0 && memWaste == 0 {
			continue
		}
		wasteRatio := maxFloat(
			unusedRatio(float64(ns.CPUUsageMilli), float64(ns.CPURequestMilli)),
			unusedRatio(float64(ns.MemoryUsageBytes), float64(ns.MemoryRequestBytes)),
		)
		out = append(out, NamespaceWasteEntry{
			Namespace:                ns.Namespace,
			Environment:              valueOrDefault(ns.Environment, "unknown"),
			CPUWastePercent:          cpuWaste,
			MemoryWastePercent:       memWaste,
			EstimatedHourlyWasteCost: ns.HourlyCost * wasteRatio,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].EstimatedHourlyWasteCost > out[j].EstimatedHourlyWasteCost
	})

	if len(out) > 5 {
		return out[:5]
	}
	return out
}

func usageRatio(usage, request float64) float64 {
	if request <= 0 {
		return 0
	}
	return usage / request
}

func percent(num, denom float64) float64 {
	if denom <= 0 {
		return 0
	}
	val := (num / denom) * 100
	if val < 0 {
		return 0
	}
	return val
}

func wastePercent(usage, request float64) float64 {
	if request <= 0 {
		return 0
	}
	waste := (1 - (usage / request)) * 100
	return clampFloat(waste, 0, 100)
}

func unusedRatio(usage, request float64) float64 {
	if request <= 0 {
		return 0
	}
	return clampFloat(1-(usage/request), 0, 1)
}

func wasteCost(nodeCost, usage, request float64) float64 {
	if request <= 0 || nodeCost <= 0 {
		return 0
	}
	return nodeCost * clampFloat(1-(usage/request), 0, 1)
}

func clampIndex(idx, max int) int {
	if idx < 0 {
		return 0
	}
	if idx > max {
		return max
	}
	return idx
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func copyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func environmentMatches(filter, env string) bool {
	if filter == "" || filter == "all" {
		return true
	}
	return strings.EqualFold(filter, env)
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func datasetStatus(hasData bool, timestamp, fallback time.Time) string {
	if !hasData {
		return "missing"
	}
	effective := timestamp
	if effective.IsZero() {
		effective = fallback
	}
	if effective.IsZero() {
		return "partial"
	}
	if time.Since(effective) > datasetFreshThreshold {
		return "partial"
	}
	return "ok"
}
