# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

Gora（Goroutine + Agent）是一个纯 Go 后端的 LLM Agent 框架。每个 Agent 跑在独立 goroutine 里，思考 / 工具调用 / 工具返回 / 流式输出全部通过 SSE 事件流回前端。后端是纯 API 服务（`:8080`），前端是独立的 Vite SPA（`:5173`，开发态走代理），不再嵌入静态资源。

LLM 内核基于 `cloudwego/eino` 的 `ChatModelAgent`；Gora 在外层包了 goroutine 生命周期、事件流、工具授权 gate（`ToolPermissionGate`）、可插拔的 LLM/工具/存储。

## 链路成熟度（重要）

只有 **user 链路是完整且已审定的参考实现**，其它链路都是**待优化**状态——动这些链路前先去对照 user 链路的写法。

| 链路 | 状态 | 关键文件 |
|---|---|---|
| **user**（注册/登录/资料） | ✅ **参考实现 / golden path** | `api/request/user.go`、`api/response/user.go`、`internal/domain/user.go`、`internal/repository/{model,mysql}/user.go`、`internal/server/user.go`、`internal/controller/user.go`、`internal/router/user.go`、`internal/errs/user.go` |
| history（session/message） | ⏳ 待优化（结构搭好，未做齐 user 那一档的细节） | `internal/repository/{model,mysql}/{session,message}.go`、`internal/server/history.go`、`internal/controller/history.go`、`internal/router/session.go`、`internal/errs/history.go` |
| chat（SSE 对话流） | ⏳ 待优化 | `internal/{server,controller,router}/chat.go` |
| agent / tool / model | ⏳ 待优化 | 同名文件，各层都有 |

**完善其它链路时遵循的 user 链路结构（每条链路一份）：**

1. `api/request/<x>.go` — 请求 DTO + `binding` 标签
2. `api/response/<x>.go` — 响应 DTO（脱敏，与 model 分离）
3. `internal/domain/<x>.go` — 对外领域类型（脱敏）
4. `internal/repository/model/<x>.go` — GORM 模型（带 `gorm:"..."` 标签）
5. `internal/repository/mysql/<x>.go` — DAO 接口 + Option 模式（参考 `userDAO.FindOne` + `ByEmail`/`ByID`）；`FindOne` 找不到返回 `(nil, nil)` 而不是 `gorm.ErrRecordNotFound`
6. `internal/server/<x>.go` — 业务 service：把 model → domain，包错误（`errs.Err*`）
7. `internal/controller/<x>.go` — Gin handler：用 `pkg/ginx.WrapReq` / `WrapClaims` / `WrapClaimsAndReq` 装包，返回 `(response.Response, error)`，配 swag 注释
8. `internal/router/<x>.go` — `RegisterXxxRouter(r, handler, authMiddleware)`，需要鉴权的接口挂 `authMiddleware`
9. `internal/errs/<x>.go` — 业务错误码段（参考 user 的 `2001xx` 段落）
10. 三处 ProviderSet 同步加：`repository.ProviderSet` / `server.ProviderSet` / `controller.ProviderSet`，最后 `make wire`

**当前其它链路与 user 的主要差距**（待优化项的非穷举清单）：

- chat / agent / tool / model 的路由**没挂 auth 中间件**，user/sessions 才挂了
- chat handler 直接读 `c.Param`/`c.Query`，没做成统一的 request DTO + binding
- history 链路的 `Session.Upsert` 时机是从 runner 端写的，缺 controller 显式建 session 接口
- agent/tool/model 的 service 只在内存里跑，没有 repository 层
- 错误码尚未做齐：很多地方还在 `fmt.Errorf` 直接返回，没走 `errs.*`

## 常用命令

```bash
make server        # 启动 Gin API（go run ./cmd/gora --config configs/config.yaml）
make frontend-dev  # 启动 Vite dev server（首次需先 cd frontend && npm install）
make build         # 只构建后端二进制到 bin/gora
make test          # go test ./...
make wire          # 重新生成 cmd/gora/wire_gen.go（**改动任何 ProviderSet 后必跑**）
```

可覆盖默认配置路径：`make server CONFIG=configs/local.yaml`。

跑单包 / 单测试：

```bash
go test ./internal/repository/dao/...                       # 单包
go test ./internal/agent/eino -run TestEinoAgent_DisabledToolBlocksExecution
go test ./... -cover
```

格式化：`gofmt -w $(find . -name '*.go' -not -path './frontend/*')`。

本地基础设施：`docker compose up -d mysql redis`（DSN/addr 为空时所有持久化退化为 noop，可不起依赖直接跑）。

## 架构（必须理解的全局关系）

请求从外到内的分层（与 Gin 标准分层对齐）：

```
HTTP/SSE → router → controller → server (业务编排) → repository (DAO/Cache) + agent (Runner/Eino)
```

