# Gora — Goroutine + Agent
#
# 常用入口：
#   make server          启动 Gin Web 服务（:8080）
#   make chat            启动命令行交互式对话
#   make frontend-dev    启动 Vite dev server（:5173，/api 代理到 :8080）
#   make frontend-build  构建前端到 frontend/dist
#   make test            运行 Go 单元测试

GO     ?= go
NPM    ?= npm
CONFIG ?= config/config.yaml
ADDR   ?= :8080

.PHONY: help server chat frontend-dev frontend-build test

help: ## 显示所有可用目标
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

server: ## 启动 Gin Web 服务（默认内嵌前端）
	$(GO) run . --config $(CONFIG) --addr $(ADDR)

chat: ## 启动命令行交互式 Agent 对话
	$(GO) run . chat --config $(CONFIG)

frontend-dev: ## 启动 Vite dev server（首次先 cd frontend && npm install）
	cd frontend && $(NPM) run dev

frontend-build: ## 构建前端到 frontend/dist
	cd frontend && $(NPM) run build

test: ## 运行 Go 单元测试
	$(GO) test ./...
