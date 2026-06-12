# PicoClaw 子路径部署与多架构容器构建指南

本指南详细整理了将 PicoClaw Web 控制台（Launcher）从默认根路径 `/` 改造为支持**自定义子路径部署（以 `/picoclaw/` 为例）**的完整技术方案，并说明了如何在没有 QEMU 模拟器的开发机上完成 `ARM64` 跨架构容器打包与在目标设备上的部署实践。

---

## 目录
1. [方案架构设计](#1-方案架构设计)
2. [前端代码子路径重构](#2-前端代码子路径重构)
3. [一键式自动化构建助手 (`build.sh`)](#3-一键式自动化构建助手-buildsh)
4. [Docker / Podman 跨架构零模拟编译打包](#4-docker--podman-跨架构零模拟编译打包)
    - [4.1 新建发布版打包配置](#41-新建发布版打包配置)
    - [4.2 本地跨平台封包工作流](#42-本地跨平台封包工作流)
    - [4.3 镜像的版本控制与发布规范 (Tag 最佳实践)](#43-镜像的版本控制与发布规范-tag-最佳实践)
    - [4.4 镜像的物理分发与传送](#44-镜像的物理分发与传送)
5. [目标设备 Docker 运行与 Nginx 反向代理配置](#5-目标设备-docker-运行与-nginx-反向代理配置)

---

## 1. 方案架构设计

为了降低维护成本，本项目采用**“反向代理 Strip 前缀，前端注入 Base”**的协同方案（方案 6.1）：
- **前端打包时**：注入自定义子路径（如 `VITE_BASE_URL=/picoclaw/`），使所有静态资源引用、接口调用和浏览器路由全部被重置于该命名空间下。
- **网关转发时**：反向代理（Nginx 或 Caddy）将子路径前缀剥离后转发给 Go 后端。
- **Go 后端**：保持原样，无需为子路径开发冗余 of 路由感知逻辑，仅需在网关层处理重定向重写。

---

## 2. 前端代码子路径重构

我们对前端进行了 20 多个文件的改造，确保所有路径均为动态适配：

### 2.1 动态路径计算拦截器
新建 [base-path.ts](file:///home/jonhow/Desktop/picoclaw/web/frontend/src/lib/base-path.ts) 作为工具模块，利用 Vite 注入的 `import.meta.env.BASE_URL` 计算出运行时的基准并拦截路径：
```typescript
export const BASE_PATH: string = (() => {
  const raw = import.meta.env.BASE_URL || "/";
  let normalized = raw.startsWith("/") ? raw : `/${raw}`;
  if (!normalized.endsWith("/")) normalized += "/";
  return normalized;
})();

export function withBase(path: string): string {
  if (!path.startsWith("/")) path = `/${path}`;
  if (BASE_PATH === "/") return path;
  return `${BASE_PATH}${path.slice(1)}`;
}
```

### 2.2 修复的硬编码绝对路径
- **Vite 选项 (vite.config.ts)**：引入 `base` 选项接收 `VITE_BASE_URL`，并根据 base 动态生成开发代理（dev proxy）规则。
- **路由入口 (main.tsx)**：为 TanStack Router 配置 `basepath: BASE_PATH` 选项。
- **静态资源 (index.html)**：将 favicon、manifest 的绝对路径改为相对路径（`./`），让 Vite 自动添加子路径。
- **前端 API 层 (`src/api/*`)**：在 `launcherFetch` 和原生 `fetch` 的入口函数中，统一将请求路径包裹在 `withBase()` 内。
- **内联样式/硬编码路径修复**（易漏点）：
  - **站点 Logo**：在 [app-header.tsx](file:///home/jonhow/Desktop/picoclaw/web/frontend/src/components/app-header.tsx) 中将 `/logo_with_text.png` 改为 `{withBase("/logo_with_text.png")}`。
  - **飞书/Lark 图标**：在 [use-sidebar-channels.ts](file:///home/jonhow/Desktop/picoclaw/web/frontend/src/hooks/use-sidebar-channels.ts) 中将内联样式的 `url(/lark.svg)` 改为 `url(${withBase("/lark.svg")})`，防止直接访问根目录导致 404。

---

## 3. 一键式自动化构建助手 (`build.sh`)

为了规范本地编译，我们提供了位于 [scripts/build.sh](file:///home/jonhow/Desktop/picoclaw/scripts/build.sh) 的构建助手脚本。

### 3.1 核心特性
- **子路径处理**：使用 `-b /your-path/` 可以自动将前缀标准化并打包进前端。
- **交叉编译**：使用 `-p <os>` 和 `-a <arch>` 直接透传目标架构给 Makefile，保留 CGO 和 MIPS NaN2008 的 ELF 修改。
- **PATH 补全**：自动注册宿主机的全局 `pnpm` 目录，防止非交互 Shell 无法找到构建依赖。
- **编译审计**：彩色分步日志，打印每一步的耗时与生成的二进制最终文件大小。

### 3.2 常用命令
```bash
# 1. 默认根路径构建
./scripts/build.sh

# 2. 清理并进行自定义子路径编译
./scripts/build.sh --clean --base-path /picoclaw/

# 3. 跨平台交叉编译 ARM64 的核心客户端（不包含 Web 界面）
./scripts/build.sh --platform linux --arch arm64 --only-agent
```

---

## 4. Docker / Podman 跨架构零模拟编译打包

如果您要在 `ARM64` 设备（如树莓派、开发板等）上安装，但在当前的 `x86_64` 电脑上打包，传统的 QEMU 模拟构建在缺少内核模块支持时会报 `Exec format error` 错误。

为此，我们推荐采用 **“宿主机高速交叉编译，容器内纯静态打包”** 方案：

### 4.1 新建发布版打包配置
创建了专门的 [Dockerfile.release.arm64](file:///home/jonhow/Desktop/picoclaw/docker/Dockerfile.release.arm64)，该容器镜像**没有包含任何 `RUN` 动态指令**，仅仅在 Build 阶段把宿主机交叉编译出来的原生 ARM64 二进制 `COPY` 进去，在构建时不需要运行任何容器内指令，从而完美绕过了 QEMU 限制。

### 4.2 本地跨平台封包工作流
在您的 `x86_64` 电脑上运行以下组合指令：
```bash
# 1. 在本地交叉编译出 ARM64 的核心二进制和前端静态资源
./scripts/build.sh -c -p linux -a arm64 -b /picoclaw/

# 2. 建立暂存区（绕过 .dockerignore 对 build/ 目录的限制）
mkdir -p docker/build_tmp
cp build/picoclaw-linux-arm64 docker/build_tmp/
cp build/picoclaw-launcher-linux-arm64 docker/build_tmp/

# 3. 使用 Podman 打包标准的 ARM64 OCI 镜像
podman build --platform linux/arm64 -f docker/Dockerfile.release.arm64 -t picoclaw-launcher:subpath-arm64 .

# 4. 清除暂存区
rm -rf docker/build_tmp
```

### 4.3 镜像的版本控制与发布规范 (Tag 最佳实践)

为了长期可维护性与生产环境的安全回滚，强烈建议在本地构建时为每一次打包都打上**具体的版本号（如 v1.0.0）**，并保留**通用的动词 Tag（如 subpath-arm64 或 latest-arm64）**。

在构建时，可以通过多次使用 `-t` 参数，为同一个构建同时打上固定版本标签和测试标签：

#### 方式 A：使用语义化版本 (用于稳定版发布)
```bash
podman build --platform linux/arm64 -f docker/Dockerfile.release.arm64 \
  -t myregistry.com/picoclaw-launcher:v1.0.0-arm64 \
  -t myregistry.com/picoclaw-launcher:subpath-arm64 .
```

#### 方式 B：使用 Git Commit ID (用于日常迭代开发)
将当前的 Git Commit 唯一简写直接作为 Tag 写入，实现镜像与源码历史的精确对齐，极其方便问题排查：
```bash
COMMIT_ID=$(git rev-parse --short=8 HEAD)

podman build --platform linux/arm64 -f docker/Dockerfile.release.arm64 \
  -t myregistry.com/picoclaw-launcher:${COMMIT_ID}-arm64 \
  -t myregistry.com/picoclaw-launcher:subpath-arm64 .
```

### 4.4 镜像的物理分发与传送
打包好的 OCI 容器镜像默认保存在当前开发机的本地 Podman 引擎镜像数据库中。如要转移到目标 ARM 机器，需要在当前电脑上将其导出为物理压缩包（推荐使用带有固定版本号的镜像）：

```bash
# 1. 将打包好的固定版本镜像导出为物理 tar 压缩包
podman save myregistry.com/picoclaw-launcher:v1.0.0-arm64 | gzip > docker/images/picoclaw-launcher-v1.0.0-arm64.tar.gz

# 2. 通过网络将压缩包文件传输到目标 ARM 机器
scp docker/images/picoclaw-launcher-v1.0.0-arm64.tar.gz user@your_arm_device_ip:/home/user/
```

---

## 5. 目标设备 Docker 运行与 Nginx 反向代理配置

由于采用了反向代理剥离前缀的方案，**您不能直接去访问容器映射 of 18800 端口**。因为 Go 后端不知道子路径，当它收到请求并拦截到未登录时，会自动发出 `Location: /launcher-login` 的 302 重定向，导致浏览器跳转并脱离子路径命名空间。

必须通过配置了 `proxy_redirect` 的 Nginx 进行访问：

### 5.1 部署运行容器（在 ARM 目标设备）
传输镜像压缩包到 ARM 设备上并用 Docker 载入运行。为了让后端在不需要修改 Nginx 统一模板的情况下自动处理重定向，您可以通过传递环境变量 `PICOCLAW_BASE_PATH` 来告知后端子路径前缀：

```bash
# 1. 导入镜像
docker load < picoclaw-launcher-v1.0.0-arm64.tar.gz

# 2. 强力删除同名冲突容器
docker rm -f picoclaw-launcher-arm64

# 3. 启动运行新容器（通过 -e 传递子路径前缀）
docker run -d \
  --name picoclaw-launcher-arm64 \
  -p 18800:18800 \
  -e PICOCLAW_BASE_PATH=/picoclaw/ \
  -v ~/.picoclaw:/root/.picoclaw \
  myregistry.com/picoclaw-launcher:v1.0.0-arm64
```

### 5.2 Nginx 反向代理配置
由于已经通过环境变量 `PICOCLAW_BASE_PATH` 告诉了后端子路径前缀，Nginx **不需要**配置任何特殊的重定向处理头（如 `proxy_redirect` 或 `proxy_set_header X-Forwarded-Prefix`），直接使用您统一写入的常规反向代理模板即可：

```nginx
server {
    listen 80;
    server_name your_arm_device_ip;

    location /picoclaw/ {
        # 注意末尾的 "/" 非常关键，用于自动 strip 掉 "/picoclaw" 前缀并重写请求
        proxy_pass http://127.0.0.1:18800/; 

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持（控制台实时日志与状态所需）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

配置重载后，在浏览器访问 `http://<设备IP>/picoclaw/` 即可完美加载并运行您的 PicoClaw！

