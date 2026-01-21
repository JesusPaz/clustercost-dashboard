import type { Environment } from "./utils";

// Allow overriding the API base via env var (useful for production builds or direct CORS)
// If VITE_API_URL is set (e.g. "https://api.example.com"), we use that. 
// Otherwise default to local proxy "/api".
const API_PREFIX = import.meta.env.VITE_API_URL || "/api";

const normalizeEnvironment = (value?: string): Environment => {
  switch ((value || "").toLowerCase()) {
    case "production":
    case "prod":
      return "production";
    case "preprod":
    case "staging":
      return "preprod";
    case "system":
      return "system";
    case "nonprod":
    case "sandbox":
    case "dev":
    case "development":
      return "development";
    case "unknown":
    default:
      return "unknown";
  }
};

type OverviewResponseApi = {
  clusterName: string;
  timestamp: string;
  totalHourlyCost: number;
  totalMonthlyCost: number;
  envCostHourly: Record<string, number>;
  topNamespacesByCost: Array<{
    namespace: string;
    environment: string;
    hourlyCost: number;
  }>;
  savingsCandidates: Array<{
    namespace: string;
    environment: string;
    hourlyCost: number;
    cpuRequestMilli: number;
    cpuUsageMilli: number;
    memoryRequestBytes: number;
    memoryUsageBytes: number;
  }>;
};

export type OverviewResponse = {
  clusterName: string;
  timestamp: string;
  totalHourlyCost: number;
  totalMonthlyCost: number;
  envCostHourly: Record<string, number>;
  topNamespacesByCost: Array<{
    namespace: string;
    environment: Environment;
    hourlyCost: number;
  }>;
  savingsCandidates: Array<{
    namespace: string;
    environment: Environment;
    hourlyCost: number;
    cpuRequestMilli: number;
    cpuUsageMilli: number;
    memoryRequestBytes: number;
    memoryUsageBytes: number;
  }>;
};

type NamespaceCostRecordApi = {
  namespace: string;
  hourlyCost: number;
  podCount: number;
  cpuRequestMilli: number;
  cpuLimitMilli: number;
  cpuUsageMilli: number;
  cpuUsagePercent?: number;
  memoryRequestBytes: number;
  memoryUsageBytes: number;
  labels?: Record<string, string>;
  environment: string;
};

type NamespaceListApiResponse = {
  items: NamespaceCostRecordApi[];
  totalCount: number;
  timestamp: string;
};

export interface NamespaceCostRecord {
  namespace: string;
  hourlyCost: number;
  podCount: number;
  cpuRequestMilli: number;
  cpuLimitMilli: number;
  cpuUsageMilli: number;
  cpuUsagePercent: number;
  memoryRequestBytes: number;
  memoryUsageBytes: number;
  labels: Record<string, string>;
  environment: Environment;
}

export interface NamespacesResponse {
  lastUpdated: string;
  totalCount: number;
  records: NamespaceCostRecord[];
}

type NodeCostApi = {
  nodeName: string;
  hourlyCost: number;
  windowCost: number;
  activeHours: number;
  activeRatio: number;
  cpuUsagePercent: number;
  memoryUsagePercent: number;
  cpuRequestedMilli?: number;
  cpuLimitMilli?: number;
  memoryRequestedBytes?: number;
  memoryLimitBytes?: number;
  cpuAllocatableMilli?: number;
  memoryAllocatableBytes?: number;
  podCount: number;
  status: "Ready" | "NotReady" | "Unknown";
  isUnderPressure: boolean;
  instanceType?: string;
  labels?: Record<string, string>;
  taints?: string[];
};

type NodeListApiResponse = {
  items: NodeCostApi[];
  totalCount: number;
  timestamp: string;
};

export interface NodeCost extends NodeCostApi {
  lastUpdated?: string;
}

