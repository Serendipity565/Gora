# Gora Frontend

Gora Agent Playground 的前端，使用 **Vite + TypeScript**，与 Go 后端通过 SSE 通信。

> 注意：仓库还附带一份 `web/static/index.html` —— 单文件、自带样式，由 Go 二进制内嵌，**不依赖本目录**。
> 本目录是带类型检查、HMR、模块化的"加强版"前端，构建产物可通过 `--static frontend/dist` 替换内嵌页。

## 目录结构

```
frontend/
├── index.html          # 单页入口（dev 模式直接服务，build 时被 Vite 处理）
├── package.json
├── tsconfig.json
├── vite.config.ts      # /api 与 /health 默认代理到 :8080
└── src/
    ├── main.ts         # 入口，绑定 DOM 事件
    ├── api.ts          # /api/chat (SSE)、/api/agents、/api/tools
    ├── ui.ts           # 渲染气泡、事件、统计、工具列表
    ├── types.ts        # 与 Go agent.Event / web.AgentInfo 对齐
    └── style.css       # 同时通过 <link> 和 main.ts import 加载，避免 dev 模式 FOUC
```

## 开发（推荐用 Makefile）

```bash
# 终端 1：后端（仓库根目录）
make server

# 终端 2：前端 dev server
cd frontend && npm install   # 首次运行
make frontend-dev            # http://localhost:5173，/api 自动代理到 :8080
```

等价的原生命令：

```bash
go run . --config config/config.yaml --addr :8080
cd frontend && npm install && npm run dev
```

## 生产构建

```bash
make frontend-build                                       # 输出到 frontend/dist
go run . --config config/config.yaml --static frontend/dist
```

## 类型检查

```bash
cd frontend && npm run typecheck
```

## 关于 dev 模式的 FOUC

Vite dev 服务器只直接服务源码 `index.html`。如果只在 `main.ts` 里 `import "./style.css"`，CSS 会延后到 JS 执行时才加载，导致首屏闪一下默认样式。`index.html` 里因此**额外**保留了一行 `<link rel="stylesheet" href="/src/style.css" />`，让 CSS 与 JS 并行加载。Vite build 会自动去重，不会重复打包。
