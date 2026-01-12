package store

import (
	"context"
	"testing"
)

func TestPricingCatalog_GetNodeResourcePrices(t *testing.T) {
	pc := NewPricingCatalog(nil)

	// Test case 1: m5.large (2 vCPU, 8GB RAM)
	// Price: $0.096/hr
	// CPU Pool: $0.048 -> Per Core: $0.024
	// RAM Pool: $0.048 -> Per GB: $0.006
	t.Run("m5.large split", func(t *testing.T) {
		cpuPrice, ramPrice := pc.GetNodeResourcePrices(context.Background(), "us-east-1", "m5.large", 2, 8*1024*1024*1024)

		expectedCPU := 0.024
		expectedRAM := 0.006

		if cpuPrice != expectedCPU {
			t.Errorf("Expected CPU price %f, got %f", expectedCPU, cpuPrice)
		}
		if ramPrice != expectedRAM {
			t.Errorf("Expected RAM price %f, got %f", expectedRAM, ramPrice)
		}
	})

	// Test case 2: Default fallback
	// Price: $0.05/hr
	// Defaults: 2 vCPU, 4GB RAM
	// CPU Pool: $0.025 -> Per Core: $0.0125
	// RAM Pool: $0.025 -> Per GB: $0.00625
	t.Run("Default split", func(t *testing.T) {
		cpuPrice, ramPrice := pc.GetNodeResourcePrices(context.Background(), "", "unknown.type", 0, 0)

		expectedCPU := 0.0125
		expectedRAM := 0.00625

		if cpuPrice != expectedCPU {
			t.Errorf("Expected CPU price %f, got %f", expectedCPU, cpuPrice)
		}
		if ramPrice != expectedRAM {
			t.Errorf("Expected RAM price %f, got %f", expectedRAM, ramPrice)
		}
	})
}

func TestCalculateHourlyCost(t *testing.T) {
	// 2 vCPU * 0.05/2/2 = 0.025
	// 4 GB * 0.05/2/4 = 0.00625
	cpuPrice := 0.025   // per core
	memPrice := 0.00625 // per GB

	// Usage: 1 core, 2 GB
	// Network: 1 GB Public, 1 GB CrossAZ
	// Cost = (1 * 0.025) + (2 * 0.00625) + (1 * 0.09) + (1 * 0.01)
	// Cost = 0.025 + 0.0125 + 0.09 + 0.01 = 0.1375

	t.Run("Includes Network Costs", func(t *testing.T) {
		cost := calculateHourlyCost(1.0, 2.0, 1.0, 1.0, cpuPrice, memPrice)
		expected := 0.1375

		if cost != expected {
			t.Errorf("Expected cost %f, got %f", expected, cost)
		}
	})
}
