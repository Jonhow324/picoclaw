import path from "path"

import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// Read base path from environment variable, default to "/" for root-path deployments.
// Example: VITE_BASE_URL=/picoclaw/ pnpm build:backend
const BASE_URL = process.env.VITE_BASE_URL || "/"

// Derive the path prefix without trailing slash for proxy key matching.
// When BASE_URL is "/" this is an empty string, keeping proxy keys identical
// to the existing "/api", "/pico/media", "/pico/ws" behaviour.
const basePath = BASE_URL === "/" ? "" : BASE_URL.replace(/\/$/, "")

// https://vite.dev/config/
export default defineConfig({
  base: BASE_URL,
  plugins: [
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    chunkSizeWarningLimit: 2048,
  },
  server: {
    proxy: {
      [`${basePath}/api`]: {
        target: "http://localhost:18800",
        changeOrigin: true,
        rewrite: basePath
          ? (p) => p.replace(new RegExp(`^${basePath}`), "")
          : undefined,
      },
      [`${basePath}/pico/media`]: {
        target: "http://localhost:18800",
        changeOrigin: true,
        rewrite: basePath
          ? (p) => p.replace(new RegExp(`^${basePath}`), "")
          : undefined,
      },
      [`${basePath}/pico/ws`]: {
        target: "ws://localhost:18800",
        ws: true,
        rewrite: basePath
          ? (p) => p.replace(new RegExp(`^${basePath}`), "")
          : undefined,
      },
    },
  },
})
