import { useCallback, useMemo, useState } from "react";
import ReactFlow, {
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  type Edge,
  type Node,
  type NodeProps
} from "reactflow";
import "reactflow/dist/style.css";

import { fetchNetworkTopology, type NetworkEdge } from "@/lib/api";
import { useApiData } from "@/hooks/useApiData";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Box,
  Cloud,
  Database,
  Globe,
  Layers,
  Network,
  Repeat,
  Server,
  Circle
} from "lucide-react";

const lookbackOptions = [
  { label: "15m", value: "15m" },
  { label: "1h", value: "1h" },
  { label: "6h", value: "6h" },
  { label: "24h", value: "24h" }
];

const nodeSpacing = 220;
const groupPadding = 22;
const groupHeaderHeight = 36;
const defaultCostThreshold = 0.01;
const maxExternalEndpoints = 10;
const maxInfraEndpoints = 10;

type ResourceKind =
  | "service"
  | "deployment"
  | "statefulset"
  | "daemonset"
  | "pod"
  | "node"
  | "ip"
  | "external"
  | "unknown";

type ResourceNodeData = {
  title: string;
  kind: ResourceKind;
  namespace: string;
};

type NamespaceNodeData = {
  name: string;
  count: number;
};

type AggregatedEdge = {
  srcId: string;
  dstId: string;
  bytesSent: number;
  bytesReceived: number;
  egressCostUsd: number;
  connectionCount: number;
  isExternal: boolean;
  isCrossAz: boolean;
  direction: "egress" | "ingress" | "internal";
};

const parseWorkloadFromPod = (podName: string) => {
  const parts = podName.split("-");
  const last = parts[parts.length - 1];
  const secondLast = parts[parts.length - 2] ?? "";

  if (parts.length >= 2 && /^\d+$/.test(last)) {
    return { name: parts.slice(0, -1).join("-"), kind: "statefulset" as const };
  }

  if (parts.length >= 3 && /^[a-z0-9]{5}$/.test(last) && /^[a-z0-9]{9,10}$/.test(secondLast)) {
    return { name: parts.slice(0, -2).join("-"), kind: "deployment" as const };
  }

  if (parts.length >= 2 && /^[a-z0-9]{5}$/.test(last)) {
    return { name: parts.slice(0, -1).join("-"), kind: "daemonset" as const };
  }

  return { name: podName, kind: "pod" as const };
};

const extractNamespaceFromServices = (services: string | null | undefined) => {
  if (!services) return "";
  const first = services.split(",")[0]?.trim();
  if (!first) return "";
  const [namespace] = first.split("/");
  return namespace ?? "";
};

const extractServiceName = (services: string | null | undefined) => {
  if (!services) return "";
  const first = services.split(",")[0]?.trim();
  if (!first) return "";
  const [_, name] = first.split("/");
  return name ?? "";
};

const getExternalKey = (edge: NetworkEdge) => {
  if (edge.dstKind !== "external") return "";
  return `external:${edge.dstIp || "unknown"}`;
};

const getInfraKey = (edge: NetworkEdge, side: "src" | "dst") => {
  const namespace = side === "src" ? edge.srcNamespace : edge.dstNamespace;
  const pod = side === "src" ? edge.srcPodName : edge.dstPodName;
  const node = side === "src" ? edge.srcNodeName : edge.dstNodeName;
  const ip = side === "src" ? edge.srcIp : edge.dstIp;

  if (namespace || pod) return "";
  if (node) return `infra:node:${node}`;
  if (ip) return `infra:ip:${ip}`;
  return "";
};

