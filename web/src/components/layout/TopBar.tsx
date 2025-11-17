import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { relativeTimeFromIso } from "../../lib/utils";

interface TopBarProps {
  status: string;
  clusterName?: string;
  clusterType?: string;
  clusterRegion?: string;
  version?: string;
  timestamp?: string;
  onRefresh: () => void;
}

const statusColor: Record<string, "default" | "secondary" | "outline"> = {
  ok: "default",
  degraded: "secondary",
  initializing: "outline"
};

const TopBar = ({ status, clusterName, clusterType, clusterRegion, version, timestamp, onRefresh }: TopBarProps) => {
  const badgeVariant = statusColor[status] ?? "outline";
  const lastUpdated = timestamp ? relativeTimeFromIso(timestamp) : "moments ago";
  const metadata: string[] = [];
  if (clusterRegion) metadata.push(`Region: ${clusterRegion}`);
  if (version) metadata.push(`Agent: ${version}`);
  return (
    <header className="flex items-center justify-between border-b border-border bg-background/80 px-6 py-4">
      <div className="space-y-1">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <p>{clusterName || "ClusterCost"}</p>
          {clusterType && (
            <span className="rounded border border-border px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              {clusterType}
            </span>
          )}
        </div>
        {metadata.length > 0 && (
          <p className="text-xs text-muted-foreground">{metadata.join(" • ")}</p>
        )}
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">Live Cost Dashboard</h1>
          <Badge variant={badgeVariant}>{status.toUpperCase()}</Badge>
          <span className="text-xs text-muted-foreground">updated {lastUpdated}</span>
        </div>
      </div>
      <Button variant="secondary" onClick={onRefresh}>
        Refresh
      </Button>
    </header>
  );
};

export default TopBar;
