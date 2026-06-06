# Repository Guidelines

## Project Structure & Module Organization

入口在 `cmd/gora/main.go`：用 stdlib `flag` 解析 `--config / --addr / --cors`，调 `initInfra()` 拿到 `*app.Infra` + cleanup（同包 wire 注入器），再交给 `app.Run` 启动并阻塞 HTTP 服务。所有业务代码均位于 `internal/` 之下，按"agent 框架 / 后端框架 / 数据访问 / 装配 / 配置"分层：

- `internal/agent/`：Agent 框架。
  - `core/`：Agent 接口、`BaseAgent`、`Event` 类型、工具授权 gate（`ToolPermissionGate` 等）。
  - `eino/`：基于 cloudwego/eino 的 `EinoAgent` 实现。
  - `llm/`：OpenAI 兼容的 LLM 客户端封装。
  - `tool/` + `tool/builtin/`：工具接口、注册表、内置工具（HTTP 等）。
- `internal/server/`：Gin 后端框架（参考 muxi/FeedBack 风格分层）。
  - `api/v1/{request,response}/`：DTO 定义。
  - `handler/`：controller 等价物（chat/agent/tool/model/health/permission）。
  - `service/`：业务编排（SSE chat 编排等）。
  - `middleware/`、`router/`、`sse/`：中间件、路由集中注册、SSE 写入器。
  - `server.go`：类型别名/函数转发，方便上层一键 import。
- `internal/repository/`：数据访问层。
  - `model/`：领域类型（`ChatMessage`、`ModelSelection`，无 GORM 标签）。
  - `dao/`：MySQL 持久化（GORM）。
  - `cache/`：Redis 短期记忆。
  - `repository.go`：聚合别名 + Open* 入口。
- `internal/app/`：装配 + 启动层。`app.go` 是 `Infra` 聚合类型；`runner.go` 是注册到 server.Handler 的多会话 Runner；`chat.go` 是 Agent 构建 / 历史读写助手；`server.go` 暴露 `Run(ctx, out, errOut, infra, cfg, opts)` 启动并阻塞 HTTP 服务；`options.go` 处理环境变量覆盖与 normalize 工具。
- `internal/ioc/`：基础设施 wire provider（`NewMySQL` / `NewRedis` / `NewToolRegistry`），聚合在 `ioc.ProviderSet` 中。
- 入口：`cmd/gora/{main.go, wire.go, wire_gen.go}` 同包（kratos 风格）。`main.go` 用 stdlib `flag` 解析 `--config / --addr / --cors`，调 `initInfra` 拿到 `*app.Infra` 与 cleanup，再交给 `app.Run`。修改任意 `ProviderSet` 后必须 `make wire` 重新生成 `wire_gen.go`。
- 项目**没有 cli 子命令**：单二进制单职责，启动 HTTP 服务器即可；任何额外能力通过 API 暴露，而不是新增 cobra 子命令。
- `internal/config/`：YAML 配置加载与校验。
- `configs/`：YAML 配置文件，`configs/config.example.yaml` 为模板，本地覆盖在 `configs/config.yaml`。
- `frontend/`：可选的 Vite + TS 前端。

测试文件与实现同目录（`*_test.go`）。

## Build, Test, and Development Commands
推荐通过 `make help` 查看完整目标列表。常用入口：
- `make server` 默认启动 Gin Web 服务（`:8080`，等价 `go run ./cmd/gora --config configs/config.yaml`）。
- `make frontend-dev` 启动 Vite dev server（`:5173`，`/api` 与 `/health` 自动代理到 `:8080`）；首次需先 `cd frontend && npm install`。
- `make frontend-build` 构建前端到 `frontend/dist`。
- `make test` 运行 `go test ./...`；需要覆盖率时直接 `go test ./... -cover`。
- `make wire` 重新生成 `cmd/gora/wire_gen.go`；改动任意 `ProviderSet` 后必跑。
- `docker compose up -d mysql redis` 给 storage 相关工作准备本地依赖。
- `gofmt -w $(rg --files -g '*.go')` 批量格式化所有 Go 源码。
- 覆盖默认变量：`make server CONFIG=configs/local.yaml ADDR=:9090`；复杂 flag 直接走 `go run ./cmd/gora`。

## Coding Style & Naming Conventions
Follow standard Go formatting and let `gofmt` decide indentation and spacing. Keep package names short and lowercase, exported identifiers in `CamelCase`, and error strings lowercase. Match existing config key patterns such as `max_history_messages` and `short_term_ttl`. User-facing CLI text is currently Chinese, so keep new prompts and messages consistent unless you are intentionally changing localization.

## Testing Guidelines
Add tests next to the implementation you change, for example `internal/app/chat_test.go` or `internal/repository/dao/dao_test.go`. Prefer table-driven tests for config parsing, selector logic, and storage edge cases. Name tests `TestXxx`. There is no enforced coverage gate, but touched packages should keep or improve coverage and should pass `go test ./...` before review.

## Commit & Pull Request Guidelines
Recent history uses short, imperative, lowercase commit subjects such as `add configuration files and Redis active memory store implementation`. Follow that style, keep each commit focused, and mention the subsystem when useful. PRs should describe behavior changes, config or schema impact, linked tasks, and the verification performed. Include terminal output or screenshots only when CLI behavior materially changes.

## Configuration & Security Tips
Do not commit real API keys, database credentials, or local DSNs. Prefer environment variables such as `DEEPSEEK_API_KEY`, `OPENAI_API_KEY`, `GORA_DATABASE_URL`, and `GORA_REDIS_ADDR`, or keep secrets in an untracked `configs/config.yaml`.
