# Golang System — 多微服务系统

一个面向生产环境的多服务 Go 微服务平台模板，遵循 **六角架构 (端口与适配器)** 和 **事件驱动** 设计模式。

> English version: [README.md](./README.md)

## 目录

1. [系统架构](#1-系统架构)
2. [项目结构](#2-项目结构)
3. [子项目详解](#3-子项目详解)
4. [服务间通讯](#4-服务间通讯)
5. [NATS 事件驱动消息](#5-nats-事件驱动消息)
6. [API 接口参考](#6-api-接口参考)
7. [本地开发指南](#7-本地开发指南)
8. [部署指南](#8-部署指南)
9. [CI/CD 流水线](#9-cicd-流水线)
10. [配置参考](#10-配置参考)
11. [命令参考](#11-命令参考)

---

## 1. 系统架构

```text
                      ┌────────────────┐
                      │   客户端 /      │
                      │   API 网关      │
                      └───────┬────────┘
                              │ REST
              ┌───────────────┴───────────────┐
              ▼                               ▼
      ┌───────────────┐               ┌───────────────┐
      │   golang-app   │ ─── HTTP ───► │   golang-app2  │
      │   (商品服务)    │ ◄── NATS ──  │   (订单服务)    │
      │    :8080       │               │    :8081       │
      └───────┬───────┘               └───────┬───────┘
              │                               │
   ┌──────────┴─────┐                ┌─────────┴──────┐
   │   PostgreSQL    │                │   PostgreSQL    │
   │    (app_db)     │                │   (app2_db)     │
   │   端口: 5432    │                │   端口: 5433    │
   └─────────────────┘                └────────────────┘
                       │                ▲
                       └──────┬─────────┘
                              ▼
                  ┌──────────────────────┐
                  │        NATS           │
                  │  JetStream :4222      │
                  │  监控面板  :8222      │
                  └──────────────────────┘
```

### 通讯模式总览

| 模式 | 方向 | 说明 |
|------|------|------|
| HTTP REST       | app2 → app        | 创建订单时同步调用商品服务验证商品是否存在 |
| NATS 发布/订阅  | app → app2        | 商品删除时自动取消关联的待处理订单 |
| NATS 发布/订阅  | app2 → app        | 订单创建时通知商品服务记录引用 |
| NATS 请求-应答  | app2 → app        | 通过 NATS 同步查询商品详情 (HTTP 的替代方案) |

---

## 2. 项目结构

```text
golang-system/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml            # CI 流水线: 代码检查 → 测试 → 构建 → Docker 镜像
│   │   ├── cd.yml            # CD 流水线: dev → staging → production
│   │   └── release.yml       # Release 流水线: 多架构二进制 + 镜像 + Helm Chart
│   └── dependabot.yml
├── golang-commons/             # [共享库] 通用可复用包
│   ├── apperror/errors.go
│   ├── database/postgres.go
│   ├── httpclient/client.go
│   ├── httputil/{response.go, middleware.go}
│   ├── logger/logger.go
│   ├── messaging/nats.go
│   └── go.mod
├── golang-events/              # [共享库] 事件契约
│   ├── events.go
│   └── go.mod
├── golang-app/                 # [服务] 商品微服务 (:8080)
│   ├── cmd/server/main.go
│   ├── internal/{app, config, domain, service, ports, adapter}
│   ├── db/migrations/
│   ├── api/openapi.yaml
│   ├── helm/
│   └── Dockerfile
├── golang-app2/                # [服务] 订单微服务 (:8081)
│   ├── cmd/server/main.go
│   ├── internal/{app, config, domain, client, service, ports, adapter}
│   ├── db/migrations/
│   ├── api/openapi.yaml
│   ├── helm/
│   └── Dockerfile
├── golang-system-chart/        # [Helm] 总 Helm Chart
├── docker-compose.yml
├── go.work
├── Makefile
├── README.md                   # 英文文档
└── README_CN.md                # 中文文档 (本文件)
```

---

## 3. 子项目详解

### 3.1 golang-commons (共享库)

所有服务复用的通用包，**不依赖任何业务服务模块**。

| 包名 | 用途 | 关键类型/函数 |
|------|------|---------------|
| `apperror`   | 结构化应用错误           | `AppError`、`BadInput()`、`NotFound()`、`Internal()`、`Conflict()` |
| `database`   | PostgreSQL 工具          | `Config`、`DSN()`、`NewHealthChecker()` |
| `httpclient` | 服务间 HTTP 客户端       | `Client`、`New()`、`Get()`/`Post()`/`Put()`/`Delete()`、`IsNotFound()` |
| `httputil`   | HTTP 响应工具 + 中间件   | `RequestIDMiddleware`、`Error()`、`Created()`、`NoContent()`、`MaxBodySizeMiddleware`、`ContentTypeMiddleware`、`LoggingMiddleware`、`AuthContextMiddleware`、`RecoveryMiddleware` |
| `logger`     | Zerolog 日志初始化       | `Setup(level, mode)` (DEV 彩色控制台 / PROD JSON) |
| `messaging`  | NATS 封装                | `Publisher` 接口、`NatsPublisher`、`NoopPublisher`、`Connect()` |

### 3.2 golang-events (共享事件模型)

```go
type Event[T any] struct {
    ID        string    `json:"id"`
    Type      string    `json:"type"`
    Source    string    `json:"source"`
    Timestamp time.Time `json:"timestamp"`
    Data      T         `json:"data"`
}
```

### NATS Subject 定义

| Subject | 方向 | 模式 | 负载 |
|---------|------|------|------|
| `item.created`     | app → 全部  | 发布/订阅 | `ItemCreatedData{ID, Name, Description, Status}` |
| `item.updated`     | app → 全部  | 发布/订阅 | `ItemUpdatedData{ID, Name, Description, Status}` |
| `item.deleted`     | app → 全部  | 发布/订阅 | `ItemDeletedData{ID}` |
| `item.get_by_id`   | app2 → app  | 请求-应答 | `GetItemRequest{ID}` → `GetItemResponse{ID, Name, Description, Status}` |
| `order.created`    | app2 → 全部 | 发布/订阅 | `OrderCreatedData{ID, Items[], Total, Status}` |
| `order.updated`    | app2 → 全部 | 发布/订阅 | `OrderUpdatedData{ID, Items[], Total, Status}` |
| `order.canceled`   | app2 → 全部 | 发布/订阅 | `OrderCanceledData{ID, Reason}` |

### 3.3 golang-app (商品服务)

| 维度 | 说明 |
|------|------|
| **端口** | 8080 |
| **数据库** | PostgreSQL (`golang-app`) |
| **架构** | 六角架构 |
| **API 前缀** | `/rest/v1/items` |
| **发布事件** | `item.created`、`item.updated`、`item.deleted` |
| **订阅事件** | `order.created`、`item.get_by_id`、`item.external.create` |

### 3.4 golang-app2 (订单服务)

| 维度 | 说明 |
|------|------|
| **端口** | 8081 |
| **数据库** | PostgreSQL (`golang-app2`) |
| **架构** | 六角架构 |
| **API 前缀** | `/rest/v1/orders` |
| **服务间调用** | 创建订单前通过 HTTP 调用商品服务验证商品是否存在 |
| **发布事件** | `order.created`、`order.updated`、`order.canceled` |
| **订阅事件** | `item.deleted`、`item.updated` |

### 3.5 golang-system-chart (总 Helm Chart)

以单个 Helm Release 部署整个系统：两个应用 + 两个 PostgreSQL + NATS。

---

## 4. 服务间通讯

### 4.1 HTTP - 同步商品验证

```go
// internal/client/item_client.go
func (c *ItemClient) GetByID(ctx context.Context, id string) (*domain.ItemView, error) {
    var item domain.ItemView
    if err := c.client.Get(ctx, fmt.Sprintf("/rest/v1/items/%s", id), &item); err != nil {
        return nil, err
    }
    return &item, nil
}
```

### `httpclient.Client` 特性

- **请求头传播**：自动将 `X-Request-ID`、`X-Organization-ID`、`X-User-ID` 传递到下游
- **结构化错误**：非 2xx 返回 `ServiceError` (含状态码和响应体)
- **可配置超时**：默认 30 秒，可通过 `httpclient.WithTimeout()` 自定义
- **请求日志**：记录调用耗时、目标服务、响应状态

### 4.2 NATS - 异步事件驱动

**流程 1: 创建订单 → 跟踪商品引用**

**流程 2: 删除商品 → 自动取消订单**

**流程 3: NATS 请求-应答 (替代 HTTP 同步查询)**

---

## 5. NATS 事件驱动消息

### 5.1 事件信封

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "item.created",
  "source": "golang-app",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": { "id": "...", "name": "widget A", "description": "...", "status": "active" }
}
```

### 5.2 队列组

| 服务 | 队列组 | 作用 |
|------|--------|------|
| `golang-app`  | `golang-app-queue`  | 确保每个事件只被一个实例处理 |
| `golang-app2` | `golang-app2-queue` | 防止重复取消订单 |

### 5.3 事件处理器

**golang-app 订阅：**

| Subject | 处理器 | 业务逻辑 |
|---------|--------|----------|
| `order.created`         | `handleOrderCreated()`      | 记录新订单引用了哪些商品 |
| `item.get_by_id`        | `handleGetItemByID()`       | 请求-应答：返回商品详情 |
| `item.external.create`  | `handleExternalCreate()`    | 接收外部系统的商品创建事件 |

**golang-app2 订阅：**

| Subject | 处理器 | 业务逻辑 |
|---------|--------|----------|
| `item.deleted` | `handleItemDeleted()` | 查找所有引用已删除商品的待处理订单并自动取消 |
| `item.updated` | `handleItemUpdated()` | 记录商品变更日志，用于可见性控制和缓存失效 |

---

## 6. API 接口参考

### 6.1 golang-app - 商品 API (`:8080`)

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET`    | `/rest/v1/items?limit=&offset=&status=` | 分页查询商品列表，支持状态筛选 |
| `GET`    | `/rest/v1/items/{id}`                   | 获取商品详情 |
| `POST`   | `/rest/v1/items`                        | 创建新商品 |
| `PATCH`  | `/rest/v1/items/{id}`                   | 部分更新商品 |
| `DELETE` | `/rest/v1/items/{id}`                   | 删除商品 |
| `GET`    | `/admin/health/liveness`                | 存活探针 |
| `GET`    | `/admin/health/readiness`               | 就绪探针 (检查数据库连接) |

#### 创建商品请求/响应示例

请求:
```json
{
  "name": "widget A",
  "description": "一款优质组件",
  "tags": ["电子产品", "促销"]
}
```

响应:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "widget A",
  "description": "一款优质组件",
  "status": "active",
  "tags": ["电子产品", "促销"],
  "created_at": "2024-01-15T10:30:00Z"
}
```

### 6.2 golang-app2 - 订单 API (`:8081`)

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET`    | `/rest/v1/orders?limit=&offset=&status=` | 分页查询订单列表 |
| `GET`    | `/rest/v1/orders/{id}`                   | 获取订单详情 |
| `POST`   | `/rest/v1/orders`                        | 创建新订单 |
| `PATCH`  | `/rest/v1/orders/{id}`                   | 部分更新订单 |
| `DELETE` | `/rest/v1/orders/{id}`                   | 取消订单 |
| `GET`    | `/admin/health/liveness`                 | 存活探针 |
| `GET`    | `/admin/health/readiness`                | 就绪探针 |

#### 创建订单请求/响应示例

请求:
```json
{
  "note": "加急配送",
  "items": [
    { "item_id": "550e8400-e29b-41d4-a716-446655440000", "quantity": 2, "price": 29.99 }
  ]
}
```

响应:
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "total": 59.98,
  "note": "加急配送",
  "items": [
    {
      "id": "...",
      "item_id": "550e8400-e29b-41d4-a716-446655440000",
      "item_name": "widget A",
      "quantity": 2,
      "price": 29.99
    }
  ],
  "created_at": "2024-01-15T10:35:00Z"
}
```

#### 订单状态流转

```text
pending (待处理) → confirmed (已确认) → shipped (已发货)
            │
            └──────────────→ canceled (已取消)
```

---

## 7. 本地开发指南

### 7.1 前置条件

| 工具 | 版本 | 用途 |
|------|------|------|
| Go | 1.26.1+ | 语言运行时 |
| Docker | 24+ | 容器运行时 |
| Docker Compose | v2+ | 多容器编排 |
| Make | 任意 | 构建自动化 |
| golangci-lint | 最新 | (可选) 代码检查 |
| Helm | 3.1+ | (可选) K8s 部署 |

### 7.2 快速启动 - Docker Compose (推荐)

```bash
cd golang-system
make docker-up
make migrate
curl http://localhost:8080/admin/health/readiness
curl http://localhost:8081/admin/health/readiness
make docker-logs
make docker-down
```

#### `docker-up` 后的服务端口

| 服务 | 地址 |
|------|------|
| golang-app  (商品服务)  | http://localhost:8080 |
| golang-app2 (订单服务)  | http://localhost:8081 |
| PostgreSQL (商品库)    | localhost:5432 |
| PostgreSQL (订单库)    | localhost:5433 |
| NATS 消息总线          | localhost:4222 |
| NATS 监控面板          | http://localhost:8222 |

### 7.3 端到端测试流程

```bash
# 步骤 1: 创建一个商品
ITEM_ID=$(curl -s -X POST http://localhost:8080/rest/v1/items \
  -H "Content-Type: application/json" \
  -d '{"name": "widget A", "description": "一款优质组件", "tags": ["演示"]}' \
  | jq -r '.id')

echo "已创建商品: $ITEM_ID"

# 步骤 2: 创建一个引用该商品的订单
# golang-app2 会通过 HTTP 调用 golang-app 验证商品是否存在
ORDER_ID=$(curl -s -X POST http://localhost:8081/rest/v1/orders \
  -H "Content-Type: application/json" \
  -d "{\"note\": \"第一个订单\", \"items\": [{\"item_id\": \"$ITEM_ID\", \"quantity\": 2, \"price\": 9.99}]}" \
  | jq -r '.id')

echo "已创建订单: $ORDER_ID"
# golang-app 通过 NATS 收到 order.created 事件

# 步骤 3: 删除该商品
curl -s -X DELETE "http://localhost:8080/rest/v1/items/$ITEM_ID"

# golang-app 发布 item.deleted 事件到 NATS
# golang-app2 收到 item.deleted 事件后自动取消该待处理订单

# 步骤 4: 验证订单已被自动取消
curl -s "http://localhost:8081/rest/v1/orders/$ORDER_ID" | jq '.status'
# → "canceled"
```

---

## 8. 部署指南

### 8.1 构建 Docker 镜像

```bash
docker build -f golang-app/Dockerfile -t golang-app:latest .
docker build -f golang-app2/Dockerfile -t golang-app2:latest .
docker tag golang-app:latest ghcr.io/<组织>/golang-system/golang-app:latest
docker push ghcr.io/<组织>/golang-system/golang-app:latest
```

### 8.2 Kubernetes 部署

```bash
cd golang-system-chart
helm dependency update
helm install golang-system ./golang-system-chart --namespace golang-system --create-namespace
kubectl -n golang-system port-forward svc/golang-system-golang-app 8080:80
kubectl -n golang-system port-forward svc/golang-system-golang-app2 8081:80
```

### 8.3 生产环境检查清单

- [ ] 设置 `APP_MODE=PROD` 启用 JSON 结构化日志
- [ ] 为所有数据库设置独立的强密码
- [ ] 在 Helm values 中配置资源限制
- [ ] 启用 HPA 水平 Pod 自动伸缩
- [ ] 配置 NATS JetStream 持久化
- [ ] 配置网络策略 (NetworkPolicy)
- [ ] 搭建监控体系 (Prometheus + Grafana)
- [ ] 为 NATS 和数据库连接配置 TLS 加密
- [ ] 使用外部密钥管理 (HashiCorp Vault / K8s Secrets)
- [ ] 审查并配置生产域名的 CORS 设置

---

## 9. CI/CD 流水线

### 9.1 CI 流水线 (`.github/workflows/ci.yml`)

在每次 **push** 和 **Pull Request** 到 `main` / `develop` 分支时触发。

```text
[变更检测] → [代码检查 (matrix)] → [测试 (matrix)] → [构建 (matrix)] → [Docker (仅 main)]
```

| 阶段 | 行为 |
|------|------|
| 变更检测 | 使用 `dorny/paths-filter` 检测变更模块 |
| 代码检查 | 对每个变更的模块并行运行 `golangci-lint` |
| 测试 | `go test -race -coverprofile` |
| 构建 | 静态编译 + 上传二进制 |
| Docker | 仅 main 分支构建并推送多阶段 Docker 镜像 |

### 9.2 CD 流水线 (`.github/workflows/cd.yml`)

CI 通过且在 `main` 分支后触发，三环境逐级部署 + 审批。

```text
部署到 DEV (自动) → 部署到 STAG (需审批) → 部署到 PROD (需审批)
```

| Secret | 环境 |
|--------|------|
| `KUBE_CONFIG_DEV`     | development |
| `KUBE_CONFIG_STAGING` | staging     |
| `KUBE_CONFIG_PROD`    | production  |

| Environment | 配置 |
|-------------|------|
| `development` | 无限制 (自动部署) |
| `staging`     | 至少 1 名团队成员审批 |
| `production`  | 至少 2 名团队成员审批，限定 `main` 分支部署 |

### 9.3 Release 流水线 (`.github/workflows/release.yml`)

推送 `v*` 标签时触发。

```text
构建多架构二进制 → 创建 GitHub Release → 发布 Helm Chart (GHCR OCI)
```

**交叉编译**: `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`

### 9.4 Dependabot

- **Go 模块** - 每周检查，按模块独立更新
- **Docker** - 每周检查，更新基础镜像
- **GitHub Actions** - 每周检查，更新 Action 版本

---

## 10. 配置参考

### 10.1 golang-app 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_PORT`     | `8080`                    | HTTP 服务端口 |
| `APP_MODE`     | `DEV`              | 应用运行模式：`DEV`（控制台彩色日志）/ `PROD`（JSON 结构化日志） |
| `APP_NAME`     | `golang-app`              | 应用名称 |
| `LOG_LEVEL`    | `info`                    | `debug`、`info`、`warn`、`error` |
| `DB_HOST`      | `localhost`               | PostgreSQL 主机 |
| `DB_PORT`      | `5432`                    | PostgreSQL 端口 |
| `DB_USER`      | `app`                     | PostgreSQL 用户名 |
| `DB_NAME`      | `golang-app`              | 数据库名 |
| `DB_PASSWORD`  |                          | 密码 |
| `DB_SSLMODE`   | `disable`                 | SSL 模式 |
| `NATS_URL`     | `nats://localhost:4222`   | NATS 地址 |
| `QUEUE_GROUP`  | `golang-app-queue`        | NATS 队列组 |

### 10.2 golang-app2 环境变量

除上述通用变量外，还包括：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_PORT`         | `8081`                 | HTTP 服务端口 |
| `APP_NAME`         | `golang-app2`          | 应用名称 |
| `DB_NAME`          | `golang-app2`          | 数据库名 |
| `ITEM_SERVICE_URL` | `http://localhost:8080` | 商品服务 URL |
| `QUEUE_GROUP`      | `golang-app2-queue`    | NATS 队列组 |

---

## 11. 命令参考

| 命令 | 说明 |
|------|------|
| `make all`             | 代码检查 + 测试 + 构建 |
| `make build`           | 构建所有服务二进制到 `<服务>/server` |
| `make test`            | 带竞态检测的测试 |
| `make lint`            | `golangci-lint` 代码检查 |
| `make tidy`            | `go mod tidy` |
| `make docker-up`       | 启动基础设施：PostgreSQL x2 + NATS (Docker Compose) |
| `make docker-down`     | 停止并移除所有容器 |
| `make docker-logs`     | 实时查看基础设施服务日志 |
| `make migrate`         | 运行所有数据库迁移 |
| `make clean`           | 清理构建产物 |
| `make generate`        | 运行 `go generate` |
| `make helm-deps`       | 更新 Helm Chart 依赖 |
| `make helm-template`   | 本地渲染 Helm 模板 |
| `make helm-install`    | 部署到 Kubernetes |
| `make helm-uninstall`  | 从 Kubernetes 卸载 |