const buildEndpointGroup = (
  edge: NetworkEdge,
  side: "src" | "dst",
  topExternal: Set<string>,
  topInfra: Set<string>
) => {
  const namespace = side === "src" ? edge.srcNamespace : edge.dstNamespace;
  const pod = side === "src" ? edge.srcPodName : edge.dstPodName;
  const node = side === "src" ? edge.srcNodeName : edge.dstNodeName;
  const ip = side === "src" ? edge.srcIp : edge.dstIp;
  const isExternal = side === "dst" && edge.dstKind === "external";
  const services = side === "dst" ? edge.dstServices : "";

  if (isExternal) {
    const externalKey = getExternalKey(edge);
    if (externalKey && !topExternal.has(externalKey)) {
      return {
        id: "external:other",
        title: "Other external IPs",
        kind: "external" as const,
        namespace: "external",
        isExternal: true
      };
    }
    return {
      id: externalKey || `external:${ip || "unknown"}`,
      title: ip || "External",
      kind: "external" as const,
      namespace: "external",
      isExternal: true
    };
  }

  const serviceName = !pod ? extractServiceName(services) : "";
  const serviceNamespace = !namespace && serviceName ? extractNamespaceFromServices(services) : namespace;

  if (serviceNamespace && serviceName) {
    return {
      id: `${side}:service:${serviceNamespace}/${serviceName}`,
      title: serviceName,
      kind: "service" as const,
      namespace: serviceNamespace,
      isExternal: false
    };
  }

  if (namespace && pod) {
    const workload = parseWorkloadFromPod(pod);
    return {
      id: `${side}:${workload.kind}:${namespace}/${workload.name}`,
      title: workload.name,
      kind: workload.kind,
      namespace,
      isExternal: false
    };
  }

  if (node) {
    const infraKey = getInfraKey(edge, side);
    if (infraKey && !topInfra.has(infraKey)) {
      return {
        id: "infra:other",
        title: "Other infra endpoints",
        kind: "node" as const,
        namespace: "infrastructure",
        isExternal: false
      };
    }
    return {
      id: `${side}:node:${node}`,
      title: node,
      kind: "node" as const,
      namespace: "infrastructure",
      isExternal: false
    };
  }

  if (ip) {
    const infraKey = getInfraKey(edge, side);
    if (infraKey && !topInfra.has(infraKey)) {
      return {
        id: "infra:other",
        title: "Other infra endpoints",
        kind: "ip" as const,
        namespace: "infrastructure",
        isExternal: false
      };
    }
    return {
      id: `${side}:ip:${ip}`,
      title: ip,
      kind: "ip" as const,
      namespace: "infrastructure",
      isExternal: false
    };
  }

  return {
    id: `${side}:unknown`,
    title: "Unknown",
    kind: "unknown" as const,
    namespace: "unknown",
    isExternal: false
  };
};

const edgeColor = (edge: AggregatedEdge) => {
  if (edge.isExternal && edge.direction === "ingress") return "#2563eb";
  if (edge.isExternal) return "#ef4444";
  if (edge.isCrossAz) return "#f97316";
  return "#0ea5a4";
};

const edgeLabel = (edge: AggregatedEdge) => {
  const cost = edge.egressCostUsd ? `$${edge.egressCostUsd.toFixed(2)}` : "$0.00";
  const bytes = edge.bytesSent + edge.bytesReceived;
  if (bytes <= 0) return cost;
  const mb = bytes / (1024 * 1024);
  const direction = edge.direction === "internal" ? "Internal" : edge.direction === "egress" ? "Egress" : "Ingress";
  return `${direction} • ${cost} • ${mb.toFixed(1)}MB • ${edge.connectionCount} conns`;
};

const edgeWidth = (edge: AggregatedEdge) => {
  const bytes = edge.bytesSent + edge.bytesReceived;
  if (bytes <= 0) return 1.5;
  const mb = bytes / (1024 * 1024);
  const connWeight = Math.log10(edge.connectionCount + 1) * 0.6;
  return Math.min(7, Math.max(1.5, 1.2 + Math.log10(mb + 1) + connWeight));
};

const kindMeta: Record<
  ResourceKind,
  { icon: typeof Box; color: string; bg: string; label: string }
> = {
  service: { icon: Network, color: "#0f766e", bg: "#ccfbf1", label: "Service" },
  deployment: { icon: Layers, color: "#7c3aed", bg: "#ede9fe", label: "Deployment" },
  statefulset: { icon: Database, color: "#b45309", bg: "#ffedd5", label: "StatefulSet" },
  daemonset: { icon: Repeat, color: "#1d4ed8", bg: "#dbeafe", label: "DaemonSet" },
  pod: { icon: Box, color: "#0f172a", bg: "#e2e8f0", label: "Pod" },
  node: { icon: Server, color: "#0f766e", bg: "#ccfbf1", label: "Node" },
  ip: { icon: Globe, color: "#0f172a", bg: "#e2e8f0", label: "IP" },
  external: { icon: Cloud, color: "#b91c1c", bg: "#fee2e2", label: "External" },
  unknown: { icon: Circle, color: "#475569", bg: "#e2e8f0", label: "Unknown" }
};