- **`cmd/gora/`** — 入口（`main.go` + `wire.go` + `wire_gen.go` 同包，kratos 风格）。`main.go` 用 `flag.Parse()` 读配置 → `newApp(ctx, cfg)` 拿 `*App` → 手动 `runner.New(...)` 注入到 `AgentService`/`ModelService`（Runner 依赖运行期 LLM 索引，不入 wire）→ 起 HTTP 服务器。
- **`internal/agent/`** — Agent 框架。
  - `core/`：Agent 接口、`BaseAgent` 生命周期（`BeginRun`/`FinishRun`/`stopCh`/`doneCh`）、`Event` 7 种类型、`ToolPermissionGate`。
  - `eino/`：基于 `cloudwego/eino` 的 `EinoAgent`，多会话内部以 `histories map[sessionID][]*schema.Message` 存。`DefaultSessionID = "default"` 仅是 eino 单 Run 的内部默认 key，**不**用于持久化。
  - `runner/`：`Runner` 同时实现 `server.SessionRunner` + `server.ModelSelector`。**不入 wire**——它依赖 `cfg.LLM` 索引这种运行期数据，由 `main.go` 手动 `runner.New()` 后注入到 service。`RunSession` 必须传非空 sessionID（项目去掉了 "default" 兜底语义）。
  - `tool/` + `tool/builtin/`：`Tool` 接口 + 线程安全 `Registry`，内置 `HTTPTool`（强制公网、白名单方法）。
- **`internal/server/`** — 业务编排层（**不**是 HTTP 处理；HTTP 在 controller）。`ChatService.Stream` 把 disabled tools / permission gate 注入 ctx，消费 Agent 事件流写回 `EventSink`。`HistoryService` 聚合 `SessionDAO + MessageDAO` 暴露会话/消息查询。
- **`internal/controller/`** — Gin handler，靠 `pkg/ginx` 的 `WrapReq`/`WrapClaimsAndReq`/`WrapSSEReq` 做请求绑定 + 错误码翻译；返回 `(response.Response, error)`，错误统一走 `errs.*`（`pkg/errorx` 包成 HTTP/code/msg 三元组）。
- **`internal/router/`** — 路由集中注册。`NewEngine` 把所有 handler + 中间件装配成 `*gin.Engine`。auth 中间件**只挂在受保护路由**上（user/profile、sessions），其余 chat/agent/tool/model 当前无鉴权（待优化项）。
- **`internal/repository/`** — 数据访问聚合入口。子包按"存储后端"命名，方便后续平铺增加（如 `es/`）：
  - `model/`：GORM 模型（`User`/`Session`/`Message`），带 GORM 标签。`Session.ID`/`Message.SessionID` 是业务侧生成的 string；`UserID` 也是 string，让 runner（cfg 字符串）和 API（JWT 数字字符串）共用同一列。
  - `mysql/`：MySQL 持久化（GORM）。`UserDAO`/`SessionDAO`/`MessageDAO`，**统一 Option 模式**（`ByEmail`/`ByID`/`BySessionID`/`BySessionUserID`/`AfterID`/`MessageLimit`）；`FindOne` 找不到返回 `(nil, nil)` 而不是 error，service 用 `== nil` 判定。
  - `redis/`：`ActiveMemoryCache`——**会话级活跃记忆**（不是普通缓存；丢失会丢上下文，与 MySQL 一起承担短期持久化）。TTL 默认 24h，未配置 Redis 退化为 Noop。注意：第三方 `github.com/go-redis/redis/v8` 在本子包内必须用 `goredis` 别名，避免与 `package redis` 冲突。
  - `repository.go`：类型别名 + Option helper 重新导出 + `ProviderSet` + `InitTables` 自动迁移入口，作为上层单一 import 入口。
  - 未来加 `es/`（Elasticsearch 索引/全文搜索）时，沿用同样模式：自己一个子包，`repository.go` 再 re-export 别名。
- **`internal/ioc/`** — 基础设施 wire provider（`InitMysql`/`InitRedis`/`InitLogger`）。**`InitMysql` 调 `repository.InitTables(db)` 跑 AutoMigrate**——表结构变更只动 `model/` + 在 `repository.InitTables` 列表里加一条，AutoMigrate 自动跟。
- **`internal/middleware/`** — Gin 中间件（CORS / auth / basic auth / logger / 限流），各自有 `ProviderSet`，被 `cmd/gora/wire.go` 聚合。
- **`internal/errs/`** — 业务错误码集中管理：`100xxx` 系统、`2001xx` 用户、`2002xx` Agent/工具、`2003xx` 会话；统一用 `errorx.New(httpStatus, code, msg, cause)` 构造，`pkg/ginx` 装包器自动翻译为 JSON。
- **`internal/domain/`** — 对外暴露的领域 DTO（脱敏后），与 `model/` 分离，避免持久层字段（如 `Password`）泄漏到 API。
- **`api/request/` + `api/response/`** — HTTP DTO，绑定标签（`binding:"required,email"` 等）放这里。
- **`configs/`**（注意：包名是 `configs`，不在 `internal/` 下）— YAML 加载 + `Validate()` + per-子配置 `New*Config(cfg)` provider，给 wire 拆出值类型。`DefaultPath = "configs/config.yaml"`。
- **`pkg/`** — 内部可复用工具。`pkg/ginx` 是请求装包器 + claims 提取；`pkg/errorx` 是 HTTP 错误模型；`pkg/ijwt` 是 JWT + AES 加密（`UserClaims.UserId`/`Email` 都是加密后再签）。

