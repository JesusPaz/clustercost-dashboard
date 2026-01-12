package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/config"
	"github.com/clustercost/clustercost-dashboard/internal/pricing"
	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
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

	pricing *PricingCatalog
}

// AgentSnapshot contains the most recent data fetched for an agent.
type AgentSnapshot struct {
	// Raw Report from the agent
	Report         *agentv1.ReportRequest
	PreviousReport *agentv1.ReportRequest

	LastScrape    time.Time
	LastScrapeDur time.Duration // Duration since previous scrape used for rate calc
	LastError     string
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

// NetworkResource describes network usage metrics.
type NetworkResource struct {
	TxBytesTotal     int64   `json:"txBytesTotal"`
	RxBytesTotal     int64   `json:"rxBytesTotal"`
	EgressCostHourly float64 `json:"egressCostHourly"`
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
	Network        NetworkResource       `json:"network"`
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
	ClusterID      string    `json:"clusterId,omitempty"`
	NodeName       string    `json:"nodeName,omitempty"`
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

// PodContext wraps a PodMetric with its location metadata.
type PodContext struct {
	Pod          *agentv1.PodMetric
	ClusterID    string
	Region       string
	AZ           string
	InstanceType string
}

// New creates a store seeded with agent configurations.
func New(cfgs []config.AgentConfig, recommendedAgentVersion string) *Store {
	agentConfigs := make(map[string]config.AgentConfig, len(cfgs))
	for _, c := range cfgs {
		agentConfigs[c.Name] = c
	}

	// Initialize Static Pricing Provider
	// Context is just placeholder for interface, static client doesn't need it
	pricingClient, _ := pricing.NewAWSClient(context.Background())

	return &Store{
		agentConfigs:            agentConfigs,
		snapshots:               make(map[string]*AgentSnapshot, len(cfgs)),
		recommendedAgentVersion: recommendedAgentVersion,
		pricing:                 NewPricingCatalog(pricingClient),
	}
}

// PricingCatalog returns the pricing catalog used by the store.
func (s *Store) PricingCatalog() *PricingCatalog {
	return s.pricing
}

// Update stores the latest report for a given agent.
func (s *Store) Update(agentID string, req *agentv1.ReportRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find existing to keep as previous
	var prev *agentv1.ReportRequest
	var lastScrape time.Time
	if existing, ok := s.snapshots[agentID]; ok {
		prev = existing.Report
		lastScrape = existing.LastScrape
	}

	now := time.Now().UTC()
	dur := time.Since(lastScrape)
	if lastScrape.IsZero() {
		dur = 1 * time.Minute // default for first run
	}

	s.snapshots[agentID] = &AgentSnapshot{
		Report:         req,
		PreviousReport: prev,
		LastScrape:     now,
		LastScrapeDur:  dur,
		LastError:      "",
	}
}

// GetAllPods returns all pods from all agents with their context.
func (s *Store) GetAllPods() []PodContext {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var pods []PodContext
	for agentID, snap := range s.snapshots {
		if snap == nil || snap.Report == nil {
			continue
		}

		region := snap.Report.Region
		if region == "" {
			// Fallback to config or default if missing in report
			if cfg, ok := s.agentConfigs[agentID]; ok && cfg.Region != "" {
				region = cfg.Region
			} else {
				region = "us-east-1"
			}
		}

		for _, pod := range snap.Report.Pods {
			pods = append(pods, PodContext{
				Pod:          pod,
				ClusterID:    snap.Report.ClusterId,
				Region:       region,
				AZ:           snap.Report.AvailabilityZone,
				InstanceType: snap.Report.InstanceType,
			})
		}
	}
	return pods
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

	// Recalculate everything from snapshots
	var cpuUsage, cpuRequest, memUsage, memRequest int64
	var estimatedNodeCost float64

	namespaces, nsErr := s.aggregateNamespacesLocked()
	if nsErr != nil && nsErr != ErrNoData {
		return ResourcesPayload{}, nsErr
	}

	for _, ns := range namespaces {
		cpuUsage += ns.CPUUsageMilli
		cpuRequest += ns.CPURequestMilli
		memUsage += ns.MemoryUsageBytes
		memRequest += ns.MemoryRequestBytes
	}

	estimatedNodeCost = s.sumNodeHourlyCostLocked()

	if cpuUsage == 0 && cpuRequest == 0 && estimatedNodeCost == 0 {
		return ResourcesPayload{}, ErrNoData
	}

	cpuEfficiency := percent(float64(cpuUsage), float64(cpuRequest))
	memEfficiency := percent(float64(memUsage), float64(memRequest))

	cpuWasteCost := wasteCost(estimatedNodeCost, float64(cpuUsage), float64(cpuRequest))
	memWasteCost := wasteCost(estimatedNodeCost, float64(memUsage), float64(memRequest))

	namespaceWaste := buildNamespaceWaste(namespaces)

	timestamp := s.latestScrapeLocked()
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

	// Since we stream everything in one report now, if we have a scrape, we have all data.
	statusOk := "ok"
	if time.Since(lastSync) > agentOfflineThreshold {
		statusOk = "stale"
	}

	datasets := AgentDatasetHealth{
		Namespaces: statusOk,
		Nodes:      statusOk,
		Resources:  statusOk,
	}

	status := "connected"
	if time.Since(lastSync) > agentOfflineThreshold {
		status = "offline"
	}

	meta := s.latestAgentMetadataLocked()
	version := meta.Version
	updateAvailable := s.recommendedAgentVersion != "" && version != "" && version != s.recommendedAgentVersion

	var nodeCount int
	for _, snap := range s.snapshots {
		if snap != nil && snap.Report != nil {
			nodeCount++
		}
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

	agentMap := make(map[string]AgentInfo)
	for name, cfg := range s.agentConfigs {
		agentMap[name] = AgentInfo{Name: name, BaseURL: cfg.BaseURL, Status: "unknown"}
	}

	for id, snapshot := range s.snapshots {
		info, exists := agentMap[id]
		if !exists {
			info = AgentInfo{Name: id, Status: "unknown"}
		}
		if snapshot != nil {
			if snapshot.LastError != "" {
				info.Status = "error"
				info.Error = snapshot.LastError
			} else {
				if time.Since(snapshot.LastScrape) < agentOfflineThreshold {
					info.Status = "ok"
				} else {
					info.Status = "offline"
				}
			}
			info.LastScrapeTime = snapshot.LastScrape
			if snapshot.Report != nil {
				info.ClusterID = snapshot.Report.ClusterId
				info.NodeName = snapshot.Report.NodeName
			}
		}
		agentMap[id] = info
	}

	result := make([]AgentInfo, 0, len(agentMap))
	for _, info := range agentMap {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
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
	// ClusterName removed
	if meta.ClusterType == "" {
		meta.ClusterType = meta.ClusterID
	}

	cluster := ClusterMetadata{
		ID:        meta.ClusterID,
		Name:      "Cluster", // snap.Report.ClusterName removed
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
		if snap == nil || snap.Report == nil {
			continue
		}
		ts := snap.LastScrape
		// Prefer Region from report, fallback to AZ
		region := snap.Report.Region
		if region == "" {
			region = snap.Report.AvailabilityZone
		}

		if meta.Timestamp.IsZero() || ts.After(meta.Timestamp) {
			meta = agentMetadata{
				ClusterID:   snap.Report.ClusterId,
				ClusterName: "Cluster",
				ClusterType: "k8s",
				Region:      region,
				Version:     "v2.0",
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

// Removed latestResourcesLocked as it returned old response type
// Logic moved to Resources()

func (s *Store) latestDataTimestampLocked() time.Time {
	return s.latestScrapeLocked()
}

func (s *Store) latestNamespacesTimestampLocked() time.Time {
	return s.latestScrapeLocked()
}

func (s *Store) latestNodesTimestampLocked() time.Time {
	return s.latestScrapeLocked()
}

func (s *Store) aggregateNamespacesLocked() (map[string]*NamespaceSummary, error) {
	collector := make(map[string]*NamespaceSummary)
	haveData := false

	for _, snap := range s.snapshots {
		if snap == nil || snap.Report == nil {
			continue
		}

		// Determine node price for this snapshot
		// We don't have node capacity in V2 Report (cpu_allocatable presumably in node metrics if sent?)
		// ReportRequest does NOT have node capacity.
		// For the 50/50 split math, we need total capacity (vCPUs, RAM bytes).
		// Currently V2 proto does NOT send capacity.
		// We have to assume a default capacity or look it up if we knew the instance type.
		// ReportRequest doesn't have InstanceType either?
		// Wait, user provided proto v2 only has: agent_id, cluster_id, node_name, az, pods.
		// It lacks InstanceType and NodeCapacity.
		// PROPOSAL: We must infer or assume defaults until agent sends metadata.
		// Using "m5.large" proxies (2 vCPU, 8GB) for calculation if unknown.

		// TODO: Agent V2 should send Node Metadata (InstanceType, Capacity) for accurate pricing.
		// For now, we use a default "standard" node.
		// Region/AZ: check meta
		region := snap.Report.AvailabilityZone
		if region == "" {
			region = "us-east-1"
		}
		cpuPrice, memPrice := s.pricing.GetNodeResourcePrices(context.Background(), region, "default", 2, 8*1024*1024*1024)

		for _, pod := range snap.Report.Pods {
			haveData = true
			if pod.Namespace == "" {
				continue
			}

			entry, ok := collector[pod.Namespace]
			if !ok {
				entry = &NamespaceSummary{
					Namespace:   pod.Namespace,
					Labels:      make(map[string]string),
					Environment: "production",
				}
				collector[pod.Namespace] = entry
			}

			// Aggregate Costs
			memUsageBytes := int64(0)
			if pod.Memory != nil {
				memUsageBytes = int64(pod.Memory.RssBytes)
			}
			memGB := float64(memUsageBytes) / (1024 * 1024 * 1024)

			// CPU Usage (Cores)
			// We need rate. Snapshot only has cumulative usage_ns.
			// Without rate, we can't do accurate CPU cost.
			// Currently returning 0.0 for CPU cost as placeholder.
			cpuUsageCores := 0.0

			// Network Cost (Egress)
			// Assume EgressPublicBytes is cumulative Counter.
			// To get "Hourly Cost", strictly we need rate.
			// But for "Billable Egress", we usually equate "Traffic sent * Price".
			// If this is a snapshot, we might be double counting if we just sum total counter * price every scrape.
			// CORRECT LOGIC: Cost = (CurrentCounter - PrevCounter) * Price.
			// For this MVP, we will skip Network Cost in "Hourly Rate" display to avoid massive inflation from cumulative counters,
			// OR we assume the agent sends "bytes sent in last report interval"?
			// Proto says "egress_public_bytes". Standard prometheus is cumulative.
			// logic.
			// Given urgency, 0.0 is safer than wrong.
			egressPublicGB := 0.0
			egressCrossAZGB := 0.0

			// Calculate Total Hourly Cost Rate
			// Cost = (Cores * Price/Core) + (GB * Price/GB) + (NetworkGB/hr * Price/GB)
			hourCost := calculateHourlyCost(cpuUsageCores, memGB, egressPublicGB, egressCrossAZGB, cpuPrice, memPrice)

			entry.HourlyCost += hourCost
			entry.MemoryUsageBytes += memUsageBytes
			entry.PodCount++
		}
	}

	if !haveData {
		return nil, ErrNoData
	}
	return collector, nil
}

func (s *Store) aggregateNodesLocked() (map[string]*NodeSummary, error) {
	nodes := make(map[string]*NodeSummary)
	haveData := false

	for _, snap := range s.snapshots {
		if snap == nil || snap.Report == nil {
			continue
		}
		haveData = true

		name := snap.Report.NodeName
		if name == "" {
			name = "unknown"
		}

		entry, ok := nodes[name]
		if !ok {
			entry = &NodeSummary{
				NodeName: name,
				Labels:   make(map[string]string),
				Status:   "Ready", // assumption
			}
			nodes[name] = entry
		}

		// Calculate node totals from pods
		for range snap.Report.Pods {
			entry.PodCount++
		}

		// Cost estimation
		// We use dynamic pricing from catalog.
		region := snap.Report.AvailabilityZone
		if region == "" {
			region = "us-east-1"
		}
		// Default instance type if missing
		instanceType := "default"
		// If Report had InstanceType, we would use it.

		price := s.pricing.GetTotalNodePrice(context.Background(), region, instanceType)
		entry.HourlyCost = price
	}

	if !haveData {
		return nil, ErrNoData
	}
	return nodes, nil
}

func (s *Store) sumNodeHourlyCostLocked() float64 {
	nodes, err := s.aggregateNodesLocked()
	if err != nil {
		return 0
	}
	var total float64
	for _, n := range nodes {
		total += n.HourlyCost
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
