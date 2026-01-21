import { useMemo, useState } from "react";
import { fetchResources } from "../../lib/api";
import { useApiData } from "../../hooks/useApiData";
import {
  bytesToGiB,
  formatCurrency,
  formatPercentage,
  milliToCores,
  relativeTimeFromIso,
  toMonthlyCost,
  type Environment
} from "../../lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Badge } from "@/components/ui/badge";
import { ResponsiveContainer, Bar, BarChart, Legend, Tooltip, XAxis, YAxis } from "recharts";

interface WasteItem {
  namespace: string;
  environment: Environment;
  cpuWastePercent: number;
  memoryWastePercent: number;
  estimatedMonthlyWasteCost: number;
}

const formatWasteLabel = (value: number) => `${value.toFixed(0)}%`;

const ResourcesPage = () => {
  const { data, loading, error, refresh } = useApiData(fetchResources);
  const [selected, setSelected] = useState<WasteItem | null>(null);

  const cpuMetrics = useMemo(() => {
    if (!data) {
      return {
        usageCores: 0,
        requestCores: 0,
        efficiency: 0,
        wastePercent: 0,
        wasteCost: 0
      };
    }
    const usageCores = milliToCores(data.cpu.usageMilli ?? 0);
    const requestCores = milliToCores(data.cpu.requestMilli ?? 0);
    const efficiency = data.cpu.efficiencyPercent ?? (data.cpu.requestMilli > 0 ? (data.cpu.usageMilli / data.cpu.requestMilli) * 100 : 0);
    const wastePercent = Math.max(0, 100 - efficiency);
    const wasteCost = toMonthlyCost(data.cpu.estimatedHourlyWasteCost ?? 0);
    return { usageCores, requestCores, efficiency, wastePercent, wasteCost };
  }, [data]);

  const memoryMetrics = useMemo(() => {
    if (!data) {
      return {
        usageGiB: 0,
        requestGiB: 0,
        efficiency: 0,
        wastePercent: 0,
        wasteCost: 0
      };
    }
    const usageGiB = bytesToGiB(data.memory.usageBytes ?? 0);
    const requestGiB = bytesToGiB(data.memory.requestBytes ?? 0);
    const efficiency = data.memory.efficiencyPercent ?? (data.memory.requestBytes > 0 ? (data.memory.usageBytes / data.memory.requestBytes) * 100 : 0);
    const wastePercent = Math.max(0, 100 - efficiency);
    const wasteCost = toMonthlyCost(data.memory.estimatedHourlyWasteCost ?? 0);
    return { usageGiB, requestGiB, efficiency, wastePercent, wasteCost };
  }, [data]);

  const chartData = useMemo(() => {
    if (!data) return [];
    return [
      { name: "CPU", Used: cpuMetrics.usageCores, Requested: cpuMetrics.requestCores },
      { name: "Memory", Used: memoryMetrics.usageGiB, Requested: memoryMetrics.requestGiB }
    ];
  }, [data, cpuMetrics, memoryMetrics]);

  const savings = useMemo(() => {
    if (!data) return [];
    return [...(data.namespaceWaste ?? [])]
      .map((entry) => ({
        namespace: entry.namespace,
        environment: entry.environment,
        cpuWastePercent: entry.cpuWastePercent,
        memoryWastePercent: entry.memoryWastePercent,
        estimatedMonthlyWasteCost: toMonthlyCost(entry.estimatedHourlyWasteCost ?? 0)
      }))
      .sort((a, b) => b.estimatedMonthlyWasteCost - a.estimatedMonthlyWasteCost)
      .slice(0, 4);
  }, [data]);

  const lastUpdated = data?.timestamp ? relativeTimeFromIso(data.timestamp) : "moments ago";

  if (loading && !data) {
    return <Skeleton className="h-[70vh] w-full" />;
  }

  if (error) {
    return (
      <Card className="border-destructive/40 bg-destructive/10">
        <CardContent className="py-10 text-center text-sm text-destructive">{error}</CardContent>
      </Card>
    );
  }

  if (!data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>No resource data</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          <p>We haven’t received cluster resource metrics yet.</p>
          <Button variant="outline" onClick={refresh} className="w-fit">
            Refresh
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Resources</h1>
          <p className="text-sm text-muted-foreground">See how your cluster uses CPU and memory.</p>
        </div>
        <div className="text-sm text-muted-foreground">Last updated {lastUpdated}</div>
      </header>

      <section className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">CPU efficiency</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm font-semibold">
              {cpuMetrics.usageCores.toFixed(1)} cores used / {cpuMetrics.requestCores.toFixed(1)} cores requested
            </p>
            <p className="text-2xl font-semibold mt-1">{formatPercentage(cpuMetrics.efficiency, { fractionDigits: 0 })}</p>
            <Progress value={cpuMetrics.efficiency} className="mt-3" />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">Memory efficiency</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm font-semibold">
              {memoryMetrics.usageGiB.toFixed(1)} GiB used / {memoryMetrics.requestGiB.toFixed(1)} GiB requested
            </p>
            <p className="text-2xl font-semibold mt-1">{formatPercentage(memoryMetrics.efficiency, { fractionDigits: 0 })}</p>
            <Progress value={memoryMetrics.efficiency} className="mt-3" />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">CPU waste cost</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">{formatCurrency(cpuMetrics.wasteCost)}</p>
            <p className="text-xs text-muted-foreground">
              ~{formatPercentage(cpuMetrics.wastePercent, { fractionDigits: 0 })} of node spend
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">Memory waste cost</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">{formatCurrency(memoryMetrics.wasteCost)}</p>
            <p className="text-xs text-muted-foreground">
              ~{formatPercentage(memoryMetrics.wastePercent, { fractionDigits: 0 })} of node spend
            </p>
          </CardContent>
        </Card>
      </section>

      <section className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Usage vs requests</CardTitle>
            <p className="text-xs text-muted-foreground">CPU and memory compared side by side</p>
          </CardHeader>
          <CardContent>
            <div className="h-64">
              {chartData.length > 0 ? (
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={chartData} layout="vertical" margin={{ left: 40, right: 16, top: 10, bottom: 10 }}>
                    <XAxis type="number" hide />
                    <YAxis type="category" dataKey="name" width={80} tick={{ fill: "#94a3b8", fontSize: 12 }} />
                    <Legend />
                    <Tooltip contentStyle={{ backgroundColor: "#0f172a", border: "1px solid #1e293b" }} />
                    <Bar dataKey="Requested" fill="#475569" barSize={18} radius={[0, 4, 4, 0]} />
                    <Bar dataKey="Used" fill="#38bdf8" barSize={18} radius={[0, 4, 4, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                  No usage data yet.
                </div>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Savings opportunities</CardTitle>
            <p className="text-xs text-muted-foreground">Focus on these namespaces first</p>
          </CardHeader>
          <CardContent className="space-y-3">
            {savings.length === 0 ? (
              <p className="text-sm text-muted-foreground">No obvious waste right now. Keep monitoring.</p>
            ) : (
              savings.map((item) => (
                <button
                  key={item.namespace}
                  className="w-full rounded border border-border/60 p-3 text-left transition hover:border-primary/40"
                  onClick={() => setSelected(item)}
                >
                  <div className="flex items-center justify-between text-sm">
                    <span className="font-semibold">{item.namespace}</span>
                    <span>{formatCurrency(item.estimatedMonthlyWasteCost)}</span>
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">
                    CPU waste {formatWasteLabel(item.cpuWastePercent)} · Mem waste {formatWasteLabel(item.memoryWastePercent)}
                  </p>
                  <p className="text-xs text-muted-foreground">~{formatCurrency(item.estimatedMonthlyWasteCost)}/month potential</p>
                </button>
              ))
            )}
          </CardContent>
        </Card>
      </section>

      <Sheet open={!!selected} onOpenChange={(open) => !open && setSelected(null)}>
        <SheetContent className="w-full sm:max-w-md">
          <SheetHeader>
            <SheetTitle>{selected?.namespace}</SheetTitle>
            <SheetDescription>Waste snapshot</SheetDescription>
          </SheetHeader>
          {selected && (
            <div className="mt-6 space-y-4 text-sm">
              <div>
                <p className="text-xs uppercase text-muted-foreground">Potential savings</p>
                <p className="text-2xl font-semibold">{formatCurrency(selected.estimatedMonthlyWasteCost)}</p>
              </div>
              <div>
                <p className="text-xs uppercase text-muted-foreground">CPU waste</p>
                <Progress value={Math.min(selected.cpuWastePercent, 100)} className="mt-2" />
                <p className="text-xs text-muted-foreground mt-1">{formatWasteLabel(selected.cpuWastePercent)}</p>
              </div>
              <div>
                <p className="text-xs uppercase text-muted-foreground">Memory waste</p>
                <Progress value={Math.min(selected.memoryWastePercent, 100)} className="mt-2" />
                <p className="text-xs text-muted-foreground mt-1">{formatWasteLabel(selected.memoryWastePercent)}</p>
              </div>
              <Badge variant="outline" className="text-xs">Action: right-size requests</Badge>
            </div>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
};

export default ResourcesPage;
