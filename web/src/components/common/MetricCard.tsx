import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { cn } from "../../lib/utils";

interface MetricCardProps {
    title: string;
    value: string;
    subtext?: string;
    trend?: string;
    trendUp?: boolean;
    className?: string;
    valueClassName?: string;
}

export function MetricCard({ title, value, subtext, trend, trendUp, className, valueClassName }: MetricCardProps) {
    return (
        <Card className={cn("bg-card/50 backdrop-blur-sm border-muted/50", className)}>
            <CardHeader className="p-4 pb-2 space-y-0">
                <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {title}
                </CardTitle>
            </CardHeader>
            <CardContent className="p-4 pt-0">
                <div className={cn("text-2xl font-bold font-mono tracking-tight", valueClassName)}>
                    {value}
                </div>
                {(subtext || trend) && (
                    <div className="mt-1 flex items-center text-xs text-muted-foreground">
                        {trend && (
                            <span className={cn("mr-2 font-medium", trendUp ? "text-emerald-500" : "text-rose-500")}>
                                {trend}
                            </span>
                        )}
                        {subtext}
                    </div>
                )}
            </CardContent>
        </Card>
    );
}
