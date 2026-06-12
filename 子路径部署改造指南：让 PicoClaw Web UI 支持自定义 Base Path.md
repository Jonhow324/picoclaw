# 子路径部署改造指南：让 PicoClaw Web UI 支持自定义 Base Path

## 目标

将 PicoClaw Web UI 从根路径 `/` 部署改为支持任意子路径（如 `/picoclaw/`），适用于反向代理场景。

## 问题全景

当前代码中存在 **5 类硬编码绝对路径**，分布在前后端共 **20+ 个文件** 中：

| 类别           | 典型示例                             | 涉及文件数          |
| -------------- | ------------------------------------ | ------------------- |
| 静态资源引用   | `/favicon-96x96.png`                 | 1（index.html）     |
| API 请求路径   | `/api/models`, `/api/gateway/status` | 11（src/api/*.ts）  |
| WebSocket 端点 | `/pico/ws`                           | 1（controller.ts）  |
| 页面跳转       | `/launcher-login`                    | 1（http.ts）        |
| Vite 构建配置  | 未设 `base`，proxy 硬编码 `/api`     | 1（vite.config.ts） |

------

## 第一步：Vite 构建配置

**文件**：vite.config.tsL1-L43

### 1.1 添加 `base` 选项



```
// 从环境变量读取，默认 "/"const BASE_URL = process.env.VITE_BASE_URL || "/"; export default defineConfig({  base: BASE_URL,  // 新增这一行  plugins: [ /* ... */ ],  // ...})
```

### 1.2 更新 dev proxy

当前 proxy 写死了 `/api`、`/pico/media`、`/pico/ws`。改为从 base 派生：



```
const basePath = BASE_URL.endsWith("/") ? BASE_URL : BASE_URL + "/"; server: {  proxy: {    [`${basePath}api`]: {      target: "http://localhost:18800",      changeOrigin: true,      rewrite: (p) => p.replace(basePath, "/"),  // 后端不需要前缀    },    [`${basePath}pico/media`]: {      target: "http://localhost:18800",      changeOrigin: true,      rewrite: (p) => p.replace(basePath, "/"),    },    [`${basePath}pico/ws`]: {      target: "ws://localhost:18800",      ws: true,      rewrite: (p) => p.replace(basePath, "/"),    },  },},
```

> **注意**：如果你选择让后端也感知 base path（推荐），则不需要 `rewrite`，直接透传即可。见第四步的说明。

------

## 第二步：index.html 静态资源

**文件**：index.htmlL1-L19

Vite 设置 `base` 后，会自动重写 `<script>` 和 `<link>` 中的引用路径。但 `index.html` 中手动写的绝对路径也需要处理。最简单的做法是改为**相对路径**：



```
<!-- 之前 --><link rel="icon" href="/favicon-96x96.png" /><link rel="manifest" href="/site.webmanifest" /> <!-- 改为相对路径（推荐，Vite 会正确处理） --><link rel="icon" href="./favicon-96x96.png" /><link rel="manifest" href="./site.webmanifest" />
```

或者，依赖 Vite 的 `base` 自动注入——只要资源在 `<head>` 中以 `/` 开头且能被 Vite 的 HTML 处理器识别，Vite 会自动添加前缀。但**最保险的做法是使用相对路径**。

------

## 第三步：前端 API 层改造（核心工作量）

这是改动量最大的部分。当前所有 API 模块都硬编码了 `/api/...`。

### 3.1 新建 Base Path 工具模块

**新建文件**：`web/frontend/src/lib/base-path.ts`



```
/** * 从 Vite 构建时注入的 BASE_URL 派生运行时 base path。 * Vite 会自动将 import.meta.env.BASE_URL 设置为 vite.config.ts 中的 base 值。 */export const BASE_PATH: string = (() => {  const raw = import.meta.env.BASE_URL || "/";  // 确保以 "/" 开头，以 "/" 结尾  let normalized = raw.startsWith("/") ? raw : `/${raw}`;  if (!normalized.endsWith("/")) {    normalized += "/";  }  return normalized;})(); /** * 将 "/api/models" 拼接为 "/picoclaw/api/models" */export function withBase(path: string): string {  if (!path.startsWith("/")) {    path = `/${path}`;  }  // 避免 base 为 "/" 时出现双斜杠  if (BASE_PATH === "/") return path;  return `${BASE_PATH}${path.slice(1)}`;} /** 用于 WebSocket URL 构建 */export function getWsBaseUrl(): string {  const wsScheme = window.location.protocol === "https:" ? "wss:" : "ws:";  return `${wsScheme}//${window.location.host}${BASE_PATH}`;}
```

### 3.2 统一改造各 API 模块

以下每个文件的 `request` 函数中，将硬编码路径替换为 `withBase()` 调用。

**模式示例**（以 pico.tsL15-L35 为例）：



```
// 之前async function request<T>(path: string, options?: RequestInit): Promise<T> {  const res = await launcherFetch(`${BASE_URL}${path}`, options)  // ...}export async function getPicoInfo() {  return request<PicoInfoResponse>("/api/pico/info")} // 之后import { withBase } from "@/lib/base-path" async function request<T>(path: string, options?: RequestInit): Promise<T> {  const res = await launcherFetch(withBase(path), options)  // 改这一行  // ...}export async function getPicoInfo() {  return request<PicoInfoResponse>("/api/pico/info")  // 这里不用改，request 内部处理}
```

> **关键设计决策**：将 `withBase()` 的调用集中在 `request()` 函数内部，这样各个 API 函数中的路径字符串**不需要逐个修改**。`launcherFetch` 的直接调用除外。

### 3.3 需要修改的完整文件清单

以下文件中，`request()` 内部的 `BASE_URL` 拼接需要改为 `withBase(path)`：

| 文件               | 路径                             | 说明                    |
| ------------------ | -------------------------------- | ----------------------- |
| pico.tsL15         | `BASE_URL` + path                | 标准模式                |
| channels.tsL44     | `BASE_URL` + path                | 标准模式                |
| gateway.tsL24      | `BASE_URL` + path                | 标准模式                |
| models.tsL55       | `BASE_URL` + path                | 标准模式                |
| oauth.tsL57        | `BASE_URL` + path                | 标准模式                |
| skills.tsL61       | 直接传 `path` 给 `launcherFetch` | 需改为 `withBase(path)` |
| tools.tsL53        | 直接传 `path` 给 `launcherFetch` | 需改为 `withBase(path)` |
| system.tsL39       | 直接传 `path` 给 `launcherFetch` | 需改为 `withBase(path)` |
| sessions.tsL36-L65 | 直接调用 `launcherFetch`         | 需在每处加 `withBase()` |

### 3.4 非标准 fetch 调用（`launcher-auth.ts`）

**文件**：launcher-auth.tsL17-L83

此文件使用原生 `fetch` 而非 `launcherFetch`，每个调用都需要手动加 `withBase()`：



```
// 之前const res = await fetch("/api/auth/login", { /* ... */ }) // 之后import { withBase } from "@/lib/base-path"const res = await fetch(withBase("/api/auth/login"), { /* ... */ })
```

涉及的调用（共 5 处）：

- `/api/auth/login` — 第 17 行L17
- `/api/auth/status` — 第 35 行L35
- `/api/auth/logout` — 第 47 行L47
- `/api/auth/setup` — 第 61 行L61
- `/api/skills/import` — skills.ts 第 114 行L114（`launcherFetch` 直接调用）

### 3.5 HTTP 401 重定向路径

**文件**：http.tsL38



```
// 之前globalThis.location.assign("/launcher-login") // 之后import { withBase } from "@/lib/base-path"globalThis.location.assign(withBase("/launcher-login"))
```

------

## 第四步：WebSocket 连接改造

**文件**：controller.tsL131



```
// 之前（第 131 行）const wsScheme = window.location.protocol === "https:" ? "wss:" : "ws:"const wsUrl = `${wsScheme}//${window.location.host}/pico/ws` // 之后import { getWsBaseUrl } from "@/lib/base-path"const wsUrl = `${getWsBaseUrl()}pico/ws`
```

> 注意 websocket.tsL1-L30 中的 `normalizeWsUrlForBrowser` 函数处理的是后端返回的 `ws_url`，如果后端返回的 URL 也带了 base path 前缀，则此函数不需要改动。

------

## 第五步：TanStack Router 配置

**文件**：main.tsxL16-L23

TanStack Router 需要知道 base path 以正确处理客户端路由：



```
import { BASE_PATH } from "@/lib/base-path" const router = createRouter({  routeTree,  context: { queryClient },  basepath: BASE_PATH,  // 新增})
```

------

## 第六步：Go 后端改造

### 6.1 反向代理场景（后端不感知 base path）

如果反向代理（Nginx/Caddy）在转发时 strip 掉前缀，则 Go 后端**完全不需要改动**。这是最简单的方案。

**Nginx 配置示例**：



```
location /picoclaw/ {    proxy_pass http://127.0.0.1:18800/;  # 注意末尾的 "/"，会 strip 前缀    proxy_set_header Host $host;    proxy_set_header X-Real-IP $remote_addr;    proxy_http_version 1.1;    proxy_set_header Upgrade $http_upgrade;    # WebSocket 支持    proxy_set_header Connection "upgrade";}
```

此时前端请求 `/picoclaw/api/models` → Nginx 转发为 `/api/models` → 后端正常处理。

### 6.2 后端感知 base path（可选，更灵活）

如果希望后端也感知前缀（适用于不想依赖反向代理 strip 的场景），需要改动 embed.goL43-L79 和 router.goL73-L82：

**embed.go** — 修改 SPA fallback 逻辑，在路由注册时加前缀：



```
// registerEmbedRoutes 增加 basePath 参数func registerEmbedRoutes(mux *http.ServeMux, basePath string) {    // ... 现有逻辑 ...        handlePath := basePath + "/"    if basePath == "" || basePath == "/" {        handlePath = "/"    }        mux.Handle(handlePath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {        // strip basePath 后处理        r.URL.Path = strings.TrimPrefix(r.URL.Path, basePath)        if !strings.HasPrefix(r.URL.Path, "/") {            r.URL.Path = "/" + r.URL.Path        }        // ... 现有 SPA fallback 逻辑 ...    }))}
```

**router.go** — API 路由同样需要加前缀。

> **推荐方案**：使用 6.1（反向代理 strip），后端零改动，维护成本最低。

------

## 第七步：构建与验证

### 7.1 构建



```
# 设置 base path 为 /picoclaw/cd web/frontendVITE_BASE_URL=/picoclaw/ pnpm build:backend # 然后正常构建 Go 后端cd ../..go build -o picoclaw-web ./web/backend/
```

### 7.2 验证清单

-  页面在 `/picoclaw/` 下正常加载，无 404 资源
-  API 请求（如 `/picoclaw/api/models`）返回 200
-  WebSocket 连接（`/picoclaw/pico/ws`）建立成功
-  客户端路由（如 `/picoclaw/channels/wechat`）正常导航
-  401 时重定向到 `/picoclaw/launcher-login` 而非 `/launcher-login`
-  刷新任意子路由页面不出现 404
-  favicon 和 manifest 正常加载

------

## 改动影响矩阵

| 文件                           | 改动类型               | 风险等级 |
| ------------------------------ | ---------------------- | -------- |
| vite.config.tsL1-L43           | 添加 `base` + 改 proxy | 🟡 中     |
| index.htmlL1-L19               | 改为相对路径           | 🟢 低     |
| `src/lib/base-path.ts`（新建） | 工具函数               | 🟢 低     |
| http.tsL38                     | 1 处重定向路径         | 🟢 低     |
| launcher-auth.tsL1-L83         | 5 处 fetch 路径        | 🟡 中     |
| pico.tsL15                     | request 内部 1 处      | 🟢 低     |
| channels.tsL44                 | request 内部 1 处      | 🟢 低     |
| gateway.tsL24                  | request 内部 1 处      | 🟢 低     |
| models.tsL55                   | request 内部 1 处      | 🟢 低     |
| oauth.tsL57                    | request 内部 1 处      | 🟢 低     |
| skills.tsL61                   | request + 1 处直接调用 | 🟡 中     |
| tools.tsL53                    | request 内部 1 处      | 🟢 低     |
| system.tsL39                   | request 内部 1 处      | 🟢 低     |
| sessions.tsL36-L65             | 3 处直接调用           | 🟡 中     |
| controller.tsL131              | WebSocket URL 1 处     | 🔴 高     |
| main.tsxL16-L23                | 添加 router basepath   | 🟡 中     |
| embed.goL43-L79                | 可选（仅方案 6.2）     | 🔴 高     |

------

## 总结

整个改造可以归为一句话：**创建一个 `withBase()` 拦截层，让所有路径在运行时自动加上前缀，同时通过 Vite 的 `base` 和反向代理的 strip 配置实现前后端协同。**