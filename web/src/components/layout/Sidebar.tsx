import { NavLink } from "react-router-dom";
import { cn } from "../../lib/utils";
import { Gauge, LayoutGrid, Server, PieChart, Network, Cloud } from "lucide-react";

const navItems = [
  { to: "/", label: "Overview", icon: Gauge },
  { to: "/namespaces", label: "Namespaces", icon: LayoutGrid },
  { to: "/nodes", label: "Nodes", icon: Server },
  { to: "/resources", label: "Resources", icon: PieChart },
  { to: "/agents", label: "Agents", icon: Network },
  { to: "/connect-cloud", label: "Connect Cloud", icon: Cloud }
];

const Sidebar = () => {
  return (
    <aside className="flex h-full w-64 flex-col border-r border-border bg-background/90">
      <div className="flex items-center gap-2 px-6 py-5 text-lg font-semibold">
        <span className="text-primary">💸 ClusterCost</span>
      </div>
      <nav className="flex-1 space-y-1 px-2">
        {navItems.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === "/"}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium",
                isActive ? "bg-primary/10 text-foreground" : "text-muted-foreground hover:bg-muted/40"
              )
            }
          >
            <Icon className="h-4 w-4" />
            {label}
          </NavLink>
        ))}
      </nav>
    </aside>
  );
};

export default Sidebar;
