import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const proxyTarget = process.env.VITE_API_PROXY_TARGET || "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/v1": {
        target: proxyTarget,
        changeOrigin: true
      },
      "/healthz": {
        target: proxyTarget,
        changeOrigin: true
      },
      "/openapi.json": {
        target: proxyTarget,
        changeOrigin: true
      }
    }
  }
});
