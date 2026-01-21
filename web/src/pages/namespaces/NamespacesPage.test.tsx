import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import NamespacesPage from "./NamespacesPage";
import { fetchNamespaces } from "../../lib/api";

const mockUseApiData = vi.fn();

vi.mock("../../hooks/useApiData", () => ({
  useApiData: (fetcher: any) => mockUseApiData(fetcher)
}));

// Mock ResizeObserver
// @ts-ignore
global.ResizeObserver = class ResizeObserver {
  observe() { }
  unobserve() { }
  disconnect() { }
};

const sampleNamespace = {
  clusterId: "cluster",
  namespace: "payments",
  hourlyCost: 10,
  podCount: 4,
  cpuRequestMilli: 2000,
  cpuUsageMilli: 1000,
  memoryRequestBytes: 1024 * 1024 * 1024,
  memoryUsageBytes: 512 * 1024 * 1024,
  labels: { environment: "production" },
  environment: "production"
};

afterEach(() => {
  mockUseApiData.mockReset();
});

describe("NamespacesPage", () => {
  it("renders namespace rows with provided data", () => {
    mockUseApiData.mockImplementation((fetcher: any) => {
      if (fetcher === fetchNamespaces) {
        return {
          data: { records: [sampleNamespace], lastUpdated: "2025-01-01T00:00:00Z" },
          loading: false,
          error: null,
          refresh: vi.fn()
        };
      }
      return { data: null, loading: false, error: null, refresh: vi.fn() };
    });

    render(<NamespacesPage />);

    // "payments" appears in Summary Card, Treemap, Optimization List, and Table.
    // Let's verify it exists at least once.
    expect(screen.getAllByText("payments")[0]).toBeInTheDocument();

    // Hourly cost 10 * 720 hours = $7,200
    // It appears in Total Monthly, Cost Treemap, etc.
    expect(screen.getAllByText("$7,200")[0]).toBeInTheDocument();
  });
});
