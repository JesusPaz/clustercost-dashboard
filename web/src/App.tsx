import { Outlet } from "react-router-dom";
import LayoutShell from "./components/layout/LayoutShell";
import { AuthProvider } from "./context/AuthContext";

const App = () => {
  return (
    <AuthProvider>
      <LayoutShell>
        <Outlet />
      </LayoutShell>
    </AuthProvider>
  );
};

export default App;
