import { useRef } from "react";
import { fetchAgents, type AgentInfo } from "../lib/api";
import { useApiData } from "../hooks/useApiData";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { relativeTimeFromIso } from "../lib/utils";
import { Activity, Server, AlertCircle, CheckCircle2 } from "lucide-react";

const AgentsPage = () => {
  const { data: agents, loading, error, refresh } = useApiData(fetchAgents);

  if (loading && !agents) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-32" />
          <Skeleton className="h-9 w-24" />
        </div>
        <Card>
          <CardContent className="p-0">
            <div className="space-y-2 p-4">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (error) {
    return (
      <Card className="border-destructive/40 bg-destructive/10">
        <CardContent className="py-10 text-center text-sm text-destructive">
          <p className="flex items-center justify-center gap-2">
            <AlertCircle className="h-4 w-4" />
            {error}
          </p>
          <Button variant="outline" onClick={refresh} className="mt-4 border-destructive/20 hover:bg-destructive/20">
            Retry
          </Button>
        </CardContent>
      </Card>
    );
  }

  const sortedAgents = [...(agents || [])].sort((a, b) => {
    // Sort by Last Scrape Time Descending (most recent first)
    return new Date(b.lastScrapeTime).getTime() - new Date(a.lastScrapeTime).getTime();
  });

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Agents</h1>
          <p className="text-sm text-muted-foreground">
            Manage and monitor connected agents across your clusters.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
          {loading ? "Refreshing..." : "Refresh"}
        </Button>
      </header>

      {!agents || agents.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-10 text-center">
            <Server className="h-10 w-10 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">No Agents Connected</h3>
            <p className="max-w-sm text-sm text-muted-foreground">
              Install the ClusterCost agent in your Kubernetes clusters to start seeing data.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Agent Name</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last Seen</TableHead>
                <TableHead className="hidden md:table-cell">Cluster</TableHead>
                <TableHead className="hidden lg:table-cell">Node</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedAgents.map((agent) => (
                <AgentRow key={agent.name} agent={agent} />
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
};

const AgentRow = ({ agent }: { agent: AgentInfo }) => {
  const isOk = agent.status === "connected";
  const isOffline = agent.status === "offline";

  return (
    <TableRow>
      <TableCell className="font-medium">
        <div className="flex flex-col">
          <span title={agent.baseUrl || undefined}>{agent.name}</span>
          {agent.error && (
            <span className="text-xs text-destructive mt-1 flex items-center gap-1">
              <AlertCircle className="h-3 w-3" /> {agent.error}
            </span>
          )}
        </div>
      </TableCell>
      <TableCell>
        {isOk ? (
          <Badge variant="outline" className="border-emerald-500/50 text-emerald-600 bg-emerald-500/10 gap-1 w-fit">
            <CheckCircle2 className="h-3 w-3" /> Connected
          </Badge>
        ) : isOffline ? (
          <Badge variant="destructive" className="gap-1 w-fit">
            <AlertCircle className="h-3 w-3" /> Offline
          </Badge>
        ) : (
          <Badge variant="secondary" className="gap-1 w-fit">
            <Activity className="h-3 w-3" /> {agent.status}
          </Badge>
        )}
      </TableCell>
      <TableCell className="text-muted-foreground whitespace-nowrap">
        {relativeTimeFromIso(agent.lastScrapeTime)}
      </TableCell>
      <TableCell className="hidden md:table-cell font-mono text-xs text-muted-foreground">
        {agent.clusterId || "-"}
      </TableCell>
      <TableCell className="hidden lg:table-cell font-mono text-xs text-muted-foreground">
        {agent.nodeName || "-"}
      </TableCell>
    </TableRow>
  );
};

export default AgentsPage;
