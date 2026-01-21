import { Badge } from "@/components/ui/badge";
import { formatCurrency } from "../../lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

interface EfficiencyBarProps {
    usagePercent: number;
    requestPercent: number;
    costPerMonth: number;
    usageAbsolute: number;
    totalAbsolute: number;
    unit: string;
}

export function EfficiencyBar({
    usagePercent,
    requestPercent,
    costPerMonth,
    usageAbsolute,
    totalAbsolute,
    unit
}: EfficiencyBarProps) {
    // FinOps Logic:
    // "Comfort Zone" Strategy:
    // - Container = Total Node Capacity
    // - Reserved = Light Rail (bg-white/15)
    // - Threshold = White Line (Always visible on top)
    // - Logic:
    //   - < 90% of Reserved: Gap (Cyan)
    //   - 90% - 110% of Reserved: Optimized (Green Badge, Cyan Bar)
    //   - > 110% of Reserved: Risk (Orange Bar, Orange Text)

    const ratio = requestPercent > 0 ? usagePercent / requestPercent : 0;

    // Logic Refinement (Hybrid Absolute/Relative):
    // 1. High Risk (Orange): STRICTLY > 110% of reservation (Relative > 1.1).
    // 2. Optimized (Green): Not High Risk AND Absolute Difference <= 10% (suppress "Gap: 3%" noise).
    // 3. Gap (Cyan): Everything else (Absolute Difference > 10%).

    const isHighRisk = ratio > 1.1;
    const diff = Math.abs(usagePercent - requestPercent);
    const isOptimized = !isHighRisk && diff <= 10;

    const wastePercent = Math.max(0, requestPercent - usagePercent);
    const overflowPercent = Math.max(0, usagePercent - requestPercent);
    const wastedCost = costPerMonth * (wastePercent / 100);

    return (
        <div className="w-full min-w-[140px] flex flex-col gap-1 py-1">
            {/* Micro-Text Label Row */}
            <div className="flex justify-between items-end px-0.5 font-mono">
                <span className="text-[10px] text-muted-foreground">
                    <span className="font-bold text-foreground">{usageAbsolute.toFixed(1)}</span>
                    <span className="opacity-70"> / {totalAbsolute.toFixed(1)} {unit}</span>
                </span>

                {/* Status Indicator */}
                {isOptimized ? (
                    <Badge variant="outline" className="text-[9px] h-3.5 px-1.5 py-0 border-emerald-500/40 text-emerald-500 bg-emerald-500/5 hover:bg-emerald-500/10 font-sans tracking-wide">
                        Optimized
                    </Badge>
                ) : isHighRisk ? (
                    <span className="text-[10px] text-orange-500 font-bold tracking-tight">
                        Risk: +{overflowPercent.toFixed(0)}%
                    </span>
                ) : wastePercent > 0 ? (
                    <span className="text-[10px] text-cyan-500 font-bold tracking-tight">
                        Gap: {wastePercent.toFixed(0)}%
                    </span>
                ) : null}
            </div>

            {/* Bar Container */}
            <TooltipProvider delayDuration={0}>
                <Tooltip>
                    <TooltipTrigger asChild>
                        {/* Layer 0: Total Capacity Container */}
                        <div className="relative h-3 w-full bg-muted/10 rounded-sm overflow-hidden cursor-help ring-1 ring-white/5">

                            {/* Layer 1: Reserved (The Contract Rail) */}
                            {/* Lighter grey to contrast with dark background */}
                            <div
                                className="absolute top-0 left-0 h-full bg-white/15 z-0"
                                style={{ width: `${Math.min(requestPercent, 100)}%` }}
                            />

                            {/* Layer 2: Actual Usage (The Active Liquid) */}
                            {/* Sits ON TOP of Reserved. */}
                            <div
                                className={`absolute top-0 left-0 h-full shadow-sm z-10 transition-all duration-500 ${isHighRisk ? "bg-orange-500 shadow-[0_0_12px_rgba(249,115,22,0.6)]" : "bg-cyan-500 shadow-[0_0_8px_rgba(6,182,212,0.6)]"}`}
                                style={{ width: `${Math.min(usagePercent, 100)}%` }}
                            />

                            {/* Layer 3: The Contract Line (Threshold) */}
                            {/* ALWAYS visible, white, sits on top of everything (z-20) */}
                            {requestPercent > 0 && (
                                <div
                                    className="absolute top-0 h-full w-[1.5px] bg-white z-20 shadow-[0_0_2px_rgba(0,0,0,0.5)]"
                                    style={{ left: `${Math.min(requestPercent, 100)}%` }}
                                />
                            )}
                        </div>
                    </TooltipTrigger>
                    <TooltipContent side="bottom" className="text-xs max-w-[200px] bg-slate-950 border-slate-800 font-mono">
                        <div className="space-y-1">
                            <p className="font-semibold border-b border-white/10 pb-1 mb-1 text-slate-200 font-sans">
                                {isOptimized ? "State: Optimized" : isHighRisk ? "Stability Risk" : "Efficiency Gap"}
                            </p>
                            <div className="flex justify-between gap-4 text-slate-300">
                                <span>Usage:</span>
                                <span className={isHighRisk ? "text-orange-400 font-bold" : "text-cyan-400 font-bold"}>
                                    {usagePercent.toFixed(1)}% ({usageAbsolute.toFixed(2)} {unit})
                                </span>
                            </div>
                            <div className="flex justify-between gap-4 text-slate-300">
                                <span>Reserved:</span>
                                <span className="text-slate-400">{requestPercent.toFixed(1)}%</span>
                            </div>
                            <div className="flex justify-between gap-4 text-slate-300">
                                <span>Total:</span>
                                <span className="text-slate-500">{totalAbsolute.toFixed(1)} {unit}</span>
                            </div>
                            {!isHighRisk && !isOptimized && wastedCost > 1 && (
                                <div className="flex justify-between gap-4 pt-1 border-t border-white/10 text-cyan-400 font-bold">
                                    <span>Waste:</span>
                                    <span>{formatCurrency(wastedCost)}/mo</span>
                                </div>
                            )}
                            {isOptimized && (
                                <div className="pt-1 border-t border-white/10 text-emerald-400 text-[10px] leading-tight mt-1">
                                    Usage is within 10% of reservation. Perfect balance.
                                </div>
                            )}
                        </div>
                    </TooltipContent>
                </Tooltip>
            </TooltipProvider>
        </div>
    );
}
