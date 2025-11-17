import { useMemo } from "react";
import { fetchAgentStatus, fetchHealth, type AgentDatasetStatus, type AgentStatusResponse } from "../lib/api";
import { useApiData } from "../hooks/useApiData";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { relativeTimeFromIso, formatNumber } from "../lib/utils";

const statusConfig: Record<AgentStatusResponse["status"], { label: string; tone: string; description: string }> = {
  connected: {
    label: "Agent Connected",
    tone: "bg-emerald-500",
    description: "Data is flowing normally"
  },
  partial: {
    label: "Agent Partially Connected",
    tone: "bg-amber-400",
    description: "Some datasets are delayed"
  },
  offline: {
    label: "Agent Offline",
    tone: "bg-destructive",
    description: "No recent data received"
  }
};

const datasetLabels: Array<{ key: keyof AgentStatusResponse["datasets"];
  label: string }>
  = [
    { key: "namespaces", label: "Namespaces" },
    { key: "nodes", label: "Nodes" },
    { key: "resources", label: "Resources" }
  ];

const datasetTone: Record<AgentDatasetStatus, { label: string; className: string }> = {
  ok: { label: "OK", className: "text-emerald-400" },
  partial: { label: "Partial", className: "text-amber-400" },
  missing: { label: "Missing", className: "text-destructive" }
};

const AgentsPage = () => {
  const { data, loading, error, refresh } = useApiData(fetchAgentStatus);
  const {
    data: health,
    refresh: refreshHealth
  } = useApiData(fetchHealth);
  const statusDetails = data ? statusConfig[data.status] : null;
  const lastSyncLabel = data?.lastSync ? relativeTimeFromIso(data.lastSync) : "Unknown";

  const clusterMeta = useMemo(() => {
    const clusterName = data?.clusterName || health?.clusterName || health?.clusterId;
    const clusterType = data?.clusterType || health?.clusterType;
    const region = data?.clusterRegion || data?.region || health?.clusterRegion;
    const nodeCount = typeof data?.nodeCount === "number" ? data?.nodeCount : undefined;

    const hasMetadata = clusterName || clusterType || region || typeof nodeCount === "number";
    if (!hasMetadata) return null;

    return {
      clusterName: clusterName || "Unknown",
      clusterType: clusterType || (clusterName ? "Unknown" : undefined),
      region: region || "Unknown",
      nodeCount
    };
  }, [data, health]);

  const handleRefresh = () => {
    refresh();
    refreshHealth();
  };

  if (loading && !data) {
    return <Skeleton className="h-[60vh] w-full" />;
  }

  if (error) {
    return (
      <Card className="border-destructive/40 bg-destructive/10">
        <CardContent className="py-10 text-center text-sm text-destructive">{error}</CardContent>
      </Card>
    );
  }

  if (!data || !statusDetails) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>No agent data</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground space-y-3">
          <p>The dashboard hasn\'t received any metrics from your agent yet.</p>
          <Button variant="outline" onClick={handleRefresh}>Refresh</Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Agent</h1>
          <p className="text-sm text-muted-foreground">Agent connection and data health</p>
        </div>
        <Button variant="outline" onClick={handleRefresh} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </header>

      <section className="grid gap-4 md:grid-cols-2">
        <Card className="md:col-span-2">
          <CardContent className="flex flex-wrap items-center justify-between gap-4 py-6">
            <div className="flex items-center gap-4">
              <span className={`h-6 w-6 rounded-full ${statusDetails.tone}`} aria-hidden="true" />
              <div>
                <p className="text-sm text-muted-foreground">{statusDetails.description}</p>
                <p className="text-3xl font-semibold">{statusDetails.label}</p>
              </div>
            </div>
            <div className="text-sm text-muted-foreground">
              Last sync {lastSyncLabel}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Data Health</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {datasetLabels.map(({ key, label }) => (
              <div key={key} className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{label}</span>
                <span className={datasetTone[data.datasets[key]].className}>
                  {datasetTone[data.datasets[key]].label}
                </span>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Agent Version</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <p className="text-3xl font-semibold">{data.version || "Unknown"}</p>
            {data.updateAvailable && <Badge variant="destructive">Update available</Badge>}
            <p className="text-xs text-muted-foreground">Keep the agent updated for best accuracy.</p>
          </CardContent>
        </Card>
      </section>

      {clusterMeta && (
        <Card>
          <CardHeader>
            <CardTitle>Cluster metadata</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 text-sm text-muted-foreground md:grid-cols-4">
            <div>
              <p className="text-xs uppercase">Cluster</p>
              <p className="text-base text-foreground">{clusterMeta.clusterName}</p>
            </div>
            {clusterMeta.clusterType && (
              <div>
                <p className="text-xs uppercase">Type</p>
                <p className="text-base text-foreground">{clusterMeta.clusterType}</p>
              </div>
            )}
            <div>
              <p className="text-xs uppercase">Region</p>
              <p className="text-base text-foreground">{clusterMeta.region}</p>
            </div>
            <div>
              <p className="text-xs uppercase">Nodes</p>
              <p className="text-base text-foreground">
                {typeof clusterMeta.nodeCount === "number" ? formatNumber(clusterMeta.nodeCount) : "Unknown"}
              </p>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
};

export default AgentsPage;
