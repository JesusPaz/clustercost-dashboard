import { createBrowserRouter } from "react-router-dom";
import App from "./App";
import OverviewPage from "./pages/overview/OverviewPage";
import NamespacesPage from "./pages/namespaces/NamespacesPage";
import NodesPage from "./pages/nodes/NodesPage";
import ResourcesPage from "./pages/resources/ResourcesPage";
import AgentsPage from "./pages/AgentsPage";
import ConnectCloudPage from "./pages/ConnectCloudPage";
import LoginPage from "./pages/LoginPage";
import NetworkTopologyPage from "./pages/network/NetworkTopologyPage";
import ProtectedRoute from "./components/layout/ProtectedRoute";
import RouteError from "./components/layout/RouteError";

const router = createBrowserRouter([
  {
    path: "/",
    element: <App />,
    errorElement: <RouteError />,
    children: [
      {
        path: "login",
        element: <LoginPage />
      },
      {
        element: <ProtectedRoute />,
        children: [
          { index: true, element: <OverviewPage /> },
          { path: "namespaces", element: <NamespacesPage /> },
          { path: "nodes", element: <NodesPage /> },
          { path: "resources", element: <ResourcesPage /> },
          { path: "network", element: <NetworkTopologyPage /> },
          { path: "agents", element: <AgentsPage /> },
          { path: "connect-cloud", element: <ConnectCloudPage /> }
        ]
      }
    ]
  }
]);

export default router;