const ResourceNode = ({ data }: NodeProps<ResourceNodeData>) => {
  const meta = kindMeta[data.kind];
  const Icon = meta.icon;

  return (
    <div
      style={{
        width: 190,
        padding: "10px 12px",
        borderRadius: 12,
        background: "#ffffff",
        border: `1px solid ${meta.color}40`,
        boxShadow: "0 12px 26px rgba(15, 23, 42, 0.12)",
        color: "#0f172a",
        wordBreak: "break-word"
      }}
    >
      <Handle type="target" position={Position.Left} style={{ background: meta.color }} />
      <div className="flex items-center gap-2">
        <span
          className="flex h-7 w-7 items-center justify-center rounded-full"
          style={{ background: meta.bg, color: meta.color }}
        >
          <Icon size={16} />
        </span>
        <div className="text-xs font-semibold uppercase tracking-wide" style={{ color: meta.color }}>
          {meta.label}
        </div>
      </div>
      <div className="mt-2 text-sm font-semibold leading-tight">{data.title}</div>
      <div className="text-xs text-muted-foreground">{data.namespace}</div>
      <Handle type="source" position={Position.Right} style={{ background: meta.color }} />
    </div>
  );
};

const NamespaceNode = ({ data }: NodeProps<NamespaceNodeData>) => (
  <div
    style={{
      width: "100%",
      height: "100%",
      borderRadius: 18,
      border: "1px solid rgba(15, 23, 42, 0.12)",
      background: "linear-gradient(135deg, rgba(248,250,252,0.7), rgba(226,232,240,0.6))",
      boxShadow: "inset 0 0 0 1px rgba(148,163,184,0.2)",
      padding: groupPadding
    }}
  >
    <div className="flex items-center justify-between">
      <div className="text-sm font-semibold text-slate-800">{data.name}</div>
      <div className="rounded-full bg-slate-900/10 px-2 py-0.5 text-xs font-semibold text-slate-700">
        {data.count}
      </div>
    </div>
  </div>
);

