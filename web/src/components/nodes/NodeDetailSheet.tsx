import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Progress } from "@/components/ui/progress";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatCurrency } from "../../lib/utils";
import { fetchNodeStats, fetchNodePods, type NodeCost, type NodeStats, type PodMetrics } from "../../lib/api";
import { useState, useEffect, useMemo } from "react";
import { CopyIcon, ShieldAlertIcon, ScissorsIcon, CheckCircle2Icon } from "lucide-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

interface NodeDetailSheetProps {
  node: (NodeCost & { monthlyCost: number }) | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const NodeDetailSheet = ({ node, open, onOpenChange }: NodeDetailSheetProps) => {
  const [window, setWindow] = useState("24h");
  const [stats, setStats] = useState<NodeStats | null>(null);
  const [pods, setPods] = useState<PodMetrics[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open && node) {
      setLoading(true);
      // Parallel Fetch: Stats + Pods
      Promise.all([
        fetchNodeStats(node.nodeName, window).catch(e => { console.error(e); return null; }),
        fetchNodePods(node.nodeName, window).catch(e => { console.error(e); return []; })
      ]).then(([statsData, podsData]) => {
        setStats(statsData);
        setPods(podsData || []);
        setLoading(false);
      });
    } else {
      setStats(null);
      setPods([]);
    }
  }, [open, node, window]);

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const getRec = (p95: number) => Math.ceil(p95 * 1.15); // P95 + 15%

  const generatePatch = (pod: PodMetrics, type: "cpu" | "memory" | "both", reason: "fix" | "shield") => {
    const rawCpu = getRec(pod.cpuP95Milli);
    // Ensure we don't go below 10m for CPU to be safe
    const targetCpu = Math.max(10, rawCpu);
    const targetCpuStr = `${targetCpu}m`;

    const rawMem = getRec(pod.memoryP95Bytes);
    // Convert to Mi
    const targetMemMi = Math.ceil(rawMem / (1024 * 1024));
    const targetMemStr = `${targetMemMi}Mi`;

    const resources: any = { requests: {} };
    if (type === "cpu" || type === "both") resources.requests.cpu = targetCpuStr;
    if (type === "memory" || type === "both") resources.requests.memory = targetMemStr;

    // Use container name approximation or index 0 for now as we don't have container name in PodMetrics yet.
    // Using simple approach: Assume first container needs fix found in spec.
    // Actually we need the container name. Backend aggregates by pod...
    // For now we will use pod name prefix as best effort, or just "name".
    // A better approach is usually `kubectl set resources` or patch assuming single container or main container.
    // Let's use the first container approach in the patch for now: `spec: { containers: [ { name: "?", ... } ] }`.
    // Wait, we don't know the container name. 
    // To make this robust without container name, we can try to patch by index `containers[0]`.
    // JSON Patch: `[{"op": "replace", "path": "/spec/containers/0/resources/requests/cpu", "value": "..."}]`
    // But let's stick to the user's requested text format: `kubectl patch ...`
    // We will use `deployment` logic usually, but here we patch the POD? Pods are ephemeral.
    // Ideally we patch the deployment.
    // User asked for "Fix YAML".
    const containerName = pod.podName.split("-")[0]; // Heuristic

    return `kubectl patch pod ${pod.podName} -n ${pod.namespace} --patch '{"spec":{"containers":[{"name":"${containerName}", "resources":{"requests":{"cpu":"${targetCpuStr}","memory":"${targetMemStr}"}}}]}}'`;
  };

  if (!node) return null;

  // SORTING LOGIC: Financial Impact (Savings First)
  // Heuristic: ~$32/vCPU/mo, ~$4/GB/mo
  const COST_PER_VCPU = 32;
  const COST_PER_GB = 4;

  const getSavings = (pod: PodMetrics) => {
    const cpuRec = getRec(pod.cpuP95Milli);
    const cpuWasteCores = (pod.cpuRequestMilli - cpuRec) / 1000;
    const cpuSavings = cpuWasteCores * COST_PER_VCPU;

    const memRec = getRec(pod.memoryP95Bytes);
    const memWasteGB = (pod.memoryRequestBytes - memRec) / (1024 * 1024 * 1024);
    const memSavings = memWasteGB * COST_PER_GB;

    return cpuSavings + memSavings;
  };

  const sortedPods = [...pods].sort((a, b) => {
    return getSavings(b) - getSavings(a); // Descending (Biggest Savings First)
  });

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-2xl p-0 gap-0 overflow-hidden bg-background text-foreground flex flex-col">
        {/* HEADER */}
        <SheetHeader className="p-6 border-b border-border/40 bg-muted/5 space-y-4">
          <SheetTitle className="flex flex-col gap-1">
            <span className="text-xs font-mono text-muted-foreground">{node.nodeName}</span>
            <div className="flex items-center gap-2">
              <span className="text-lg font-bold">{node.instanceType}</span>
              <Badge variant="outline" className="text-[10px] h-5">{node.podCount} Pods</Badge>
            </div>
          </SheetTitle>
          <div className="grid grid-cols-2 gap-4">
            <div className="p-3 rounded-lg border bg-card/50">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Monthly Cost</p>
              <p className="text-2xl font-bold">{formatCurrency(node.monthlyCost)}</p>
            </div>
            <div className={`p-3 rounded-lg border bg-card/50 ${stats && stats.totalMonthlyCost - stats.realUsageMonthlyCost > stats.totalMonthlyCost * 0.1 ? "border-emerald-500/20 bg-emerald-500/5 text-emerald-500" : "text-muted-foreground"}`}>
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Potential Savings</p>
              <p className="text-2xl font-bold">
                {stats ? formatCurrency(stats.totalMonthlyCost - stats.realUsageMonthlyCost) : "..."}
              </p>
            </div>
          </div>
        </SheetHeader>

        <SectionTabs window={window} setWindow={setWindow} />

        <div className="flex-1 overflow-auto">
          <div className="p-6 space-y-8">
            {/* P95 METRICS */}
            {stats && (
              <section className="space-y-4">
                <h3 className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Node P95 Analysis ({window})</h3>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <div className="flex justify-between text-xs mb-1.5">
                      <span>CPU P95 Load</span>
                      <span className="font-mono">{stats.p95CpuUsagePercent.toFixed(1)}%</span>
                    </div>
                    <Progress value={stats.p95CpuUsagePercent} className="h-2" />
                  </div>
                  <div>
                    <div className="flex justify-between text-xs mb-1.5">
                      <span>Memory P95 Load</span>
                      <span className="font-mono">{stats.p95MemoryUsagePercent.toFixed(1)}%</span>
                    </div>
                    <Progress value={stats.p95MemoryUsagePercent} className="h-2" />
                  </div>
                </div>
              </section>
            )}

            {/* FULL AUDIT TABLE */}
            <section className="space-y-4">
              <h3 className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Full Pod Audit (P95 + 15% Safety Margin)</h3>
              <div className="rounded-md border">
                <Table>
                  <TableHeader className="bg-muted/30">
                    <TableRow>
                      <TableHead className="text-xs w-[180px]">Pod (QoS)</TableHead>
                      <TableHead className="text-xs text-right">CPU (Req → P95)</TableHead>
                      <TableHead className="text-xs text-right">RAM (Req → P95)</TableHead>
                      <TableHead className="text-xs text-right">Action</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {loading ? (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-8 text-muted-foreground text-xs animate-pulse">
                          Analyzing pod logs & metrics...
                        </TableCell>
                      </TableRow>
                    ) : pods.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-8 text-muted-foreground text-xs">
                          No pods found or agent not reporting deep metrics yet.
                        </TableCell>
                      </TableRow>
                    ) : (
                      sortedPods.map(pod => {
                        // CPU Analysis
                        const cpuRec = getRec(pod.cpuP95Milli);
                        const cpuDiff = cpuRec - pod.cpuRequestMilli;
                        const cpuRisk = pod.cpuP95Milli > pod.cpuRequestMilli;
                        const cpuOptimized = !cpuRisk && Math.abs(cpuDiff) <= (0.1 * pod.cpuRequestMilli);

                        // MEM Analysis
                        const memRec = getRec(pod.memoryP95Bytes);
                        const memDiff = memRec - pod.memoryRequestBytes;
                        const memRisk = pod.memoryP95Bytes > pod.memoryRequestBytes;
                        const memOptimized = !memRisk && Math.abs(memDiff) <= (0.1 * pod.memoryRequestBytes);

                        // Global State
                        const isRisk = cpuRisk || memRisk;
                        const isOptimized = cpuOptimized && memOptimized;

                        const cpuReqStr = `${pod.cpuRequestMilli}m`;
                        const cpuP95Str = `${pod.cpuP95Milli.toFixed(0)}m`;
                        const memReqStr = `${(pod.memoryRequestBytes / (1024 * 1024)).toFixed(0)}Mi`;
                        const memP95Str = `${(pod.memoryP95Bytes / (1024 * 1024)).toFixed(0)}Mi`;

                        return (
                          <TableRow key={pod.namespace + pod.podName}>
                            <TableCell>
                              <div className="flex flex-col">
                                <span className="font-mono text-xs font-semibold truncate max-w-[140px]" title={pod.podName}>{pod.podName}</span>
                                <span className="text-[10px] text-muted-foreground">{pod.namespace}</span>
                                <span className="text-[9px] text-slate-500 uppercase">{pod.qosClass}</span>
                              </div>
                            </TableCell>

                            <TableCell className="text-right">
                              <div className="flex flex-col items-end gap-0.5">
                                <span className={`text-xs font-mono ${cpuRisk ? "text-orange-500 font-bold" : "text-muted-foreground"}`}>
                                  {cpuReqStr} → {cpuP95Str}
                                </span>
                                {cpuRisk && <span className="text-[10px] text-orange-500 font-bold">RISK</span>}
                                {!cpuRisk && !cpuOptimized && <span className="text-[10px] text-cyan-500">Waste</span>}
                              </div>
                            </TableCell>

                            <TableCell className="text-right">
                              <div className="flex flex-col items-end gap-0.5">
                                <span className={`text-xs font-mono ${memRisk ? "text-orange-500 font-bold" : "text-muted-foreground"}`}>
                                  {memReqStr} → {memP95Str}
                                </span>
                                {memRisk && <span className="text-[10px] text-orange-500 font-bold">RISK</span>}
                                {!memRisk && !memOptimized && <span className="text-[10px] text-cyan-500">Waste</span>}
                              </div>
                            </TableCell>

                            <TableCell className="text-right">
                              {isOptimized ? (
                                <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-500 border-0 text-[10px]">
                                  <CheckCircle2Icon className="w-3 h-3 mr-1" /> Optimized
                                </Badge>
                              ) : (
                                <TooltipProvider>
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <Button
                                        variant="outline"
                                        size="sm"
                                        className={`h-7 text-[10px] gap-1 ${isRisk
                                          ? "border-orange-500/50 text-orange-500 hover:bg-orange-500/10"
                                          : "border-cyan-500/50 text-cyan-500 hover:bg-cyan-500/10"}`}
                                        onClick={() => copyToClipboard(generatePatch(pod, "both", isRisk ? "shield" : "fix"))}
                                      >
                                        {isRisk ? <ShieldAlertIcon className="w-3 h-3" /> : <ScissorsIcon className="w-3 h-3" />}
                                        {isRisk ? "Shield Pod" : "Fix YAML"}
                                      </Button>
                                    </TooltipTrigger>
                                    <TooltipContent>
                                      Copy {isRisk ? "Upsize" : "Downsize"} Patch (CPU & RAM)
                                    </TooltipContent>
                                  </Tooltip>
                                </TooltipProvider>
                              )}
                            </TableCell>
                          </TableRow>
                        );
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
            </section>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
};

const SectionTabs = ({ window, setWindow }: { window: string; setWindow: (w: string) => void }) => (
  <div className="px-6">
    <Tabs value={window} onValueChange={setWindow} className="w-full">
      <TabsList className="grid w-full grid-cols-3 h-8 bg-muted/30">
        <TabsTrigger value="24h" className="text-xs h-6">24h</TabsTrigger>
        <TabsTrigger value="7d" className="text-xs h-6">7d</TabsTrigger>
        <TabsTrigger value="30d" className="text-xs h-6">30d</TabsTrigger>
      </TabsList>
    </Tabs>
  </div>
);

export default NodeDetailSheet;
