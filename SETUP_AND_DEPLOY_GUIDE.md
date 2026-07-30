# golang-system 从零搭建与部署完整指南

> 本文档基于对项目实际代码的分析编写，涵盖：项目结构解析、VSCode（及兼容 IDE 如 Cursor / Windsurf / VSCodium）从零搭建步骤、本地运行、测试、以及 Docker / Kubernetes / Istio 部署全流程。

---

## 目录

1. [项目概览](#1-项目概览)
2. [环境准备（从零开始）](#2-环境准备从零开始)
3. [VSCode IDE 搭建步骤](#3-vscode-ide-搭建步骤)
4. [拉取代码与初始化项目](#4-拉取代码与初始化项目)
5. [本地运行（开发模式）](#5-本地运行开发模式)
6. [验证系统功能（端到端测试）](#6-验证系统功能端到端测试)
7. [测试与代码质量](#7-测试与代码质量)
8. [运行部署](#8-运行部署)
9. [常见问题排查](#9-常见问题排查)
10. [附录：端口与配置速查表](#10-附录端口与配置速查表)

---

## 1. 项目概览

### 1.1 项目定位

`golang-system` 是一个生产级 Go 微服务系统模板，采用**六角架构（Ports & Adapters）**和**事件驱动**设计模式，通过 Go Workspace 管理多个模块。

### 1.2 模块组成

| 模块 | 类型 | 说明 |
|------|------|------|
| `golang-app` | 微服务 | 商品服务（Item Service），端口 `:8080`，数据库 `app_db`（5432） |
| `golang-app2` | 微服务 | 订单服务（Order Service），端口 `:8081`，数据库 `app2_db`（5433） |
| `golang-commons` | 共享库 | 日志（zerolog）、HTTP 工具、中间件、NATS 封装、错误处理、Metrics |
| `golang-events` | 共享库 | 事件契约定义（Event 泛型信封、NATS Subject 常量、Payload 结构体） |
| `golang-system-chart` | Helm Chart | 整个系统的 Kubernetes 部署 Chart |
| `istio-config` | 配置 | Istio 服务网格治理配置（熔断、限流、网关） |

### 1.3 技术栈

- **语言**: Go 1.26.1（`go.work` 工作区模式）
- **HTTP 框架**: go-chi/chi v5
- **数据库**: PostgreSQL 16（pgx/v5 驱动），golang-migrate 迁移工具
- **消息队列**: NATS 2.10（含 JetStream）
- **日志**: zerolog（DEV 控制台输出 / PROD JSON 输出）
- **监控**: Prometheus Metrics（`/metrics` 端点）
- **部署**: Docker Compose / Helm / Kubernetes (RKE2) + Istio 服务网格

### 1.4 服务间通信

| 模式 | 方向 | 说明 |
|------|------|------|
| HTTP REST | app2 → app | 创建订单时同步校验商品是否存在 |
| NATS Pub/Sub | app → app2 | 删除商品时自动取消关联的待处理订单 |
| NATS Pub/Sub | app2 → app | 创建订单后通知商品服务追踪引用 |
| NATS Req-Reply | app2 → app | 通过 NATS 同步查询商品（HTTP 替代方案） |

---

## 2. 环境准备（从零开始）

以下步骤以 Linux（Ubuntu/Debian 系）为例，macOS 使用 `brew` 替代 `apt`，Windows 建议使用 WSL2。

### 2.1 安装 Go 1.26.1+

```bash
# 下载并安装 Go 1.26.1（以 linux/amd64 为例）
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz

# 配置环境变量（写入 ~/.bashrc 或 ~/.zshrc）
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# 生效并验证
source ~/.bashrc
go version   # 应输出 go version go1.26.1 linux/amd64
```

> 可选：配置国内代理加速依赖下载
> ```bash
> go env -w GOPROXY=https://goproxy.cn,direct
> ```

### 2.2 安装 Docker 与 Docker Compose

```bash
# Ubuntu 示例
sudo apt update
sudo apt install -y docker.io docker-compose-v2

# 将当前用户加入 docker 组（免 sudo）
sudo usermod -aG docker $USER
newgrp docker

# 验证
docker version
docker compose version
```

### 2.3 安装 Make 与基础工具

```bash
sudo apt install -y make git curl jq
```

### 2.4 安装 Go 开发工具链

```bash
# 代码检查工具（Makefile lint 目标依赖）
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# 数据库迁移 CLI（Makefile migrate 目标会自动安装，也可提前装好）
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# 调试器 Delve（VSCode 调试 Go 必需）
go install github.com/go-delve/delve/cmd/dlv@latest
```

### 2.5 （可选）安装部署工具

仅当需要部署到 Kubernetes 时安装：

```bash
# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Helm 3
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# istioctl（Istio 服务网格）
curl -L https://istio.io/downloadIstio | sh -
sudo cp istio-*/bin/istioctl /usr/local/bin/
```

---

## 3. VSCode IDE 搭建步骤

以下步骤同样适用于 Cursor、Windsurf、VSCodium 等 VSCode 兼容 IDE。

### 3.1 安装 VSCode

```bash
# Ubuntu/Debian
sudo apt install -y code

# 或从官网下载: https://code.visualstudio.com/
```

### 3.2 安装必装插件

打开 VSCode，进入扩展面板（`Ctrl+Shift+X`），搜索并安装：

| 插件 | ID | 用途 |
|------|-----|------|
| **Go**（官方） | `golang.go` | Go 语言支持（必装，含 gopls 语言服务器） |
| Makefile Tools | `ms-vscode.makefile-tools` | Makefile 目标可视化运行 |
| YAML | `redhat.vscode-yaml` | YAML 语法校验（K8s/Helm/Compose 文件） |
| Docker | `ms-azuretools.vscode-docker` | Dockerfile / Compose 管理 |
| Kubernetes | `ms-kubernetes-tools.vscode-kubernetes-tools` | K8s 资源管理（部署阶段用） |
| GitLens | `eamodio.gitlens` | Git 增强（可选） |

安装 Go 插件后，首次打开 `.go` 文件时会提示安装 Go 工具（gopls、staticcheck 等），**点击 "Install All" 全部安装**。也可手动执行：

```
Ctrl+Shift+P → Go: Install/Update Tools → 全选 → 确定
```

### 3.3 配置 VSCode 工作区设置

在项目根目录创建 `.vscode/settings.json`：

```json
{
  "go.useLanguageServer": true,
  "go.gopath": "",
  "go.toolsManagement.autoUpdate": true,
  "gopls": {
    "ui.semanticTokens": true,
    "experimentalWorkspaceModule": true
  },
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "go.testFlags": ["-race"],
  "go.coverOnSave": false,
  "editor.formatOnSave": true,
  "editor.codeActionsOnSave": {
    "source.organizeImports": true
  },
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  },
  "files.eol": "\n"
}
```

> **关键点**：本项目使用 `go.work` 工作区，`gopls` 0.14+ 已原生支持 workspace 模式，打开**项目根目录**（而非单个子模块）即可获得跨模块跳转、重构能力。若跨模块跳转失效，在 `settings.json` 中确认没有设置 `go.buildTags` 限制，并执行 `Ctrl+Shift+P → Go: Restart Language Server`。

### 3.4 配置调试器 launch.json

创建 `.vscode/launch.json`，支持一键调试两个微服务：

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "调试 golang-app (商品服务 :8080)",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/golang-app/cmd/server",
      "cwd": "${workspaceFolder}/golang-app",
      "env": {
        "APP_PORT": "8080",
        "APP_MODE": "DEV",
        "LOG_LEVEL": "debug",
        "DB_HOST": "localhost",
        "DB_PORT": "5432",
        "DB_USER": "app_user",
        "DB_NAME": "app_db",
        "DB_PASSWORD": "app_password",
        "NATS_URL": "nats://localhost:4222"
      }
    },
    {
      "name": "调试 golang-app2 (订单服务 :8081)",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/golang-app2/cmd/server",
      "cwd": "${workspaceFolder}/golang-app2",
      "env": {
        "APP_PORT": "8081",
        "APP_MODE": "DEV",
        "LOG_LEVEL": "debug",
        "DB_HOST": "localhost",
        "DB_PORT": "5433",
        "DB_USER": "app2_user",
        "DB_NAME": "app2_db",
        "DB_PASSWORD": "app2_password",
        "NATS_URL": "nats://localhost:4222",
        "ITEM_SERVICE_URL": "http://localhost:8080"
      }
    },
    {
      "name": "附加到运行中的进程 (Attach)",
      "type": "go",
      "request": "attach",
      "mode": "local",
      "processId": 0
    }
  ],
  "compounds": [
    {
      "name": "同时调试两个服务",
      "configurations": ["调试 golang-app (商品服务 :8080)", "调试 golang-app2 (订单服务 :8081)"],
      "stopAll": true
    }
  ]
}
```

> **注意**：配置完全走环境变量，无 .env 文件加载逻辑。`env` 中显式列出的值与 `docker-compose.yml` 一致（也是代码中的默认值），显式声明便于按需修改。

### 3.5 配置任务 tasks.json（可选）

创建 `.vscode/tasks.json`，将常用 Make 命令集成到 `Ctrl+Shift+B`：

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "make: 启动基础设施",
      "type": "shell",
      "command": "docker compose up -d",
      "group": "build"
    },
    {
      "label": "make: 数据库迁移",
      "type": "shell",
      "command": "make migrate",
      "group": "build"
    },
    {
      "label": "make: 构建全部",
      "type": "shell",
      "command": "make build",
      "group": { "kind": "build", "isDefault": true }
    },
    {
      "label": "make: 测试",
      "type": "shell",
      "command": "make test",
      "group": { "kind": "test", "isDefault": true }
    },
    {
      "label": "make: Lint",
      "type": "shell",
      "command": "make lint",
      "group": "build"
    }
  ]
}
```

### 3.6 验证 IDE 环境

1. 打开项目根目录：`code /home/darrell/workspace/golang-system`
2. 打开 `golang-app/internal/service/item_service.go`，确认无红线报错
3. 按住 `Ctrl` 点击 `golang-commons` 中的类型引用，确认可跨模块跳转
4. 在测试文件中点击 `run test` 按钮，确认测试可运行

---

## 4. 拉取代码与初始化项目

```bash
# 1. 克隆项目
git clone <仓库地址> golang-system
cd golang-system

# 2. 确认 go.work 工作区（已随仓库提交，无需修改）
cat go.work
# go 1.26.1
# use ( ./golang-app ./golang-app2 ./golang-commons ./golang-events )

# 3. 下载所有模块依赖
make tidy
# 或手动:
# cd golang-commons && go mod tidy && cd ..
# cd golang-events && go mod tidy && cd ..
# cd golang-app && go mod tidy && cd ..
# cd golang-app2 && go mod tidy && cd ..

# 4. 编译验证（应无错误）
make build
```

> **go.work 说明**：`use` 指令声明的 4 个模块共享依赖解析，`golang-app/go.mod` 中的 `replace` 指令将共享库指向本地相对路径，修改 `golang-commons` / `golang-events` 后立即生效，无需发布版本。

---

## 5. 本地运行（开发模式）

### 5.1 启动基础设施（数据库 + 消息队列）

`docker-compose.yml` 只包含基础设施（2 个 PostgreSQL + NATS），应用服务在本地以原生进程运行：

```bash
docker compose up -d

# 确认三个容器健康
docker compose ps
# golang-app-db    postgres:16-alpine   Up (healthy)   0.0.0.0:5432->5432/tcp
# golang-app2-db   postgres:16-alpine   Up (healthy)   0.0.0.0:5433->5432/tcp
# golang-nats      nats:2.10-alpine     Up (healthy)   0.0.0.0:4222->4222/tcp, 0.0.0.0:8222->8222/tcp
```

### 5.2 执行数据库迁移

```bash
make migrate
# 等价于对两个库分别执行 migrate up：
#   app_db  ← golang-app/db/migrations
#   app2_db ← golang-app2/db/migrations

# 查看迁移版本
make migrate-version
```

其他迁移命令：

```bash
make migrate-down COUNT=1                    # 回滚 1 个版本
make migrate-create NAME=create_xxx_table    # 新建迁移文件（两个服务各生成一份）
```

### 5.3 启动微服务

**方式 A：VSCode 调试启动（推荐开发时使用）**

先确保 5.1/5.2 完成，然后按 `F5` 选择"同时调试两个服务"复合配置。

**方式 B：命令行启动**

```bash
# 构建
make build   # 产物为 golang-app/server 和 golang-app2/server

# 终端 1：商品服务（配置默认值已与 docker-compose 对齐，直接启动即可）
cd golang-app && ./server

# 终端 2：订单服务
cd golang-app2 && ./server
```

> **说明**：两个服务的数据库默认值（`app_user`/`app_db`、`app2_user`/`app2_db` 等）已对齐 docker-compose，本地开发开箱即用；所有配置均可通过环境变量覆盖，生产环境务必显式设置强密码（PROD 模式下使用默认开发密码会输出告警）。

### 5.4 验证服务运行

```bash
curl http://localhost:8080/admin/health/readiness   # {"status":"ok"}
curl http://localhost:8081/admin/health/readiness   # {"status":"ok"}
curl http://localhost:8080/metrics                  # Prometheus 指标
curl http://localhost:8222                          # NATS 监控页（浏览器打开）
```

---

## 6. 验证系统功能（端到端测试）

完整走一遍"创建商品 → 创建订单 → 删除商品 → 订单自动取消"的事件驱动流程：

```bash
# 第 1 步：创建商品
ITEM_ID=$(curl -s -X POST http://localhost:8080/rest/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name": "widget A", "description": "a quality component", "tags": ["demo"]}' \
  | jq -r '.id')
echo "created item: $ITEM_ID"

# 第 2 步：创建订单（app2 会同步 HTTP 调用 app 校验商品存在）
ORDER_ID=$(curl -s -X POST http://localhost:8081/rest/v1/orders \
  -H "Content-Type: application/json" \
  -d "{\"note\": \"first order\", \"items\": [{\"item_id\": \"$ITEM_ID\", \"quantity\": 2, \"price\": 9.99}]}" \
  | jq -r '.id')
echo "created order: $ORDER_ID"

# 第 3 步：删除商品（app 发布 item.deleted 事件到 NATS）
curl -s -X DELETE "http://localhost:8080/rest/v1/items/$ITEM_ID"

# 第 4 步：稍等片刻，验证订单已被自动取消（app2 消费事件触发）
sleep 1
curl -s "http://localhost:8081/rest/v1/orders/$ORDER_ID" | jq '.status'
# 预期输出: "canceled"
```

---

## 7. 测试与代码质量

```bash
# 运行全部单元测试（带竞态检测）
make test

# 单模块测试 + 覆盖率
cd golang-app && go test -race -coverprofile=cover.out ./... && go tool cover -html=cover.out

# Lint 检查
make lint

# 完整流水线（lint + test + build）
make all
```

VSCode 中：测试函数上方的 `run test | debug test` 按钮可直接运行/调试单个测试；`Ctrl+Shift+B` 运行默认构建任务。

---

## 8. 运行部署

部署提供三种路径，按场景选择：

| 场景 | 方式 | 章节 |
|------|------|------|
| 本地一体化体验 | Docker Compose（仅基础设施）+ 本地进程 | 8.1 |
| 测试/预发 K8s 集群 | Helm Chart | 8.2 |
| 生产 RKE2 集群 + 服务网格 | Helm + Istio 自动化脚本 | 8.3 |

### 8.1 本地一体化运行

即第 5 章流程的封装：

```bash
make docker-up    # 启动基础设施并等待就绪
make migrate      # 数据库迁移
make build        # 构建二进制
# 分别启动两个 ./server 进程（见 5.3）
make docker-down  # 停止基础设施
```

### 8.2 Kubernetes + Helm 部署

**前置条件**：可用的 K8s 集群（kind / k3s / minikube / RKE2 均可）、kubectl、helm。

**Step 1：构建并推送镜像**

两个服务的 `Dockerfile` 已随仓库提供（`golang-app/Dockerfile`、`golang-app2/Dockerfile`，多阶段构建，镜像内含 `server` + `migrate` CLI + 迁移 SQL）：

```bash
# 在仓库根目录构建（context 必须是根目录，因为要 COPY go.work 与共享模块）
docker build -f golang-app/Dockerfile  -t ghcr.io/<org>/golang-app:v1.0.0 .
docker build -f golang-app2/Dockerfile -t ghcr.io/<org>/golang-app2:v1.0.0 .

# 可选：注入版本号（默认 dev）
docker build -f golang-app/Dockerfile --build-arg VERSION=v1.0.0 -t ghcr.io/<org>/golang-app:v1.0.0 .

docker push ghcr.io/<org>/golang-app:v1.0.0
docker push ghcr.io/<org>/golang-app2:v1.0.0
```

**Step 2：修改 `golang-system-chart/values.yaml`**

```yaml
global:
  imageRegistry: ghcr.io        # 你的镜像仓库
golangApp:
  image: { repository: <org>/golang-app, tag: v1.0.0 }
golangApp2:
  image: { repository: <org>/golang-app2, tag: v1.0.0 }
postgresApp:
  password: <强密码>             # 生产必须修改 change-me
postgresApp2:
  password: <强密码>
```

**Step 3：部署**

```bash
make helm-deps      # 更新 Chart 依赖
make helm-template  # 本地渲染检查（可选）
make helm-install   # 部署到 golang-system 命名空间

# 或手动:
helm install golang-system ./golang-system-chart \
  --namespace golang-system --create-namespace
```

**Step 4：访问与验证**

```bash
kubectl -n golang-system port-forward svc/golang-system-golang-app 8080:80 &
kubectl -n golang-system port-forward svc/golang-system-golang-app2 8081:80 &
curl http://localhost:8080/admin/health/readiness
curl http://localhost:8081/admin/health/readiness
```

**Step 5：数据库迁移（K8s 环境）**

镜像内置了迁移 CLI（工作目录 `/home/appuser`，`MIGRATE_PATH` 默认 `./db/migrations`）：

```bash
kubectl -n golang-system exec -it deploy/golang-system-golang-app -- \
  sh -c 'DATABASE_URL="postgres://app:CHANGE_ME@postgres-app:5432/golang-app?sslmode=disable" ./migrate up'

kubectl -n golang-system exec -it deploy/golang-system-golang-app2 -- \
  sh -c 'DATABASE_URL="postgres://app2:CHANGE_ME@postgres-app2:5432/golang-app2?sslmode=disable" ./migrate up'
```

**卸载**：

```bash
make helm-uninstall
```

### 8.3 RKE2 + Istio 生产部署（一键自动化）

项目提供 `deploy-istio.sh` 自动化脚本和完整的 Makefile Istio 命令组，适用于 RKE2 集群。

**前置条件**：RKE2 集群可用，且已安装 `kubectl`、`helm`、`istioctl`。

**方式 A：一键脚本（推荐）**

```bash
chmod +x deploy-istio.sh

# 完整安装：Istio + 微服务 + 服务治理 + 可观测性 + 验证
./deploy-istio.sh install

# 分步执行也可以：
./deploy-istio.sh istio           # 仅安装 Istio
./deploy-istio.sh microservices   # 仅部署微服务
./deploy-istio.sh governance      # 仅配置熔断/限流/网关
./deploy-istio.sh observability   # 仅安装 Kiali/Prometheus/Grafana/Jaeger
./deploy-istio.sh verify          # 验证部署
./deploy-istio.sh test            # 运行治理测试（fortio 压测熔断/限流）
./deploy-istio.sh cleanup         # 清理全部环境
```

**方式 B：Makefile 命令**

```bash
make istio-all
# 等价于: istio-install → istio-deploy → istio-governance → istio-verify
```

**脚本执行内容说明**：

1. `istioctl install -f istio-config/istio-config.yaml` 安装控制面
2. 给 `default` 命名空间打 `istio-injection=enabled` 标签开启 Sidecar 自动注入
3. `helm upgrade --install` 部署两个微服务（自动注入 istio-proxy）
4. 应用治理配置：`destination-rule-circuit-breaking.yaml`（熔断）、`envoy-filter-rate-limiting.yaml`（限流）、`gateway.yaml` + `virtual-service-gateway.yaml`（API 网关）
5. 安装 Kiali / Prometheus / Grafana / Jaeger 可观测性套件

**访问外部流量**：

```bash
# 获取 Ingress Gateway 地址
INGRESS_IP=$(kubectl get svc istio-ingressgateway -n istio-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# 通过网关访问（Host 头匹配 gateway.yaml 中配置的域名）
curl http://$INGRESS_IP/rest/v1/items  -H "Host: api.example.com"
curl http://$INGRESS_IP/rest/v1/orders -H "Host: api.example.com"

# 打开可观测性面板
make istio-dashboard
# Kiali: http://localhost:20001  Grafana: http://localhost:3000
# Jaeger: http://localhost:16686  Prometheus: http://localhost:9090
```

### 8.4 生产上线检查清单

- [ ] `APP_MODE=PROD`（启用 JSON 结构化日志）
- [ ] 所有数据库使用强密码，通过 Kubernetes Secret / Vault 注入
- [ ] 配置资源 requests/limits 与 HPA（Chart values 已内置模板）
- [ ] 配置 CORS 白名单（`CORS_ALLOWED_ORIGINS` 不要使用 `*`，PROD 模式下配置校验会输出警告）
- [ ] 启用 TLS（`TLS_ENABLED=true` + `TLS_CERT_FILE`/`TLS_KEY_FILE`）
- [ ] NATS JetStream 开启持久化，避免事件丢失
- [ ] 配置 NetworkPolicy 限制服务间流量
- [ ] Prometheus 抓取 `/metrics` 端点，配置 Grafana 看板与告警

---

## 9. 常见问题排查

| 问题 | 原因与解决 |
|------|-----------|
| 服务启动报数据库连接失败 | 确认 `docker compose ps` 中数据库容器为 healthy；若自定义了 compose 凭据，需通过 `DB_USER`/`DB_PASSWORD`/`DB_NAME` 环境变量同步覆盖默认值 |
| `migrate` 命令不存在 | 执行 `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`，并确认 `$GOPATH/bin` 在 PATH 中 |
| VSCode 跨模块跳转/补全失效 | 确认打开的是项目根目录（含 go.work）；`Ctrl+Shift+P → Go: Restart Language Server` |
| `make lint` 报错找不到 golangci-lint | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| 调试时断点不命中 | 确认安装了 dlv；确认 launch.json 的 `program` 指向 `cmd/server` 目录而非单个文件 |
| 端口被占用（8080/5432/4222） | `sudo lsof -i :<port>` 找到进程并停止，或修改 docker-compose 端口映射 |
| K8s 中 Pod 一直 CrashLoopBackOff | `kubectl logs <pod> -n golang-system` 查看，多为数据库未就绪或密码不匹配；检查 values.yaml 密码与 Secret |
| Istio 注入未生效 | 确认命名空间标签：`kubectl get ns default --show-labels`，应有 `istio-injection=enabled`；已运行的 Pod 需重启才会注入 |

---

## 10. 附录：端口与配置速查表

### 端口

| 服务 | 端口 | 说明 |
|------|------|------|
| golang-app | 8080 | 商品服务 HTTP API（`/rest/v1/items`） |
| golang-app2 | 8081 | 订单服务 HTTP API（`/rest/v1/orders`） |
| PostgreSQL (app_db) | 5432 | 商品库（app_user / app_password） |
| PostgreSQL (app2_db) | 5433 | 订单库（app2_user / app2_password） |
| NATS | 4222 | 消息总线客户端端口 |
| NATS Monitoring | 8222 | NATS 监控页面 |
| /metrics | 同服务端口 | Prometheus 指标端点 |
| /admin/health/liveness、readiness | 同服务端口 | 健康探针 |

### 核心环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_PORT` | 8080 / 8081 | HTTP 监听端口 |
| `APP_MODE` | DEV | `DEV` 彩色控制台日志 / `PROD` JSON 日志 |
| `LOG_LEVEL` | info | debug / info / warn / error |
| `DB_HOST` / `DB_PORT` | localhost / 5432 | PostgreSQL 地址 |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | — | 数据库凭据（必须与部署环境一致） |
| `NATS_URL` | nats://localhost:4222 | NATS 连接地址 |
| `QUEUE_GROUP` | golang-app-queue 等 | NATS 队列组（多实例负载均衡、防重复消费） |
| `ITEM_SERVICE_URL` | http://localhost:8080 | （仅 app2）商品服务地址 |
| `TLS_ENABLED` / `TLS_CERT_FILE` / `TLS_KEY_FILE` | false | HTTPS 配置 |
| `CORS_ALLOWED_ORIGINS` 等 | `*` | CORS 配置（PROD 必须显式指定） |

### Makefile 命令速查

```bash
make help              # 查看全部命令
make all               # lint + test + build
make docker-up/-down   # 启停基础设施
make migrate           # 数据库迁移
make build / test / lint / tidy / clean
make helm-deps / helm-template / helm-install / helm-uninstall
make istio-all         # Istio 一键部署（install + deploy + governance + verify）
```
