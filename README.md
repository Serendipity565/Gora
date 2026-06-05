# Gora — Goroutine + Agent

> 用 Go 原生并发能力驱动的 Agent 框架。每个 Agent 就是一个 goroutine，天生并行、独立决策、协作执行。

Gora 把 LLM Agent 的执行过程做成一颗能直接观察的"心脏"：思考（thinking）、工具调用（tool_call）、工具返回（tool_result）、流式输出（chunk）全部以 SSE 事件实时流到前端，方便调试和演示。

```
┌────────────────────────────────────────────────────────────┐
│                    前端 (frontend/, web/static/)            │
│   Vite + TypeScript SPA  ⇆  内嵌单页（无需 npm 也能跑）     │
└──────────────────┬─────────────────────────────────────────┘
                   │ HTTP / SSE
┌──────────────────▼─────────────────────────────────────────┐
│                       Gin Web 层 (web/)                     │
│   /api/chat/:agentId · /api/agents · /api/tools · /health   │
└──────────────────┬─────────────────────────────────────────┘
                   │
┌──────────────────▼─────────────────────────────────────────┐
│                      Agent 核心 (agent/)                    │
│   BaseAgent / EinoAgent · Event 流 · Session 历史            │
├──────────┬──────────────────┬───────────────────────────────┤
│  llm/    │   tool/ +        │   storage/                    │
│  OpenAI  │   tool/builtin/  │   MySQL 历史 · Redis 短期记忆 │
│  兼容    │   工具注册表     │                               │
└──────────┴──────────────────┴───────────────────────────────┘
```

---

## 项目结构

```
.
├── cmd/
│   └── gora/main.go              # 程序入口，调用 internal/cli.Execute
├── internal/
│   ├── agent/                    # Agent 框架
│   │   ├── core/                 # Agent 接口、BaseAgent、Event、ToolPermissionGate
│   │   ├── eino/                 # 基于 cloudwego/eino 的 EinoAgent 实现
│   │   ├── llm/                  # OpenAI 兼容的 LLM 客户端
│   │   └── tool/{,builtin/}      # 工具接口、注册表与内置工具（HTTP 等）
│   ├── server/                   # Gin 后端框架
│   │   ├── api/v1/{request,response}/  # API DTO
│   │   ├── handler/              # controller 等价物
│   │   ├── service/              # 业务编排（SSE chat 流）
│   │   ├── middleware/           # Gin 中间件（CORS 等）
│   │   ├── router/               # 集中路由注册
│   │   ├── sse/                  # Server-Sent Events 写入器
│   │   └── server.go             # 类型别名 + NewRouter / NewHandler 转发
│   ├── repository/               # 数据访问层
│   │   ├── model/                # 领域类型（ChatMessage、ModelSelection）
│   │   ├── dao/                  # MySQL 持久化（GORM）
│   │   ├── cache/                # Redis 短期记忆
│   │   └── repository.go         # 别名 + Open* 入口
│   ├── app/                      # 聚合类型 Infra（Registry/DB/Cache）
│   ├── ioc/                      # 基础设施 wire provider（MySQL/Redis/Registry）
│   ├── wired/                    # wire 注入器（wire.go + 生成的 wire_gen.go）
│   ├── cli/                      # Cobra 命令（root / serve / chat）
│   └── config/                   # YAML 配置加载与校验
├── configs/                      # YAML 配置文件
│   ├── config.example.yaml       # 模板
│   └── config.yaml               # 本地覆盖（被 .gitignore）
├── frontend/                     # 可选的 Vite + TS 前端
├── docker-compose.yml            # 本地 MySQL + Redis
├── Makefile                      # 一键启动 / 构建 / 测试
└── AGENTS.md / Phase.md / PLAN.md
```

---

## 快速开始

### 0. 准备配置

```bash
cp configs/config.example.yaml configs/config.yaml
# 编辑 configs/config.yaml，填入你的 LLM API Key（或通过环境变量注入）
```

环境变量也行（推荐）：

