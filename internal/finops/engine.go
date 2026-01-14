package finops

import (
	"context"
	"math"

	agentv1 "github.com/clustercost/clustercost-dashboard/internal/proto/agent/v1"
	"github.com/clustercost/clustercost-dashboard/internal/store"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

// EfficiencyReport represents the financial analysis of a workload.
type EfficiencyReport struct {
	Namespace          string  `json:"namespace"`
	Service            string  `json:"service"`
	RequestedCostMo    float64 `json:"requested_cost_mo"`
	ActualUsageCostMo  float64 `json:"actual_usage_cost_mo"`
	PotentialSavingsMo float64 `json:"potential_savings_mo"`
	ConfidenceScore    float64 `json:"confidence_score"`
	EfficiencyScore    float64 `json:"efficiency_score"` // 0-100
}

// Engine calculates efficiency scores.
type Engine struct {
	vmClient *vm.Client
	pricing  *store.PricingCatalog
}

// NewEngine creates a new FinOps engine.
func NewEngine(vmClient *vm.Client, pricing *store.PricingCatalog) *Engine {
	return &Engine{
		vmClient: vmClient,
		pricing:  pricing,
	}
}

// CalculatePodEfficiency computes the efficiency report for a single pod.
func (e *Engine) CalculatePodEfficiency(ctx context.Context, pod *agentv1.PodMetric, clusterID, nodeRegion, nodeAZ, instanceType string) (*EfficiencyReport, error) {
	// 1. Get Pricing Dimensions
	if instanceType == "" {
		instanceType = "m5.large" // Fallback
	}

	// Get unit prices
	cpuPricePerCore, ramPricePerGB := e.pricing.GetNodeResourcePrices(ctx, nodeRegion, instanceType, 2, 8*1024*1024*1024)

	// 2. Calculate Requested Cost (The "Price Tag")
	// CPU
	reqCPUCores := float64(pod.Cpu.RequestMillicores) / 1000.0
	reqCPUHourly := reqCPUCores * cpuPricePerCore

	// Memory
	reqRAMGB := float64(pod.Memory.RequestBytes) / (1024 * 1024 * 1024)
	reqRAMHourly := reqRAMGB * ramPricePerGB

	totalReqHourly := reqCPUHourly + reqRAMHourly
	totalReqMonthly := totalReqHourly * 730 // 730 hours in a month

	// 3. Get Actual Usage (P95 from VictoriaMetrics)
	p95CPU, p95RAMBytes, err := e.vmClient.GetPodP95Usage(ctx, clusterID, pod.Namespace, pod.PodName)
	if err != nil {
		// If VM is down or no data, we can't calculate usage.
		// Return partial report or error.
		return nil, err
	}

	p95RAMGB := float64(p95RAMBytes) / (1024 * 1024 * 1024)

	// 4. Calculate Usage Cost (The "Reality")
	usageCPUHourly := p95CPU * cpuPricePerCore
	usageRAMHourly := p95RAMGB * ramPricePerGB
	totalUsageHourly := usageCPUHourly + usageRAMHourly
	totalUsageMonthly := totalUsageHourly * 730

	// 5. Calculate Waste & Savings
	// Safety Buffer: Recommended = P95 * 1.2
	safeCPU := p95CPU * 1.2
	safeRAM := p95RAMGB * 1.2

	// Potential Savings = Cost(Requested) - Cost(Safe)
	// If Requested < Safe, Savings is 0 (underprovisioned).
	savingsHourly := 0.0

	costSafeCPU := safeCPU * cpuPricePerCore
	costSafeRAM := safeRAM * ramPricePerGB
	totalSafeHourly := costSafeCPU + costSafeRAM

	if totalReqHourly > totalSafeHourly {
		savingsHourly = totalReqHourly - totalSafeHourly
	}

	potentialSavingsMo := savingsHourly * 730

	// 6. Efficiency Score
	// Score = (Safe / Requested) * 100
	// If Unset/Zero requests, efficiency is undefined (or 0).
	efficiencyScore := 0.0
	if totalReqHourly > 0 {
		efficiencyScore = (totalSafeHourly / totalReqHourly) * 100
		if efficiencyScore > 100 {
			efficiencyScore = 100 // Cap at 100% (underprovisioned is "efficient" in terms of waste, but risky)
		}
	}

	return &EfficiencyReport{
		Namespace:          pod.Namespace,
		Service:            pod.PodName, // Or workload name if available
		RequestedCostMo:    totalReqMonthly,
		ActualUsageCostMo:  totalUsageMonthly,
		PotentialSavingsMo: potentialSavingsMo,
		ConfidenceScore:    0.95, // Hardcoded for MVP as per VM availability
		EfficiencyScore:    math.Round(efficiencyScore*100) / 100,
	}, nil
}
