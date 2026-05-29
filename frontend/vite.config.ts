import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    port: 5173,
    // Proxy API calls to the Go server so the SPA can use same-origin /api.
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  // `vite preview` (production build) needs its own proxy — it does not reuse server.proxy.
  preview: {
    port: 4173,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
