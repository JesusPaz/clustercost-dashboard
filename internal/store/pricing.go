package store

import (
	"context"
	"fmt"

	"github.com/clustercost/clustercost-dashboard/internal/pricing"
)

// Pricing constants
const (
	// Egress costs (public internet)
	CostEgressPublic   = 0.09 // $0.09 per GB
	CostEgressCrossAZ  = 0.01 // $0.01 per GB
	CostEgressInternal = 0.00 // Free
)

// PricingCatalog allows looking up node prices.
type PricingCatalog struct {
	// No provider needed, we use static data from internal/pricing
}

// NewPricingCatalog returns a catalog.
func NewPricingCatalog() *PricingCatalog {
	return &PricingCatalog{}
}

// GetTotalNodePrice returns the total hourly cost of a node.
func (pc *PricingCatalog) GetTotalNodePrice(ctx context.Context, region, instanceType string) float64 {
	// 1. Try Shared Static Data
	key := fmt.Sprintf("%s|%s", region, instanceType)
	if price, ok := pricing.InstancePrices[key]; ok {
		return price
	}

	// 2. Fallback to generic defaults if completely unknown
	// check if we have a default for the instance type regardless of region (common for US-East-1 based defaults)
	// (Optional optimization: try "us-east-1|instanceType" as fallback?)
	fallbackKey := fmt.Sprintf("us-east-1|%s", instanceType)
	if price, ok := pricing.InstancePrices[fallbackKey]; ok {
		return price
	}

	return 0.05 // Ultimate fallback
}

// GetNodeResourcePrices calculates the cost per vCPU and per GB of RAM based on the instance type.
// Policy: 50% of instance cost allocated to CPU, 50% allocated to RAM.
func (pc *PricingCatalog) GetNodeResourcePrices(ctx context.Context, region, instanceType string, vCPUs int64, ramBytes int64) (cpuPricePerCore, ramPricePerGB float64) {
	totalHourlyPrice := pc.GetTotalNodePrice(ctx, region, instanceType)

	if vCPUs <= 0 {
		vCPUs = 2 // Default fallback
	}
	if ramBytes <= 0 {
		ramBytes = 4 * 1024 * 1024 * 1024 // Default fallback 4GB
	}
	ramGB := float64(ramBytes) / (1024 * 1024 * 1024)

	// User Policy: "Divide precio de instancia entre dos, mitad cpu y mitad ram"
	cpuPoolCost := totalHourlyPrice * 0.5
	ramPoolCost := totalHourlyPrice * 0.5

	cpuPricePerCore = cpuPoolCost / float64(vCPUs)
	ramPricePerGB = ramPoolCost / ramGB

	return cpuPricePerCore, ramPricePerGB
}

// Estimated Cost Calculation
// This calculates the *rate* of spend based on current usage.
// cpuUsageCores: Number of cores currently being used (e.g. 0.5 for 500m)
// memUsageGB: Amount of RAM currently used in GB
// monthlyEgressGB: Estimated monthly egress based on current rate/counter
func calculateHourlyCost(cpuUsageCores, memUsageGB, egressPublicGB, egressCrossAZGB float64, cpuPrice, memPrice float64) float64 {
	computeCost := (cpuUsageCores * cpuPrice)
	memoryCost := (memUsageGB * memPrice)

	networkCost := (egressPublicGB * CostEgressPublic) + (egressCrossAZGB * CostEgressCrossAZ)

	return computeCost + memoryCost + networkCost
}
