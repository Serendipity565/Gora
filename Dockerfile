# syntax=docker/dockerfile:1.7

# ──────────────────────────────────────────────────────────────────────
# Stage 1: build
#   - 使用与 go.mod 一致的 Go 版本
#   - 关闭 CGO，产出静态二进制，便于在 alpine / distroless 中运行
# ──────────────────────────────────────────────────────────────────────
FROM golang:1.26.3-alpine AS builder

WORKDIR /src

# 先拷贝 module 描述文件，最大化利用 build cache
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# 再拷贝源码
COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/gora .

# ──────────────────────────────────────────────────────────────────────
# Stage 2: runtime
#   - alpine 而非 distroless，方便排查（带 sh / wget）
#   - 以非 root 用户运行
#   - 配置目录通过 docker-compose 挂载，敏感信息通过环境变量覆盖
# ──────────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S gora && adduser -S -G gora gora \
    && mkdir -p /app/config \
    && chown -R gora:gora /app

WORKDIR /app

COPY --from=builder /out/gora /app/gora

USER gora

EXPOSE 8080

# 健康检查走应用内置的 /health 接口
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/gora"]
CMD ["--config", "/app/config/config.yaml", "--addr", ":8080"]