type ResourcesApiResponse = {
  timestamp: string;
  cpu: {
    usageMilli: number;
    requestMilli: number;
    efficiencyPercent: number;
    estimatedHourlyWasteCost: number;
  };
  memory: {
    usageBytes: number;
    requestBytes: number;
    efficiencyPercent: number;
    estimatedHourlyWasteCost: number;
  };
  namespaceWaste: Array<{
    namespace: string;
    environment: string;
    cpuWastePercent: number;
    memoryWastePercent: number;
    estimatedHourlyWasteCost: number;
  }>;
};

export type ResourcesSummary = {
  timestamp: string;
  cpu: {
    usageMilli: number;
    requestMilli: number;
    efficiencyPercent: number;
    estimatedHourlyWasteCost: number;
  };
  memory: {
    usageBytes: number;
    requestBytes: number;
    efficiencyPercent: number;
    estimatedHourlyWasteCost: number;
  };
  namespaceWaste: Array<{
    namespace: string;
    environment: Environment;
    cpuWastePercent: number;
    memoryWastePercent: number;
    estimatedHourlyWasteCost: number;
  }>;
};

export type AgentDatasetStatus = "ok" | "partial" | "missing";

export interface AgentStatusResponse {
  status: "connected" | "partial" | "offline";
  lastSync: string;
  datasets: {
    namespaces: AgentDatasetStatus;
    nodes: AgentDatasetStatus;
    resources: AgentDatasetStatus;
  };
  version?: string;
  updateAvailable: boolean;
  clusterName?: string;
  clusterType?: string;
  clusterRegion?: string;
  region?: string;
  nodeCount?: number;
}

export interface HealthResponse {
  status: string;
  clusterId?: string;
  clusterName?: string;
  clusterType?: string;
  clusterRegion?: string;
  version?: string;
  timestamp: string;
}

export type NetworkEdge = {
  srcNamespace: string;
  srcPodName: string;
  srcNodeName: string;
  srcIp: string;
  srcDnsName?: string;
  srcAvailabilityZone: string;
  dstNamespace: string;
  dstPodName: string;
  dstNodeName: string;
  dstIp: string;
  dstDnsName?: string;
  dstAvailabilityZone: string;
  dstKind: string;
  serviceMatch: string;
  dstServices: string;
  protocol: number;
  bytesSent: number;
  bytesReceived: number;
  egressCostUsd: number;
  connectionCount: number;
  firstSeen: number;
  lastSeen: number;
};

export type NetworkTopologyResponse = {
  clusterId: string;
  namespace?: string;
  start: string;
  end: string;
  edges: NetworkEdge[];
  totalEdges: number;
  requestedLimit: number;
  timestamp: string;
};

let authToken: string | null = null;
let unauthorizedHandler: (() => void) | null = null;

export const setAuthToken = (token: string | null) => {
  authToken = token;
};

