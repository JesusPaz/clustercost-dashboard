import { useMemo, useState, type ChangeEvent } from "react";
import { fetchNodes, type NodeCost } from "../../lib/api";
import { formatCurrency, formatPercentage, relativeTimeFromIso, toMonthlyCost, milliToCores } from "../../lib/utils";
import { useApiData } from "../../hooks/useApiData";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import NodeDetailSheet from "@/components/nodes/NodeDetailSheet";
import { MetricCard } from "@/components/common/MetricCard";
import { EfficiencyBar } from "@/components/nodes/EfficiencyBar";
import { AlertTriangleIcon, CheckCircle2Icon, SearchIcon, ArrowDownIcon } from "lucide-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

type SortKey = "cost" | "waste" | "efficiency";

const NodesPage = () => {
  const { data, loading, error, refresh } = useApiData(fetchNodes);
  const nodes = data ?? [];
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("waste");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const [selectedNode, setSelectedNode] = useState<(NodeCost & { monthlyCost: number }) | null>(null);

  // Fallback Cost Logic
  const getEstimatedCost = (instanceType: string | undefined): number => {
    if (!instanceType) return 73;
    if (instanceType.includes("nano")) return 4;
    if (instanceType.includes("micro")) return 8;
    if (instanceType.includes("small")) return 16;
    if (instanceType.includes("medium")) return 32;
    if (instanceType.includes("large") && !instanceType.includes("xlarge")) return 64;
    if (instanceType.includes("xlarge")) return 128;
    if (instanceType.includes("2xlarge")) return 256;
    return 73;
  };

  const derivedNodes = useMemo(() => {
    return nodes.map((node) => {
      let hourlyCost = node.hourlyCost ?? 0;
      let isEstimate = false;

      if (hourlyCost === 0) {
        hourlyCost = getEstimatedCost(node.instanceType) / 730;
        isEstimate = true;
      }

      const monthlyCost = hourlyCost * 730;
      const cpuAllocatable = node.cpuAllocatableMilli ?? 0;

      const cpuRequestPercent = cpuAllocatable > 0 ? ((node.cpuRequestedMilli ?? 0) / cpuAllocatable) * 100 : 0;
      const cpuUsage = node.cpuUsagePercent ?? 0;

      // Calculate Memory Stats
      const memAllocatable = node.memoryAllocatableBytes ?? 0;
      const memRequestPercent = memAllocatable > 0 ? ((node.memoryRequestedBytes ?? 0) / memAllocatable) * 100 : 0;
      const memUsage = node.memoryUsagePercent ?? 0;

      // FinOps Waste Calculation: Paying for Request but not Using it (CPU dominant for now, but could blend)
      const wastePercent = Math.max(0, cpuRequestPercent - cpuUsage);
      const wasteAmount = monthlyCost * (wastePercent / 100);

      const isEfficient = wastePercent < 15;
      const isOverProvisioned = wastePercent > 30;

      return {
        ...node,
        cpuUsagePercent: cpuUsage,
        cpuRequestPercent,
        memoryUsagePercent: memUsage,
        memRequestPercent,
        monthlyCost,
        isEstimate,
        wastePercent,
        wasteAmount,
        isEfficient,
        isOverProvisioned,
        shortName: node.nodeName.length > 20 ? node.nodeName.substring(0, 15) + "..." : node.nodeName
      };
    });
  }, [nodes]);

  const summary = useMemo(() => {
    const totalMonthly = derivedNodes.reduce((sum, n) => sum + n.monthlyCost, 0);
    const totalWaste = derivedNodes.reduce((sum, n) => sum + n.wasteAmount, 0);
    const potentialSavings = totalWaste * 0.6; // Conservative achievable savings

    return { totalMonthly, totalWaste, potentialSavings };
  }, [derivedNodes]);

  const sortedNodes = useMemo(() => {
    const rows = [...derivedNodes];
    rows.sort((a, b) => {
      const valA = sortKey === "waste" ? a.wasteAmount : (sortKey === "cost" ? a.monthlyCost : a.wastePercent);
      const valB = sortKey === "waste" ? b.wasteAmount : (sortKey === "cost" ? b.monthlyCost : b.wastePercent);
      return sortDirection === "asc" ? valA - valB : valB - valA;
    });
    return rows;
  }, [derivedNodes, sortKey, sortDirection]);

  // Sorting Handler
  const handleSort = (key: SortKey) => {
    if (key === sortKey) setSortDirection(d => d === "desc" ? "asc" : "desc");
    else { setSortKey(key); setSortDirection("desc"); }
  };

  if (loading && !data) return <Skeleton className="h-[80vh] w-full rounded-xl" />;
  if (error) return <div className="text-red-500 p-10 text-center">Failed to load data: {error}</div>;

  return (
    <div className="space-y-8 p-1">
      {/* Header Section */}
      <div className="flex justify-between items-end border-b border-border/40 pb-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Cluster Financials</h1>
          <p className="text-muted-foreground mt-1">Real-time analysis of infrastructure efficiency and waste.</p>
        </div>
        <div className="flex gap-3">
          <Button variant="outline" size="sm" onClick={refresh}>Refresh</Button>
          <Button size="sm">Download Report</Button>
        </div>
      </div>

      {/* The "Truth" Cards - High Impact Typography */}
      <div className="grid gap-6 md:grid-cols-3">
        <Card className="bg-card/50 backdrop-blur-sm shadow-sm md:col-span-1">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider">Monthly Spend</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-4xl font-bold tracking-tighter text-foreground">
              {formatCurrency(summary.totalMonthly)}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              Run rate based on current capacity
            </p>
          </CardContent>
        </Card>

        <Card className="bg-card/50 backdrop-blur-sm shadow-sm md:col-span-1 border-destructive/20 relative overflow-hidden">
          <div className="absolute top-0 right-0 p-2">
            {summary.totalWaste > 0 && (
              <Badge variant="destructive" className="uppercase text-[10px] tracking-widest font-bold">Action Required</Badge>
            )}
          </div>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider flex items-center gap-2">
              Monthly Waste <AlertTriangleIcon className="w-4 h-4 text-destructive" />
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className={`text-5xl font-extrabold tracking-tighter ${summary.totalWaste > 0 ? "text-destructive" : "text-emerald-500"}`}>
              {formatCurrency(summary.totalWaste)}
            </div>
            <p className="text-xs text-muted-foreground mt-2 font-mono">
              Money burned on unused reservations
            </p>
          </CardContent>
        </Card>

        <Card className="bg-card/50 backdrop-blur-sm shadow-sm md:col-span-1 border-emerald-500/20">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider">Actionable Savings</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-4xl font-bold tracking-tighter text-emerald-500">
              {formatCurrency(summary.potentialSavings)}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              Conservative achievable reduction
            </p>
          </CardContent>
        </Card>
      </div>

      {/* FinOps Table - High Density */}
      <Card className="border-0 shadow-none bg-transparent">
        <div className="flex flex-col sm:flex-row justify-between items-center mb-4 gap-4">
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="h-6 px-2 text-xs">
              {sortedNodes.length} Nodes
            </Badge>
            {/* Future: Add Filter logic here */}
          </div>
          <div className="relative w-full sm:max-w-xs">
            <SearchIcon className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Filter by node or instance..."
              className="pl-8 h-9 bg-background/60"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </div>

        <div className="rounded-lg border bg-card/60 overflow-hidden">
          <Table>
            <TableHeader className="bg-muted/30">
              <TableRow className="hover:bg-transparent border-b border-border/50">
                <TableHead className="w-[200px] font-semibold text-foreground">Node Identity</TableHead>
                <TableHead className="w-[150px] font-semibold text-foreground cursor-pointer" onClick={() => handleSort("cost")}>
                  Cost <span className="text-[10px] ml-1 font-normal opacity-50">▼</span>
                </TableHead>
                <TableHead className="w-[250px] font-semibold text-foreground">CPU Efficiency</TableHead>
                <TableHead className="w-[250px] font-semibold text-foreground">Memory Efficiency</TableHead>
                <TableHead className="text-right pr-6 font-semibold text-foreground cursor-pointer" onClick={() => handleSort("waste")}>Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedNodes.filter(n => n.nodeName.includes(search)).map((node) => (
                <TableRow key={node.nodeName} className="border-b border-border/40 hover:bg-muted/30 transition-colors">

                  {/* Column 1: Identity */}
                  <TableCell className="py-4 align-top">
                    <div className="flex flex-col gap-0.5 max-w-[220px]">
                      <span className="font-bold text-sm text-foreground">{node.instanceType || "Unknown"}</span>
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="text-xs text-muted-foreground font-mono break-all cursor-help opacity-70 hover:opacity-100 leading-tight">
                              {node.nodeName}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>
                            <p className="font-mono text-xs">{node.nodeName}</p>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </TableCell>

                  {/* Column 2: Cost */}
                  <TableCell className="py-4 align-top">
                    <div className="flex flex-col">
                      <div className="flex items-baseline gap-1">
                        <span className={`${node.isEstimate ? "text-muted-foreground font-normal" : "font-bold text-foreground"}`}>
                          {formatCurrency(node.monthlyCost)}
                        </span>
                        {node.isEstimate && (
                          <TooltipProvider>
                            <Tooltip>
                              <TooltipTrigger asChild><span className="text-[10px] text-amber-500 cursor-help">*</span></TooltipTrigger>
                              <TooltipContent>Estimated Cost</TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        )}
                      </div>
                      <span className="text-[10px] text-muted-foreground font-mono">
                        ${Number(node.hourlyCost.toFixed(4))}/hr
                      </span>
                    </div>
                  </TableCell>

                  {/* Column 3: CPU Efficiency (Stacked) */}
                  <TableCell className="py-3 align-top">
                    <EfficiencyBar
                      usagePercent={node.cpuUsagePercent}
                      requestPercent={node.cpuRequestPercent}
                      costPerMonth={node.monthlyCost}
                    />
                  </TableCell>

                  {/* Column 4: RAM Efficiency (Stacked) */}
                  <TableCell className="py-3 align-top">
                    <EfficiencyBar
                      usagePercent={node.memoryUsagePercent}
                      requestPercent={node.memRequestPercent}
                      costPerMonth={node.monthlyCost}
                    />
                  </TableCell>

                  {/* Column 5: Action */}
                  <TableCell className="py-4 align-top text-right pr-6">
                    {node.isEfficient ? (
                      <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-600 border-0">
                        <CheckCircle2Icon className="w-3 h-3 mr-1" /> Optimized
                      </Badge>
                    ) : (
                      <div className="flex flex-col items-end gap-1">
                        <Button
                          variant="outline"
                          size="sm"
                          className={`h-7 text-xs ${node.wastePercent > 50 ? "border-destructive/50 text-destructive hover:bg-destructive/10" : "border-amber-500/50 text-amber-600 hover:bg-amber-500/10"}`}
                          onClick={() => setSelectedNode(node)}
                        >
                          <ArrowDownIcon className="w-3 h-3 mr-1" />
                          {node.wastePercent > 50 ? "Fix Waste" : "Rightsize"}
                        </Button>
                        <span className="text-[10px] text-muted-foreground font-mono">
                          Save ~{formatCurrency(node.wasteAmount)}
                        </span>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </Card>

      <NodeDetailSheet
        node={selectedNode}
        open={!!selectedNode}
        onOpenChange={(open) => { if (!open) setSelectedNode(null); }}
      />
    </div>
  );
};

export default NodesPage;
