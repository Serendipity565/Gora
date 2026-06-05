# syntax=docker/dockerfile:1.7

# ──────────────────────────────────────────────────────────────────────
# Stage 1: build
#   - 用 npm ci 安装确定性依赖（前提是 lockfile 已提交）
#   - npm run build 产出 dist/
# ──────────────────────────────────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY . .

RUN npm run build

# ──────────────────────────────────────────────────────────────────────
# Stage 2: runtime
#   - nginx:alpine 静态托管 dist/
#   - /api 与 /health 反代到 backend 服务（compose 服务名）
# ──────────────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine AS runtime

# 自定义站点配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 拷贝构建产物
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1

CMD ["nginx", "-g", "daemon off;"]
