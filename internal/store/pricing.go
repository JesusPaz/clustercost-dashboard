package store

import "context"

// Pricing constants
const (
	// Egress costs (public internet)
	CostEgressPublic   = 0.09 // $0.09 per GB
	CostEgressCrossAZ  = 0.01 // $0.01 per GB
	CostEgressInternal = 0.00 // Free
)

// PricingProvider defines the interface for fetching node pricing.
type PricingProvider interface {
	GetNodePrice(ctx context.Context, region, instanceType string) (float64, error)
}

// PricingCatalog allows looking up node prices.
type PricingCatalog struct {
	// Map instance type to hourly price
	InstancePrices map[string]float64
	Provider       PricingProvider
}

// NewPricingCatalog returns a catalog with some default mocked pricing.
func NewPricingCatalog(provider PricingProvider) *PricingCatalog {
	return &PricingCatalog{
		InstancePrices: map[string]float64{
			"t3.medium": 0.0416,
			"t3.large":  0.0832,
			"m5.large":  0.096,
			"m5.xlarge": 0.192,
			"c5.large":  0.085,
			"r5.large":  0.126,
			"default":   0.05, // Fallback
		},
		Provider: provider,
	}
}

// GetTotalNodePrice returns the total hourly cost of a node.
func (pc *PricingCatalog) GetTotalNodePrice(ctx context.Context, region, instanceType string) float64 {
	// Try Provider first
	if pc.Provider != nil && instanceType != "" && region != "" {
		price, err := pc.Provider.GetNodePrice(ctx, region, instanceType)
		if err == nil && price > 0 {
			pc.InstancePrices[instanceType] = price // Update cache
			return price
		}
	}

	// Fallback to local cache
	price, ok := pc.InstancePrices[instanceType]
	if !ok {
		price = pc.InstancePrices["default"]
	}
	return price
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