### Wire 装配规则

`cmd/gora/wire.go` 用 `wire.Build` 聚合：`appconfig.ProviderSet + ioc.ProviderSet + repository.ProviderSet + server.ProviderSet + controller.ProviderSet + router.ProviderSet + middleware.ProviderSet`，再加几个手写 provider（`provideToolRegistry` / `provideActiveMemoryCache` / `provideChatOptions`），最后 `wire.Struct(new(App), "*")` 反射填字段。

**改任何 ProviderSet 都必须 `make wire`**。`wire_gen.go` 是生成代码，不要手改。

### 关键架构选择

- **Runner 不入 wire**：依赖 `cfg.LLM[index]` 这种运行期数据流，硬塞 wire 会把"选 LLM"业务搬进装配阶段。`main.go` 手动 `runner.New()` 后调 `AgentService.Register()` / `ModelService.SetSelector()`。
- **Per-session LLM 选择直接落 `Session.LLMName`**——不再有独立的 `model_selection` 表（旧的 `chat_message` / `model_selection` 已被废弃）。Runner 在内存维护 `sessionLLM map[string]string` 做 fast-path 缓存。
- **历史读取顺序**：`restoreConversationContext` → 先 Redis 短期记忆，miss 再回 `MessageDAO.ListRecent`，命中后回填 Redis；任何一步失败只打诊断不阻塞主流程。
- **历史写入顺序**：`persistConversationTurn` → `MessageDAO.NextSeq` 取 seq → `Append(user, assistant)` → `Session.Upsert`（更新 `LLMName` / `LastMessageAt`）→ Redis 同步当前 snapshot；同样不阻塞主流程。
- **noop 退化**：DSN / Redis addr 为空时退化为 noop 实现，本地一键起服务（无 docker-compose 也能跑）。
- **错误处理约定**：service 把"不存在/越权"包成 `errs.Err*`（已经带 HTTP code），controller 直接 `return response.Response{}, err` 透传；只有真正的内部错误才 `errs.InternalServerError(err)`。

### 配置覆盖优先级

YAML（`configs/config.yaml`）< 环境变量（`DEEPSEEK_API_KEY`/`OPENAI_API_KEY`/`GORA_DATABASE_URL`/`GORA_REDIS_*`/`GORA_SHORT_TERM_MEMORY_TTL`/`GORA_USER_ID`）。LLM 由 `name`（不是 `provider`/`model`）唯一标识，切换模型只看 `LLMConfig.Name`。

## 风格 / 测试约定

- 标准 `gofmt`；包名小写、错误字符串小写。
- 配置 key 沿用 `snake_case`（如 `max_history_messages` / `short_term_ttl`）。
- 用户可见文案当前是中文，新增日志/提示保持一致。
- 测试与实现同目录（`*_test.go`），偏好表驱动；命名 `TestXxx`。无强制覆盖率门，但改动包要保持 `go test ./...` 通过。
- Commit 风格：短小 / imperative / 小写主题（如 `add configuration files and Redis active memory store implementation`）。

## 常见踩坑

- 删除/修改 `model/` 字段后忘了重启服务跑 AutoMigrate——表结构会落后；新增列不破坏旧数据，删列要手动迁移。
- 改了任意 `ProviderSet`（`internal/server/server.go`、`internal/controller/controller.go`、`internal/repository/repository.go` 等）后忘 `make wire`——build 通过但运行时缺组件。
- `RunSession` 现在拒绝空 sessionID；CLI / 测试代码如果直接用 `runner.Run()` 会立即 EventError，需要显式传 sessionID。
- `pkg/ijwt` 的 `UserClaims.UserId` 是数字字符串（`strconv.FormatUint(id, 10)`），controller 里 `strconv.ParseUint` 后再用作 `model.User.ID`；history 链路则直接把它作为 `Session.UserID`（string）写入。
- 优化非 user 链路时，**先**对照 user 链路的 9 步骨架补齐文件，**再**写业务逻辑——跳过 DTO / errs 直接堆 controller 是当前"待优化"状态的根因。
