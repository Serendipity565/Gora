# Repository Guidelines

## Project Structure & Module Organization
`main.go` boots the Cobra CLI. `cmd/` contains command wiring and the interactive chat flow. `agent/` holds agent implementations and event handling. `llm/` wraps OpenAI-compatible model clients. `tool/` and `tool/builtin/` define tool interfaces and built-in tools. `storage/` contains MySQL-backed history/model selection and Redis-backed short-term memory. `config/` loads YAML settings; start from `config/config.example.yaml` and keep local overrides in `config/config.yaml`. Tests live beside the code as `*_test.go` files.

## Build, Test, and Development Commands
推荐通过 `make help` 查看完整目标列表。常用入口：
- `make server` 默认启动 Gin Web 服务（`:8080`，等价 `go run . --config config/config.yaml`）。
- `make chat` 进入命令行交互式对话（等价 `go run . chat --config config/config.yaml`）。
- `make frontend-dev` 启动 Vite dev server（`:5173`，`/api` 与 `/health` 自动代理到 `:8080`）；首次需先 `cd frontend && npm install`。
- `make frontend-build` 构建前端到 `frontend/dist`；想一体化预览：`go run . --config config/config.yaml --static frontend/dist`。
- `make test` 运行 `go test ./...`；需要覆盖率时直接 `go test ./... -cover`。
- `docker compose up -d mysql redis` 给 storage 相关工作准备本地依赖。
- `gofmt -w $(rg --files -g '*.go')` 批量格式化所有 Go 源码。
- 覆盖默认变量：`make server CONFIG=config/local.yaml ADDR=:9090`；复杂 flag 直接走 `go run .`。

## Coding Style & Naming Conventions
Follow standard Go formatting and let `gofmt` decide indentation and spacing. Keep package names short and lowercase, exported identifiers in `CamelCase`, and error strings lowercase. Match existing config key patterns such as `max_history_messages` and `short_term_ttl`. User-facing CLI text is currently Chinese, so keep new prompts and messages consistent unless you are intentionally changing localization.

## Testing Guidelines
Add tests next to the implementation you change, for example `cmd/chat_test.go` or `storage/model_selection_test.go`. Prefer table-driven tests for config parsing, selector logic, and storage edge cases. Name tests `TestXxx`. There is no enforced coverage gate, but touched packages should keep or improve coverage and should pass `go test ./...` before review.

## Commit & Pull Request Guidelines
Recent history uses short, imperative, lowercase commit subjects such as `add configuration files and Redis active memory store implementation`. Follow that style, keep each commit focused, and mention the subsystem when useful. PRs should describe behavior changes, config or schema impact, linked tasks, and the verification performed. Include terminal output or screenshots only when CLI behavior materially changes.

## Configuration & Security Tips
Do not commit real API keys, database credentials, or local DSNs. Prefer environment variables such as `DEEPSEEK_API_KEY`, `OPENAI_API_KEY`, `GORA_DATABASE_URL`, and `GORA_REDIS_ADDR`, or keep secrets in an untracked `config/config.yaml`.
