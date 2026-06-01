# Gora 完整设计框架

## 一、项目定位

```
Gora = Goroutine + Agent
一个用 Go 原生并发能力驱动的多 Agent 框架
核心理念：每个 Agent 就是一个 goroutine，天生并行、独立决策、协作执行
```

**一句话：** 让开发者在 Go 服务中像起 goroutine 一样起 Agent。

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────┐
│                      前端可视化层                          │
│  ┌─────────┐ ┌──────────┐ ┌───────────┐ ┌────────────┐  │
│  │ 对话面板 │ │ Agent状态 │ │ 工具调用链 │ │ 并行可视化  │  │
│  └─────────┘ └──────────┘ └───────────┘ └────────────┘  │
│                    ▲ SSE / WebSocket                      │
├────────────────────┼─────────────────────────────────────┤
│                    │          Web API 层                   │
│  ┌─────────┐ ┌──────────┐ ┌───────────┐ ┌────────────┐  │
│  │ 路由层  │ │ 会话管理  │ │ SSE推送   │ │ 认证/限流   │  │
│  │ Gin     │ │ Session  │ │ EventSrc  │ │ Middleware │  │
│  └─────────┘ └──────────┘ └───────────┘ └────────────┘  │
├──────────────────────────────────────────────────────────┤
│                       Gora 核心                            │
│                                                           │
│  ┌────────────┐  ┌──────────┐  ┌────────────────────┐   │
│  │ orchestrate │  │  agent   │  │     runtime        │   │
│  │  编排层     │  │  智能体   │  │     运行时         │   │
│  │            │  │          │  │  ┌──────────────┐  │   │
│  │ · Chain    │  │ · React  │  │  │ Scheduler    │  │   │
│  │ · Parallel │  │ · Proact │  │  │ 调度器       │  │   │
│  │ · Graph    │  │ · Custom │  │  │ goroutine池  │  │   │
│  │ · Vote     │  │          │  │  └──────────────┘  │   │
│  └────────────┘  └──────────┘  └────────────────────┘   │
│                                                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────┐   │
│  │  tool    │  │  memory  │  │        llm           │   │
│  │  工具    │  │  记忆    │  │      模型层           │   │
│  │          │  │          │  │                      │   │
│  │ · HTTP   │  │ · Buffer │  │ · OpenAI · Anthropic │   │
│  │ · Code   │  │ · Store  │  │ · Ollama · 自定义    │   │
│  │ · Search │  │ · Vector │  │                      │   │
│  └──────────┘  └──────────┘  └──────────────────────┘   │
│                                                           │
│  ┌──────────┐  ┌──────────┐                              │
│  │  prompt  │  │  stream  │                              │
│  │  提示词  │  │  流式    │                              │
│  └──────────┘  └──────────┘                              │
└──────────────────────────────────────────────────────────┘
```

---

## 三、核心接口设计

### 3.1 Agent 接口

```go
// agent/agent.go
type Agent interface {
    ID()       string
    Run(ctx    context.Context, input string) <-chan Event
    State()    AgentState
    Memory()   memory.Memory
}

type AgentState int
const (
    StateIdle    AgentState = iota
    StateRunning
    StateWaiting
    StateDone
    StateError
)
```

### 3.2 Runtime 接口

```go
// runtime/runtime.go
type Runtime interface {
    Register(a agent.Agent) error
    Run(ctx context.Context, agentID string, input string) (<-chan Event, error)
    RunParallel(ctx context.Context, tasks []Task) ([]Result, error)
    Shutdown() error
    Monitor() <-chan RuntimeMetrics  // 实时指标
}
```

### 3.3 LLM 接口

```go
// llm/llm.go
type LLM interface {
    Chat(ctx context.Context, messages []Message, tools []tool.Tool) (Message, error)
    ChatStream(ctx context.Context, messages []Message, tools []tool.Tool) <-chan StreamChunk
}
```

### 3.4 Event 体系（贯穿全框架）

```go
// agent/event.go
type EventType int
const (
    EventThinking    EventType = iota  // Agent 正在思考
    EventToolCall                      // 正在调用工具
    EventToolResult                    // 工具返回结果
    EventChunk                         // LLM 流式输出
    EventDone                          // 本轮完成
    EventError                         // 出错
)