| 变量 | 用途 |
|---|---|
| `DEEPSEEK_API_KEY` / `OPENAI_API_KEY` | LLM API Key（按 provider 自动读取） |
| `GORA_DATABASE_URL` | MySQL DSN，覆盖配置文件 |
| `GORA_REDIS_ADDR` / `GORA_REDIS_PASSWORD` / `GORA_REDIS_DB` | Redis 连接 |
| `GORA_SHORT_TERM_MEMORY_TTL` | 短期记忆过期时间，如 `24h` |

### 1. 启动 MySQL + Redis

```bash
docker compose up -d mysql redis
```

### 2. 启动后端 Web 服务（最快路径）

```bash
make server
# 等价于：go run ./cmd/gora --config configs/config.yaml --addr :8080
```

打开 <http://localhost:8080> 即可看到内嵌的 Playground 页面。

### 3. CLI 对话模式

```bash
make chat
# 等价于：go run ./cmd/gora chat --config configs/config.yaml
```

### 4. 前端开发模式（带 HMR）

```bash
# 终端 1：后端
make server

# 终端 2：Vite dev server
cd frontend && npm install   # 首次运行需要装依赖
make frontend-dev            # http://localhost:5173，/api 与 /health 自动代理到 :8080
```

### 5. 一体化生产预览

```bash
make frontend-build
go run ./cmd/gora --config configs/config.yaml --static frontend/dist
```

---

## Makefile 速查

```text
make help            # 列出所有目标
make server          # 启动 Gin Web 服务（内嵌前端）
make chat            # 命令行 Agent 对话
make frontend-dev    # Vite dev (5173)
make frontend-build  # 构建到 frontend/dist
make test            # go test ./...
make wire            # 重新生成 internal/wired/wire_gen.go
```

可覆盖变量：

```bash
make server CONFIG=configs/local.yaml ADDR=:9090
```

复杂参数（如 `--model`、`--max-history`）直接走原生命令：

```bash
go run ./cmd/gora --config configs/config.yaml --model gpt-4o-mini --max-history 50
```

---

## CLI 参数（root + chat 共享）

| Flag | 说明 |
|---|---|
| `--config` | YAML 配置路径，默认 `configs/config.yaml` |
| `--api-key` | 覆盖所有模型的 API Key |
| `--base-url` | 覆盖首个模型的 OpenAI 兼容接口地址 |
| `--model` | 覆盖首个模型名 |
| `--agent-id` | 覆盖配置中的 Agent ID |
| `--user-id` | 模型选择持久化使用的用户 ID（默认 `local`） |
| `--database-url` | 覆盖 MySQL DSN |
| `--redis-addr` / `--redis-password` / `--redis-db` | 覆盖 Redis 连接 |
| `--short-term-memory-ttl` | 覆盖短期记忆过期时间 |
| `--max-history` | 覆盖历史消息保留数 |
| `--max-chunk-runes` | 覆盖单个流式事件字符数 |

仅 web 服务（root 命令）特有：

| Flag | 说明 |
|---|---|
| `--addr` | 监听地址，默认 `:8080` |
| `--static` | 用指定目录覆盖内嵌前端（用于挂载 `frontend/dist`） |
| `--cors` | 是否开启简单 CORS（默认开启） |

---

## HTTP / SSE 接口

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET`  | `/health` | 健康检查 |
| `GET`  | `/api/agents` | 列出可用 Agent |
| `GET`  | `/api/agents/:agentId` | 单个 Agent 状态 |
| `GET`  | `/api/tools` | 已注册工具元信息 |
| `POST` | `/api/chat/:agentId` | 发起对话，返回 SSE 流（`thinking` / `tool_call` / `tool_result` / `chunk` / `done` / `error`） |
| `GET`  | `/` | 前端入口（内嵌或 `--static`） |

---

## 测试

```bash
make test             # 全量单元测试
go test ./... -cover  # 需要覆盖率时直接走原生命令
```

测试文件与实现同目录（`*_test.go`），新增功能请同步补测试。
