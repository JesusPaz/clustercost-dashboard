import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "../ui/sheet";
import { Progress } from "../ui/progress";
import { Badge } from "../ui/badge";
import { Tabs, TabsList, TabsTrigger } from "../ui/tabs";
import { formatCurrency } from "../../lib/utils";
import { fetchNodeStats, type NodeCost, type NodeStats } from "../../lib/api";
import { useState, useEffect } from "react";

interface NodeDetailSheetProps {
  node: (NodeCost & { monthlyCost: number }) | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const statusStyles: Record<NodeCost["status"], string> = {
  Ready: "border-emerald-500/40 bg-emerald-500/10 text-emerald-200",
  NotReady: "border-destructive/40 bg-destructive/10 text-destructive",
  Unknown: "border-muted bg-muted/40 text-muted-foreground"
};

const NodeDetailSheet = ({ node, open, onOpenChange }: NodeDetailSheetProps) => {
  const [window, setWindow] = useState("24h");
  const [stats, setStats] = useState<NodeStats | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open && node) {
      setLoading(true);
      fetchNodeStats(node.nodeName, window)
        .then(setStats)
        .catch((err) => {
          console.error("Failed to fetch node stats", err);
          setStats(null);
        })
        .finally(() => setLoading(false));
    } else {
      setStats(null);
    }
  }, [open, node, window]);

  if (!node) return null;

  const statusBadge = (
    <Badge variant="outline" className={statusStyles[node.status]}>
      {node.status}
    </Badge>
  );

  const usageSummary = (() => {
    if (node.cpuUsagePercent > 70 || node.memoryUsagePercent > 70) return "Node is heavily used.";
    if (node.cpuUsagePercent < 30 && node.memoryUsagePercent < 30) return "This node is mostly idle.";
    return "Usage looks normal.";
  })();

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-md overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="flex flex-col gap-1">
            <span className="text-lg font-semibold">{node.nodeName}</span>
            <span className="text-sm text-muted-foreground">
              {(node.instanceType ?? "Unknown type")} · {node.podCount} pods
            </span>
          </SheetTitle>
          <SheetDescription className="flex items-center gap-2">
            {statusBadge}
            {node.isUnderPressure && <Badge variant="destructive">Under pressure</Badge>}
          </SheetDescription>
        </SheetHeader>

        <div className="mt-6 space-y-6 text-sm">
          <section>
            <p className="text-xs uppercase text-muted-foreground mb-2">Current State</p>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-2xl font-semibold">{formatCurrency(node.monthlyCost)}</p>
                <p className="text-xs text-muted-foreground">Monthly cost</p>
              </div>
              <div>
                <p className="text-2xl font-semibold">{formatCurrency(node.hourlyCost, { maximumFractionDigits: 2 })}</p>
                <p className="text-xs text-muted-foreground">Hourly cost</p>
              </div>
            </div>
          </section>

          <section className="space-y-3">
            <p className="text-xs uppercase text-muted-foreground">Current Usage</p>
            <div>
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>CPU usage</span>
                <span>{node.cpuUsagePercent.toFixed(0)}%</span>
              </div>
              <Progress value={node.cpuUsagePercent} className="mt-2" />
            </div>
            <div>
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>Memory usage</span>
                <span>{node.memoryUsagePercent.toFixed(0)}%</span>
              </div>
              <Progress value={node.memoryUsagePercent} className="mt-2" />
            </div>
            <p className="text-xs text-muted-foreground pt-1">{usageSummary}</p>
          </section>

          <div className="my-4 border-t" />

          <section className="space-y-4">
            <div className="flex items-center justify-between">
              <p className="text-xs uppercase text-muted-foreground">Historical Analysis</p>
              <Tabs value={window} onValueChange={setWindow} className="w-[180px]">
                <TabsList className="grid w-full grid-cols-3 h-7">
                  <TabsTrigger value="24h" className="text-xs h-5">24h</TabsTrigger>
                  <TabsTrigger value="7d" className="text-xs h-5">7d</TabsTrigger>
                  <TabsTrigger value="30d" className="text-xs h-5">30d</TabsTrigger>
                </TabsList>
              </Tabs>
            </div>

            {loading ? (
              <div className="space-y-2 animate-pulse">
                <div className="h-4 bg-muted rounded w-3/4"></div>
                <div className="h-4 bg-muted rounded w-1/2"></div>
              </div>
            ) : stats ? (
              <>
                <div className="rounded-lg border bg-card p-3 shadow-sm">
                  <div className="flex flex-col gap-1">
                    <span className="text-[10px] uppercase tracking-wide text-muted-foreground">Real Usage Cost</span>
                    <span className="text-xl font-bold text-primary">{formatCurrency(stats.realUsageMonthlyCost)}</span>
                    <span className="text-xs text-muted-foreground">
                      vs {formatCurrency(stats.totalMonthlyCost)} potential
                    </span>
                  </div>
                </div>

                <div className="space-y-3">
                  <div>
                    <div className="flex items-center justify-between text-xs text-muted-foreground">
                      <span>P95 CPU ({window})</span>
                      <span>{stats.p95CpuUsagePercent.toFixed(1)}%</span>
                    </div>
                    <Progress value={stats.p95CpuUsagePercent} className="mt-2 h-1.5" />
                  </div>
                  <div>
                    <div className="flex items-center justify-between text-xs text-muted-foreground">
                      <span>P95 Memory ({window})</span>
                      <span>{stats.p95MemoryUsagePercent.toFixed(1)}%</span>
                    </div>
                    <Progress value={stats.p95MemoryUsagePercent} className="mt-2 h-1.5" />
                  </div>
                </div>
              </>
            ) : (
              <p className="text-xs text-muted-foreground">No historical data available.</p>
            )}
          </section>
        </div>
      </SheetContent>
    </Sheet>
  );
};

export default NodeDetailSheet;
