import { NavLink, useLocation } from "react-router-dom";
import { Cloud, Gauge, GitBranch, LayoutGrid, Network, PieChart, Server } from "lucide-react";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator
} from "@/components/ui/sidebar";

const navItems = [
  { to: "/", label: "Overview", icon: Gauge },
  { to: "/namespaces", label: "Namespaces", icon: LayoutGrid },
  { to: "/nodes", label: "Nodes", icon: Server },
  { to: "/resources", label: "Resources", icon: PieChart },
  { to: "/network", label: "Network Map", icon: GitBranch },
  { to: "/agents", label: "Agents", icon: Network },
  { to: "/connect-cloud", label: "Connect Cloud", icon: Cloud }
];

const AppSidebar = () => {
  const location = useLocation();

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <div className="flex items-center gap-2 text-lg font-semibold">
          <span className="text-primary">💸 ClusterCost</span>
          <span className="rounded-full bg-[hsl(var(--sidebar-accent))] px-2 py-0.5 text-[11px] font-medium text-[hsl(var(--sidebar-accent-foreground))]">
            OSS
          </span>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Navigation</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map((item) => {
                const Icon = item.icon;
                const isActive = location.pathname === item.to;
                return (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton asChild isActive={isActive}>
                      <NavLink to={item.to} className="flex items-center gap-3">
                        <Icon className="h-4 w-4 shrink-0" />
                        <span className="truncate group-data-[state=collapsed]/sidebar:hidden">{item.label}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarSeparator />
        <SidebarGroup className="group-data-[state=collapsed]/sidebar:hidden">
          <SidebarGroupLabel>Status</SidebarGroupLabel>
          <SidebarGroupContent className="text-sm text-[hsl(var(--sidebar-foreground))/0.8]">
            <p>Cluster: <span className="font-medium">Active</span></p>
            <p>Agents: <span className="font-medium">Connected</span></p>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <p className="text-xs text-[hsl(var(--sidebar-foreground))/0.6] group-data-[state=collapsed]/sidebar:hidden">
          ClusterCost OSS Dashboard
        </p>
      </SidebarFooter>
    </Sidebar>
  );
};

export default AppSidebar;
