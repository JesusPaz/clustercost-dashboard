import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "path"

export default defineConfig(({ mode }) => {
  // Load env file based on `mode` in the current working directory.
  // Set the third parameter to '' to load all env regardless of the `VITE_` prefix.
  const env = loadEnv(mode, process.cwd(), '');

  // Priority: Shell Env (process.env) > .env File (env) > Default
  const apiTarget = process.env.VITE_API_TARGET || env.VITE_API_TARGET || "http://localhost:9090";

  console.log(`[Vite] Proxying /api to: ${apiTarget}`);

  return {
    plugins: [react()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      port: 5173,
      proxy: {
        "/api": {
          target: apiTarget,
          changeOrigin: true,
          // If you want to strip /api prefix from the request when proxying:
          // rewrite: (path) => path.replace(/^\/api/, ''),
        }
      }
    },
    build: {
      outDir: "dist",
      emptyOutDir: true
    },
    test: {
      globals: true,
      environment: "jsdom",
      setupFiles: "./src/test/setup.ts"
    }
  };
});
