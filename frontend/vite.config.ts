import { defineConfig } from "vite";

// 后端是纯 API 服务，前端在独立的 vite 项目里运行：
//   - 开发：npm run dev → http://localhost:5173，/api 与 /health 通过 proxy 转给后端 :8080
//   - 部署：npm run build → frontend/dist/，由静态服务器（nginx / vercel / netlify 等）独立托管
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