type Event struct {
    Type      EventType
    AgentID   string
    Content   string
    Timestamp time.Time
    Metadata  map[string]any
}
```

---

## 四、Web API 设计

### 4.1 核心端点

```yaml
POST   /api/chat/:agentId          # 发送消息，返回 SSE 流
GET    /api/agents                  # 列出所有 Agent
GET    /api/agents/:agentId/state  # Agent 状态
POST   /api/orchestrate/parallel   # 并行执行多 Agent
GET    /api/monitor/metrics        # 运行时指标 SSE 流
```

### 4.2 SSE 事件流格式

```
event: thinking
data: {"agentId":"agent-1","content":"正在分析用户需求..."}

event: tool_call
data: {"agentId":"agent-1","tool":"http_request","args":{"url":"https://..."}}

event: tool_result
data: {"agentId":"agent-1","tool":"http_request","result":"{\"status\":200}"}

event: chunk
data: {"agentId":"agent-1","content":"根据"}

event: chunk
data: {"agentId":"agent-1","content":"查询"}

event: done
data: {"agentId":"agent-1"}
```

---

## 五、前端可视化设计

```
┌──────────────────────────────────────────────────────────┐
│  Gora Agent Playground                    [🟢 Running]   │
├────────────────────────────────┬─────────────────────────┤
│                                │                         │
│   ┌─ Agent-1 ───────────────┐ │   ┌─ Agent-2 ────────┐  │
│   │ 🧠 正在分析输入...       │ │   │ 🔧 调用工具:      │  │
│   │                         │ │   │    http_request   │  │
│   │ ┌─────────────────┐    │ │   │    ⏳ 等待中...   │  │
│   │ │ 用户输入           │    │ │   └─────────────────┘  │
│   │ │ "帮我查天气"       │    │ │                         │
│   │ └─────────────────┘    │ │   ┌─ Agent-3 ────────┐  │
│   │                         │ │   │ 💬 流式输出:      │  │
│   │ ▼ 思考中...             │ │   │ "今天是晴天,气温"  │  │
│   │ ▼ 决定调用 search 工具   │ │   │ 25°C..."        │  │
│   │ ▼ 获取结果: {...}        │ │   └─────────────────┘  │
│   └─────────────────────────┘ │                         │
│                                │                         │
│  ┌────────────────────────────┴─────────────────────────┐│
│  │ 📊 运行时指标                                       ││
│  │ Agents: 3 active | Memory: 256MB | LLM Calls: 12   ││
│  └─────────────────────────────────────────────────────┘│
│                                                          │
│  ┌──────────────────────────────────────────────────────┐│
│  │ 💬 输入消息...                              [发送]   ││
│  └──────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────┘
```

核心展示要素：
- **每个 Agent 独立面板**，实时显示状态流转
- **工具调用链可视化**，展示参数和返回值
- **并行执行可视化**，多个 Agent 同时高亮
- **流式输出打字机效果**
- **底部运行时指标仪表盘**

---

## 六、分阶段开发流程

### 总工期：4 周（业余时间）

---

### 🔵 Phase 0：骨架搭建（2-3 天）

**目标：** 能 `go run main.go` 看到东西

```
任务清单：
├── 初始化 go mod (github.com/yourname/gora)
├── 搭建目录结构
├── 定义核心接口（agent/agent.go, llm/llm.go, tool/tool.go）
├── 实现 Event 体系
├── 写一个假 Agent（mock agent），直接返回字符串
├── 写最小的 main.go CLI，跑通调用链
└── README.md 写标题 + 一句话描述
```

**可交付物：** `go run main.go` 能输出 "Agent-1 says: Hello World"

---

### 🟢 Phase 1：核心最小闭环（1 周）

**目标：** 一个能调用 OpenAI 工具的真实 Agent

```
任务清单：
├── llm/openai.go 实现 OpenAI 适配器
│   ├── Chat（非流式）
│   └── ChatStream（SSE 流式）
├── agent/base.go 实现 BaseAgent
│   ├── ReAct 循环（思考→行动→观察→...→回答）
│   └── 工具调用解析
├── tool/registry.go + tool/builtin/http.go
│   ├── Tool 接口 + 注册表
│   └── 至少一个可用的内置工具（HTTP 请求）
├── runtime/runtime.go 实现基础运行时
│   ├── 注册 Agent
│   └── 执行单个 Agent
├── examples/simple/main.go
│   └── 一个完整可跑的 CLI 示例
└── 单元测试
```

**可交付物：** CLI 输入 "帮我查 https://api.github.com 的状态"，Agent 自动调用 HTTP 工具获取并回答

---

### 🟡 Phase 2：Web 化 + 可视化（1.5 周）

**目标：** 简历上能放 Demo 链接的状态

```
任务清单：
├── cmd/server/main.go
│   ├── Gin 路由搭建
│   ├── POST /api/chat/:agentId → SSE
│   ├── GET /api/agents
│   └── GET /api/monitor/metrics → SSE
├── Web 前端（单文件 HTML）
│   ├── 对话面板（支持 Markdown 渲染）
│   ├── Agent 状态面板（实时状态流转）
│   ├── 工具调用链展示
│   ├── 流式输出打字机效果
│   └── 运行时指标面板
├── runtime 增强
│   ├── Monitor() 实时指标
│   └── goroutine 池管理
├── stream/sse.go SSE 工具封装
└── Dockerfile（方便部署 Demo）
```

**可交付物：** 本地 `docker-compose up` 后浏览器打开 `localhost:8080` 看到完整可视化界面

---

### 🟣 Phase 3：多 Agent 编排（1 周）

**目标：** 真正的并行多 Agent 协作

```
任务清单：
├── orchestrate/parallel.go
│   └── 多个 Agent 同时执行不同任务，结果汇总
├── orchestrate/chain.go
│   └── Agent 串行接力，A 的输出是 B 的输入
├── orchestrate/vote.go
│   └── 多 Agent 对同一问题投票决策
├── 前端增强
│   ├── 并行执行可视化（多个 Agent 面板同时高亮）
│   └── 执行图展示
├── memory/store.go 长期记忆
│   └── 可插拔存储（内存/Redis）
└── 完整示例：多 Agent 协作写报告
```

**可交付物：** 页面同时启动 3 个 Agent，一个负责查资料、一个负责分析、一个负责润色，协作完成一篇文章

---

### 🔴 Phase 4：打磨 + 部署（3-4 天）

**目标：** 线上能访问，简历上放链接

```
任务清单：
├── 部署到 Fly.io / Railway / Vercel
├── 配置域名（可选）
├── README.md 完善
│   ├── Logo + Badge
│   ├── 架构图
│   ├── 快速开始
│   ├── GIF 演示
│   └── API 文档
├── 录制演示视频/GIF
├── examples/ 下补全所有示例
└── 写一篇技术博客（可选但加分）
```

**可交付物：** 一个公网可访问的 Demo 链接，可以直接放在简历上

---

## 七、技术栈总结

```
语言：       Go 1.22+
Web 框架：   Gin
SSE：        Go 标准库 + 自定义封装
LLM：        OpenAI API / Ollama（本地）
前端：       单文件 HTML + Vanilla JS + EventSource
             （可选：Alpine.js 或 HTMX 增强交互）
部署：       Docker + Fly.io（免费额度够用）
```

---

## 八、简历呈现模板

```
Gora — Go 并发多 Agent 框架 | 2026.06 - 至今
github.com/yourname/gora  |  在线 Demo: gora-demo.fly.dev

· 设计并实现基于 goroutine 的 AI Agent 运行时，原生支持多 Agent 并发调度
· 实现 ReAct 循环、工具调用、流式输出等核心机制，兼容 OpenAI/Ollama 多模型
· 使用 SSE 实现 Agent 执行过程实时可视化，包含思考链、工具调用、并行状态
· 支持 Chain/Parallel/Vote 多种编排模式，多 Agent 协作完成复杂任务

技术栈：Go, Gin, SSE, OpenAI API, Docker, Fly.io
```