const NetworkTopologyPage = () => {
  const [lookback, setLookback] = useState("1h");
  const [namespace, setNamespace] = useState("");
  const [limit, setLimit] = useState(500);
  const [costThreshold, setCostThreshold] = useState(defaultCostThreshold);
  const nodeTypes = useMemo(() => ({ resource: ResourceNode, namespace: NamespaceNode }), []);

  const fetchTopology = useCallback(
    () =>
      fetchNetworkTopology({
        lookback,
        namespace: namespace || undefined,
        limit: limit > 0 ? limit : undefined
      }),
    [lookback, namespace, limit]
  );

  const { data, loading, error, refresh } = useApiData(fetchTopology);

  const { nodes, edges } = useMemo(() => {
    if (!data?.edges?.length) return { nodes: [], edges: [] };

    const nodeMap = new Map<string, Node<ResourceNodeData>>();
    const aggregatedEdges = new Map<string, AggregatedEdge>();

    const externalUsage = new Map<string, number>();
    const infraUsage = new Map<string, number>();

    data.edges.forEach((edge) => {
      if (edge.egressCostUsd <= costThreshold) return;
      const bytes = edge.bytesSent + edge.bytesReceived;
      const externalKey = getExternalKey(edge);
      if (externalKey) {
        externalUsage.set(externalKey, (externalUsage.get(externalKey) ?? 0) + bytes);
      }

      (["src", "dst"] as const).forEach((side) => {
        const infraKey = getInfraKey(edge, side);
        if (!infraKey) return;
        infraUsage.set(infraKey, (infraUsage.get(infraKey) ?? 0) + bytes);
      });
    });

    const topExternal = new Set(
      Array.from(externalUsage.entries())
        .sort((a, b) => b[1] - a[1])
        .slice(0, maxExternalEndpoints)
        .map(([key]) => key)
    );
    const topInfra = new Set(
      Array.from(infraUsage.entries())
        .sort((a, b) => b[1] - a[1])
        .slice(0, maxInfraEndpoints)
        .map(([key]) => key)
    );

    data.edges.forEach((edge) => {
      if (edge.egressCostUsd <= costThreshold) return;
      const srcGroup = buildEndpointGroup(edge, "src", topExternal, topInfra);
      const dstGroup = buildEndpointGroup(edge, "dst", topExternal, topInfra);

      if (!nodeMap.has(srcGroup.id)) {
        nodeMap.set(srcGroup.id, {
          id: srcGroup.id,
          data: { title: srcGroup.title, kind: srcGroup.kind, namespace: srcGroup.namespace },
          position: { x: 0, y: 0 },
          type: "resource"
        });
      }
      if (!nodeMap.has(dstGroup.id)) {
        nodeMap.set(dstGroup.id, {
          id: dstGroup.id,
          data: { title: dstGroup.title, kind: dstGroup.kind, namespace: dstGroup.namespace },
          position: { x: 0, y: 0 },
          type: "resource"
        });
      }

      const key = `${srcGroup.id}->${dstGroup.id}`;
      const isCrossAz =
        Boolean(edge.srcAvailabilityZone) &&
        Boolean(edge.dstAvailabilityZone) &&
        edge.srcAvailabilityZone !== edge.dstAvailabilityZone;
      const direction =
        srcGroup.kind === "external" ? "ingress" : dstGroup.kind === "external" ? "egress" : "internal";

      const existing = aggregatedEdges.get(key);
      if (existing) {
        existing.bytesSent += edge.bytesSent;
        existing.bytesReceived += edge.bytesReceived;
        existing.egressCostUsd += edge.egressCostUsd;
        existing.connectionCount += edge.connectionCount;
        existing.isCrossAz = existing.isCrossAz || isCrossAz;
        existing.isExternal = existing.isExternal || edge.dstKind === "external";
        existing.direction = existing.direction === "internal" ? direction : existing.direction;
      } else {
        aggregatedEdges.set(key, {
          srcId: srcGroup.id,
          dstId: dstGroup.id,
          bytesSent: edge.bytesSent,
          bytesReceived: edge.bytesReceived,
          egressCostUsd: edge.egressCostUsd,
          connectionCount: edge.connectionCount,
          isExternal: edge.dstKind === "external",
          isCrossAz,
          direction
        });
      }
    });

    const flowEdges: Edge[] = Array.from(aggregatedEdges.values())
      .filter((edge) => edge.egressCostUsd > costThreshold)
      .map((edge) => ({
        id: `${edge.srcId}->${edge.dstId}`,
        source: edge.srcId,
        target: edge.dstId,
        animated: edge.isExternal && edge.direction === "egress",
        label: edgeLabel(edge),
        style: {
          stroke: edgeColor(edge),
          strokeWidth: edgeWidth(edge)
        },
        labelStyle: {
          fill: "#0f172a",
          fontWeight: 600
        }
      }));

    const activeNodeIds = new Set<string>();
    flowEdges.forEach((edge) => {
      activeNodeIds.add(edge.source);
      activeNodeIds.add(edge.target);
    });

    const namespaceMap = new Map<string, Node<ResourceNodeData>[]>();
    nodeMap.forEach((node) => {
      if (!activeNodeIds.has(node.id)) return;
      const ns = node.data.namespace || "unknown";
      if (!namespaceMap.has(ns)) namespaceMap.set(ns, []);
      namespaceMap.get(ns)?.push(node);
    });

    const namespaceList = Array.from(namespaceMap.keys()).sort((a, b) => a.localeCompare(b));
    const namespaceNodes: Node<NamespaceNodeData>[] = [];
    const childNodes: Node<ResourceNodeData>[] = [];
    const namespaceColumns = namespaceList.length > 6 ? 3 : 2;
    const namespaceColumnWidth = 980;
    const namespaceGap = 40;
    const rowHeights: number[] = [];

    namespaceList.forEach((ns, index) => {
      const children = (namespaceMap.get(ns) ?? []).sort((a, b) => {
        if (a.data.kind !== b.data.kind) return a.data.kind.localeCompare(b.data.kind);
        return a.data.title.localeCompare(b.data.title);
      });

      const childColumns = Math.min(4, Math.max(2, Math.ceil(Math.sqrt(children.length || 1))));
      const childRows = Math.max(1, Math.ceil(children.length / childColumns));
      const groupWidth = Math.max(420, childColumns * nodeSpacing + groupPadding * 2);
      const groupHeight = childRows * nodeSpacing + groupPadding * 2 + groupHeaderHeight;

      const col = index % namespaceColumns;
      const row = Math.floor(index / namespaceColumns);
      rowHeights[row] = Math.max(rowHeights[row] ?? 0, groupHeight);

      namespaceNodes.push({
        id: `namespace:${ns}`,
        type: "namespace",
        data: { name: ns, count: children.length },
        position: { x: 0, y: 0 },
        style: { width: groupWidth, height: groupHeight, zIndex: 0, pointerEvents: "none" },
        selectable: false,
        draggable: false
      });

      children.forEach((node, idx) => {
        const rowIndex = Math.floor(idx / childColumns);
        const colIndex = idx % childColumns;
        node.parentNode = `namespace:${ns}`;
        node.extent = "parent";
        node.position = {
          x: groupPadding + colIndex * nodeSpacing,
          y: groupPadding + groupHeaderHeight + rowIndex * nodeSpacing
        };
        node.style = { ...(node.style ?? {}), zIndex: 2 };
        childNodes.push(node);
      });
    });

    const rowOffsets: number[] = [];
    rowHeights.forEach((height, idx) => {
      rowOffsets[idx] = (rowOffsets[idx - 1] ?? 0) + (idx === 0 ? 0 : rowHeights[idx - 1] + namespaceGap);
    });

    namespaceNodes.forEach((node, index) => {
      const col = index % namespaceColumns;
      const row = Math.floor(index / namespaceColumns);
      node.position = {
        x: col * (namespaceColumnWidth + namespaceGap),
        y: rowOffsets[row] ?? 0
      };
    });

    return { nodes: [...namespaceNodes, ...childNodes], edges: flowEdges };
  }, [data, costThreshold]);

  const totalCost = useMemo(() => {
    if (!data?.edges) return 0;
    return data.edges.reduce((sum, edge) => (edge.egressCostUsd > costThreshold ? sum + edge.egressCostUsd : sum), 0);
  }, [data, costThreshold]);

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-foreground">Cost-Aware Network Topology</h1>
          <p className="text-sm text-muted-foreground">
            Highlight cross-AZ and internet egress connections with real cost impact.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <div className="w-36">
            <Select value={lookback} onValueChange={setLookback}>
              <SelectTrigger>
                <SelectValue placeholder="Lookback" />
              </SelectTrigger>
              <SelectContent>
                {lookbackOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Input
            value={namespace}
            onChange={(event) => setNamespace(event.target.value)}
            placeholder="Namespace filter"
            className="w-44"
          />
          <Input
            value={String(limit)}
            onChange={(event) => {
              const parsed = Number(event.target.value);
              setLimit(Number.isFinite(parsed) ? Math.max(0, Math.floor(parsed)) : 0);
            }}
            placeholder="Max edges"
            className="w-28"
            inputMode="numeric"
          />
          <Input
            value={costThreshold.toString()}
            onChange={(event) => {
              const parsed = Number(event.target.value);
              setCostThreshold(Number.isFinite(parsed) ? Math.max(0, parsed) : 0);
            }}
            placeholder="Min cost $"
            className="w-28"
            inputMode="decimal"
          />
          <Button onClick={refresh}>Refresh</Button>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card className="p-4">
          <p className="text-xs uppercase tracking-wide text-muted-foreground">Edges</p>
          <p className="text-2xl font-semibold">{data?.totalEdges ?? 0}</p>
          <p className="text-xs text-muted-foreground">
            Aggregated {edges.length} • Limit {data?.requestedLimit ?? limit}
          </p>
        </Card>
        <Card className="p-4">
          <p className="text-xs uppercase tracking-wide text-muted-foreground">Estimated Egress Cost</p>
          <p className="text-2xl font-semibold">${totalCost.toFixed(2)}</p>
          <p className="text-xs text-muted-foreground">Across current window</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs uppercase tracking-wide text-muted-foreground">Legend</p>
          <div className="mt-2 space-y-1 text-sm text-muted-foreground">
            <div className="flex items-center gap-2">
              <span className="h-2 w-6 rounded-full bg-[#0ea5a4]" />
              <span>Intra-AZ / Internal</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="h-2 w-6 rounded-full bg-[#f97316]" />
              <span>Cross-AZ</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="h-2 w-6 rounded-full bg-[#ef4444]" />
              <span>Internet Egress</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="h-2 w-6 rounded-full bg-[#2563eb]" />
              <span>Internet Ingress</span>
            </div>
          </div>
        </Card>
      </div>

      <Card className="h-[640px] overflow-hidden">
        {loading ? (
          <div className="flex h-full items-center justify-center">
            <Skeleton className="h-[520px] w-[90%]" />
          </div>
        ) : error ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <p>Failed to load topology.</p>
            <Button variant="outline" onClick={refresh}>
              Retry
            </Button>
          </div>
        ) : nodes.length === 0 ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            No network connections found for this window.
          </div>
        ) : (
          <ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView>
            <Background gap={18} size={1} color="#e2e8f0" />
            <MiniMap zoomable pannable />
            <Controls />
          </ReactFlow>
        )}
      </Card>
    </div>
  );
};

export default NetworkTopologyPage;
