# Gora — Goroutine + Agent
#
# 后端是纯 API 服务，前端是独立的 Vite 项目，二者分开启动：
#   make server          启动后端 Gin API 服务（:8080，无 UI）
#   make chat            启动命令行交互式对话
#   make frontend-install 安装前端依赖（首次或依赖变更）
#   make frontend-dev    启动前端 Vite dev server（:5173，/api & /health 代理到后端）
#   make frontend-build  构建前端产物到 frontend/dist/，可由任意静态服务器托管
#   make build           只构建后端二进制（前端独立部署，不再嵌入）
#   make test            运行 Go 单元测试

CONFIG ?= config/config.yaml
ADDR   ?= :8080

.PHONY: help server chat frontend-install frontend-dev frontend-build build test

help: ## 显示所有可用目标
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

server: ## 启动后端 Gin API 服务（无 UI，前端请用 frontend-dev）
	go run . --config $(CONFIG) --addr $(ADDR)

chat: ## 启动命令行交互式 Agent 对话
	go run . chat --config $(CONFIG)

frontend-install: ## 安装前端依赖（首次或依赖变更时运行）
	cd frontend && npm install

frontend-dev: ## 启动前端 Vite dev server，与后端联调
	cd frontend && npm run dev

frontend-build: ## 构建前端到 frontend/dist/，可由静态服务器独立托管
	cd frontend && npm run build

build: ## 构建后端二进制（不再嵌入前端）
	go build -o bin/gora .

test: ## 运行 Go 单元测试
	go test ./...