export const setUnauthorizedHandler = (handler: (() => void) | null) => {
  unauthorizedHandler = handler;
};

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (authToken) {
    headers.set("Authorization", `Bearer ${authToken}`);
  }

  const response = await fetch(`${API_PREFIX}${path}`, {
    ...options,
    headers,
  });

  if (response.status === 401) {
    setAuthToken(null);
    unauthorizedHandler?.();
    throw new Error("Unauthorized");
  }

  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed with ${response.status}`);
  }
  return response.json();
}

export interface AgentInfo {
  name: string;
  baseUrl: string;
  status: string;
  lastScrapeTime: string;
  error?: string;
  clusterId?: string;
  nodeName?: string;
}

export const fetchAgents = async (): Promise<AgentInfo[]> => {
  return request<AgentInfo[]>("/agents");
};

export const login = async (username: string, password: string): Promise<{ token: string }> => {
  const resp = await request<{ token: string }>("/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
    headers: { "Content-Type": "application/json" },
  });
  return resp;
};

export const fetchOverview = async (): Promise<OverviewResponse> => {
  const resp = await request<OverviewResponseApi>("/cost/overview");
  return {
    ...resp,
    topNamespacesByCost: resp.topNamespacesByCost.map((item) => ({
      ...item,
      environment: normalizeEnvironment(item.environment)
    })),
    savingsCandidates: resp.savingsCandidates.map((item) => ({
      ...item,
      environment: normalizeEnvironment(item.environment)
    }))
  };
};

export const fetchNamespaces = async (): Promise<NamespacesResponse> => {
  const resp = await request<NamespaceListApiResponse>("/cost/namespaces");
  return {
    lastUpdated: resp.timestamp,
    totalCount: resp.totalCount,
    records: resp.items.map((record) => ({
      ...record,
      labels: record.labels ?? {},
      environment: normalizeEnvironment(record.environment),
      cpuLimitMilli: record.cpuLimitMilli ?? 0,
      cpuUsagePercent: record.cpuUsagePercent ?? 0
    }))
  };
};

export const fetchNodes = async (window?: string): Promise<NodeCost[]> => {
  const query = window ? `?window=${window}` : "";
  const resp = await request<NodeListApiResponse>(`/cost/nodes${query}`);
  return resp.items.map((node) => ({
    ...node,
    labels: node.labels ?? {},
    taints: node.taints ?? [],
    lastUpdated: resp.timestamp
  }));
};

export interface NodeStats {
  nodeName: string;
  p95CpuUsagePercent: number;
  p95MemoryUsagePercent: number;
  totalMonthlyCost: number;
  realUsageMonthlyCost: number;
  window: string;
}

export const fetchNodeStats = async (name: string, window: string): Promise<NodeStats> => {
  return request<NodeStats>(`/cost/nodes/${name}/stats?window=${window}`);
};

export interface PodMetrics {
  podName: string;
  namespace: string;
  qosClass: string;
  cpuRequestMilli: number;
  cpuP95Milli: number;
  memoryRequestBytes: number;
  memoryP95Bytes: number;
}

export const fetchNodePods = async (name: string, window: string): Promise<PodMetrics[]> => {
  return request<PodMetrics[]>(`/cost/nodes/${name}/pods?window=${window}`);
};

export const fetchResources = async (): Promise<ResourcesSummary> => {
  const resp = await request<ResourcesApiResponse>("/cost/resources");
  return {
    timestamp: resp.timestamp,
    cpu: resp.cpu,
    memory: resp.memory,
    namespaceWaste: resp.namespaceWaste.map((entry) => ({
      ...entry,
      environment: normalizeEnvironment(entry.environment)
    }))
  };
};

export const fetchHealth = () => request<HealthResponse>("/health");
export const fetchAgentStatus = () => request<AgentStatusResponse>("/agent");

export type NetworkTopologyParams = {
  clusterId?: string;
  namespace?: string | string[];
  lookback?: string;
  start?: string | number;
  end?: string | number;
  limit?: number;
  minCost?: number;
  minBytes?: number;
  minConnections?: number;
};

export const fetchNetworkTopology = async (params: NetworkTopologyParams): Promise<NetworkTopologyResponse> => {
  const search = new URLSearchParams();
  if (params.clusterId) search.set("clusterId", params.clusterId);
  if (params.namespace) {
    const value = Array.isArray(params.namespace) ? params.namespace.join(",") : params.namespace;
    search.set("namespace", value);
  }
  if (params.lookback) search.set("lookback", params.lookback);
  if (params.start !== undefined) search.set("start", String(params.start));
  if (params.end !== undefined) search.set("end", String(params.end));
  if (params.limit !== undefined) search.set("limit", String(params.limit));
  if (params.minCost !== undefined) search.set("minCost", String(params.minCost));
  if (params.minBytes !== undefined) search.set("minBytes", String(params.minBytes));
  if (params.minConnections !== undefined) search.set("minConnections", String(params.minConnections));

  const query = search.toString();
  return request<NetworkTopologyResponse>(`/network/topology${query ? `?${query}` : ""}`);
};
