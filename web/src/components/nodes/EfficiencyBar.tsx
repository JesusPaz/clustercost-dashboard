import { Progress } from "@/components/ui/progress";
import { formatCurrency } from "../../lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

interface EfficiencyBarProps {
    usagePercent: number;
    requestPercent: number;
    costPerMonth: number;
    cpuCores?: string;
    className?: string; // Added to match usage
}

export function EfficiencyBar({ usagePercent, requestPercent, costPerMonth }: EfficiencyBarProps) {
    // FinOps Logic:
    // If Request >>> Usage, we have waste.
    // The "Gap" visually shows this.

    const wastePercent = Math.max(0, requestPercent - usagePercent);
    const wastedCost = costPerMonth * (wastePercent / 100);

    return (
        <div className="w-full min-w-[140px] flex flex-col gap-1.5 py-1">
            {/* Top Bar: Actual Usage (The "Real" Work) */}
            <div className="flex items-center justify-between text-[10px] leading-none mb-0.5">
                <span className="font-medium text-cyan-600 dark:text-cyan-400">Usage</span>
                <span className="font-mono text-muted-foreground">{usagePercent.toFixed(0)}%</span>
            </div>
            <Progress
                value={usagePercent}
                className="h-1.5 bg-muted/20"
                indicatorClassName="bg-cyan-500 shadow-[0_0_8px_rgba(6,182,212,0.5)]"
            />

            {/* Bottom Bar: Reserved / Requested (The "Billable" Reservation) */}
            <div className="flex items-center justify-between text-[10px] leading-none mt-1 mb-0.5">
                <span className="text-muted-foreground">Reserved</span>
                <span className="font-mono text-muted-foreground">{requestPercent.toFixed(0)}%</span>
            </div>
            <TooltipProvider delayDuration={0}>
                <Tooltip>
                    <TooltipTrigger asChild>
                        <div className="cursor-help">
                            <Progress
                                value={requestPercent}
                                className="h-1.5 bg-muted/20"
                                indicatorClassName="bg-primary"
                            />
                        </div>
                    </TooltipTrigger>
                    <TooltipContent side="bottom" className="text-xs max-w-[200px] bg-slate-950 border-slate-800">
                        <div className="space-y-1">
                            <p className="font-semibold border-b border-white/10 pb-1 mb-1 text-slate-200">Efficiency Gap</p>
                            <div className="flex justify-between gap-4 text-slate-300">
                                <span>Usage:</span>
                                <span className="font-mono text-cyan-400">{usagePercent.toFixed(1)}%</span>
                            </div>
                            <div className="flex justify-between gap-4 text-slate-300">
                                <span>Reserved:</span>
                                <span className="font-mono text-slate-400">{requestPercent.toFixed(1)}%</span>
                            </div>
                            {wastedCost > 1 && (
                                <div className="flex justify-between gap-4 pt-1 border-t border-white/10 text-red-400 font-bold">
                                    <span>Waste:</span>
                                    <span>{formatCurrency(wastedCost)}/mo</span>
                                </div>
                            )}
                        </div>
                    </TooltipContent>
                </Tooltip>
            </TooltipProvider>
        </div>
    );
}
