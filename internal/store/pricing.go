package store

// Pricing constants (simplified for this exercise)
const (
	// Default prices if not found in catalog
	DefaultCPUCostPerHour    = 0.031611 // per vCPU
	DefaultMemoryCostPerHour = 0.004237 // per GB

	// Egress costs
	CostEgressPublic   = 0.09 // per GB
	CostEgressCrossAZ  = 0.01 // per GB
	CostEgressInternal = 0.00 // Free
)

// PricingCatalog allows looking up node prices.
// In a real generic version, this might load from a JSON file or API.
type PricingCatalog struct{}

func (pc *PricingCatalog) GetNodePrice(nodeName, zone string) (cpuPerHour, memPerHour float64) {
	// TODO: unique lookups based on instance types if we had them.
	// For now, return standard cloud defaults.
	return DefaultCPUCostPerHour, DefaultMemoryCostPerHour
}

func calculatePodCost(cpuSeconds, memByteSeconds, publicBytes, crossAZBytes, internalBytes float64, cpuPrice, memPrice float64) float64 {
	// This function might need to be adapted depending on what metrics we receive:
	// The proto CpuMetrics has usage_user_ns (nanoseconds).
	// The proto MemoryMetrics has rss_bytes.

	// BUT wait, "Hourly Cost" is usually Rate * Usage.
	// If we are showing "Hourly Cost" as a *rate* based on current usage:
	// Cost/Hr = (Cores_Used * Price/Core/Hr) + (GB_Used * Price/GB/Hr)

	// Network cost is usually per GB. So "Hourly Cost" for network implies we infer a rate?
	// Or do we just sum up the costs for the period?
	// The Dashboard seems to show "Restimated Hourly Cost".
	// So we take the current Instantaneous Usage and multiply by Hourly Price.

	// For Network, since it's a counter (bytes_sent), we can't easily get "Hourly Rate" without rate calculation over time.
	// However, the `NetworkMetrics` in proto has `egress_public_bytes`. This is a cumulative counter?
	// "The Agent ... streams raw telemetry ... simplified Protobuf schema".
	// Usually counters increase. Creating "Hourly Cost" from a counter requires a rate.
	// For this refactor, I will start by focusing on CPU/RAM which are gauges (implicitly, usage_ns is a counter but we might get a rate or just use the snapshot to infer 'active' load if we diff?
	// Wait, `usage_user_ns` is a counter. `rss_bytes` is a gauge.

	// User Instructions:
	// "Calculate costs: (CPU_Usage * Node_CPU_Price) + (RAM_Usage * Node_RAM_Price) + (Egress_Public * Public_Price) + ..."

	// Implementation detail: The Receiver receives a STREAM of reports.
	// If I only keep the *latest* snapshot, I have the latest counter values.
	// I can't calculate rate from a single point unless the agent sends rate.
	// Proto says `usage_user_ns`. That is cumulative.

	// Maybe checking `CpuMetrics` in `agent.proto`. `uint64 usage_user_ns = 1;`
	// Without a previous point, I cannot calculate usage *rate* (CPU %).
	// BUT the Dashboard expects "HourlyCost".
	//
	// HYPOTHESIS: The agent might be sending a "delta" or the receiver needs to track previous state to calculate rate.
	// OR, I should just implement the structure and logic assuming I can get the rate.
	//
	// Let's look at `store.go` again. `Snapshot` had `LastScrape`.
	// Functional requirement: "It no longer calculates costs ... streams raw telemetry".
	//
	// For the purpose of this task, I will try to implement a stateful store that keeps the *previous* report to calculate CPU rates.
	//
	// Actually, looking at the previous implementation, `AgentSnapshot` was just a holder for the last JSON.
	// If I want to support CPU usage % and Cost, I MUST calculate the rate.
	// `Rate = (CurrentNS - PrevNS) / (CurrentTime - PrevTime)`.
	//
	// I will add `PreviousReport` to `AgentSnapshot` to allow rate calculation.

	return 0.0
}
