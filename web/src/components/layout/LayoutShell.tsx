import AppSidebar from "./AppSidebar";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { useAuth } from "@/context/AuthContext";

const LayoutShell = ({ children }: { children: React.ReactNode }) => {
  const { isAuthenticated } = useAuth();

  if (!isAuthenticated) {
    return <main className="min-h-screen bg-background text-foreground">{children}</main>;
  }

  return (
    <SidebarProvider>
      <div className="flex min-h-screen bg-background text-foreground">
        <AppSidebar />
        <SidebarInset className="flex flex-1 flex-col">
          <main className="flex-1 overflow-y-auto bg-background/40 p-6">{children}</main>
        </SidebarInset>
      </div>
    </SidebarProvider>
  );
};

export default LayoutShell;
