import { useCallback, useEffect, useState } from "react";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";
import { Skeleton } from "../ui/skeleton";
import { fetchHealth, type HealthResponse } from "../../lib/api";

const LayoutShell = ({ children }: { children: React.ReactNode }) => {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setLoading(true);
      const data = await fetchHealth();
      setHealth(data);
    } catch (err) {
      console.error("Failed to load health", err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <Sidebar />
      <div className="flex flex-1 flex-col">
        {health ? (
          <TopBar
            status={health.status}
            clusterName={health.clusterName}
            clusterType={health.clusterType}
            clusterRegion={health.clusterRegion}
            version={health.version}
            timestamp={health.timestamp}
            onRefresh={refresh}
          />
        ) : (
          <div className="border-b border-border p-4">
            <Skeleton className="h-8 w-64" />
          </div>
        )}
        <main className="flex-1 overflow-y-auto bg-background/40 p-6">
          {loading && (
            <p className="mb-2 text-xs text-muted-foreground">Updating…</p>
          )}
          {children}
        </main>
      </div>
    </div>
  );
};

export default LayoutShell;
