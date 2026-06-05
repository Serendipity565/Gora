import { defineConfig } from "vite";

// 后端默认监听 :8080，前端开发时通过 vite 代理转发 /api 与 /health。
// 生产构建会输出到 dist/，可由 `gora server --static frontend/dist` 直接挂载。
export default defineConfig({
  server: {
    port: 5173,
    strictPort: false,
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
      "/health": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: true,
  },
});
