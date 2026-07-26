# golang-system 整体技术架构

> 微服务架构 + RKE2 + Istio Service Mesh 治理层
>
> 更新日期: 2026-07-26

## 整体架构图

```
                         ┌──────────────────────────────────────────────┐
                         │              客户端 / 前端应用                 │
                         └──────────────────┬───────────────────────────┘
                                            │ HTTPS (TLS 终止, cert-manager 签发)
                                            ▼
┌────────────────────────── RKE2 Kubernetes 集群 ──────────────────────────────────┐
│                                                                                  │
│  ┌────────────────────────── istio-system ──────────────────────────┐            │
│  │  istiod (控制平面: 服务发现/配置下发/证书签发)                      │            │
│  │  istio-ingressgateway (南北向入口, HPA 1~5)    ◄── LoadBalancer  │            │
│  │  Kiali │ Prometheus │ Grafana │ Jaeger  (可观测性套件)            │            │
│  └──────────────────────────────────────────────────────────────────┘            │
│                                                                                  │
│  ┌────────────────────────── default 命名空间 (istio-injection=enabled) ────────┐│
│  │                                                                              ││
│  │   ┌───────────────────────┐          ┌───────────────────────┐               ││
│  │   │ golang-app (Item 服务) │          │ golang-app2 (Order服务)│              ││
│  │   │ ┌─────┐  ┌──────────┐ │  HTTP    │ ┌─────┐  ┌──────────┐ │               ││
│  │   │ │envoy│─►│ app:8080 │ │◄─────────┼─│envoy│─►│ app:8081 │ │               ││
│  │   │ │proxy│  │ 六角架构   │ │ item_client│proxy│  │ 六角架构   │ │               ││
│  │   │ └─────┘  └────┬─────┘ │  同步调用  │ └─────┘  └────┬─────┘ │               ││
│  │   │  限流/熔断/mTLS │       │          │  限流/熔断/mTLS  │       │               ││
│  │   └───────────────┼───────┘          └───────────────┼───────┘               ││
│  │                   │                                  │                       ││
│  │              ┌────▼─────┐                       ┌────▼─────┐                 ││
│  │              │postgres- │                       │postgres- │                 ││
│  │              │app :5432 │                       │app2 :5432│                 ││
│  │              │ (app_db) │                       │(app2_db) │                 ││
│  │              └────┬─────┘                       └────┬─────┘                 ││
│  │                   │ outbox                           │ outbox                ││
│  │                   └──────────►  ┌──────────┐  ◄──────┘                       ││
│  │              item.*/order.*     │   NATS   │                                 ││
│  │                    pub ────────►│JetStream │◄──────── sub (Ack/幂等)          ││
│  │                                 │  :4222   │                                 ││
│  │                                 └──────────┘                                 ││
│  └──────────────────────────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────────────────────────┘
```

## 分层说明

### 1. 基础设施层 — RKE2

- RKE2 作为 Kubernetes 发行版（默认 Canal CNI，`istio-config.yaml` 中 `cni.enabled: false`）。
- 工作负载用 Helm Chart (`golang-system-chart`) 部署：Service 名固定为 `golang-app` / `golang-app2`，Pod 标签含 `version: v1`，HPA 2~10（CPU 80%）。
- 密码经 K8s Secret 注入（`DB_PASSWORD`）；nginx Ingress 默认关闭，南北向走 Istio Gateway。

### 2. 服务治理层 — Istio Service Mesh

| 治理能力 | 实现方式 | 对应配置 |
|---|---|---|
| 入口网关 | `Gateway` + `istio-ingressgateway`；TLS Secret 在 `istio-system` | `gateway.yaml` |
| 路由 | `VirtualService`：GET 可重试，写方法 `retries.attempts: 0` | `virtual-service-gateway.yaml` |
| 熔断+LB | **每 host 单一** `DestinationRule`（LEAST_REQUEST + outlierDetection + subsets） | `destination-rule-circuit-breaking.yaml` |
| mTLS | `PeerAuthentication` STRICT（default ns）+ DR `ISTIO_MUTUAL` | `peer-authentication.yaml` |
| 限流（本地） | Sidecar `LocalRateLimit`（RuntimeFractionalPercent） | `envoy-filter-rate-limiting.yaml` |
| 限流（全局） | IngressGateway → Redis + `envoyproxy/ratelimit` | `global-rate-limiting.yaml` |
| JWT 鉴权 | Gateway `RequestAuthentication` + `AuthorizationPolicy`；claim → `x-jwt-*` | `jwt-authentication.yaml` |
| meshConfig | `spec.meshConfig`（access log / tracing） | `istio-config.yaml` |

### 3. 业务服务层 — 六角架构微服务

- **golang-app** (Item，:8080)：REST `/rest/v1/items`；领域写操作与 outbox 同事务；JetStream 发布 `item.*`。
- **golang-app2** (Order，:8081)：REST `/rest/v1/orders`；`item_client` 同步查 Item（透传 `X-Request-ID` 等，GET 有限重试，错误按状态码映射）；`item.deleted` 按 DB `order_items` 查 pending 订单自动取消（无进程内 map）。
- 共用：`/metrics`、CORS（按 Origin 白名单）、HTTP Server 超时、`/admin/health/liveness|readiness`（readiness 含 DB + NATS）。
- 鉴权：网关强制 JWT；应用优先读 `x-jwt-sub/org`，PROD 默认 `AUTH_TRUST_HEADERS=false` + `AUTH_REQUIRE_JWT=true`；东西向由 `httpclient` 透传 claim / Authorization。

### 4. 数据与消息层

- **Database-per-Service**：PostgreSQL × 2（`app_db` / `app2_db`）。
- **Transactional Outbox**：写业务表与 `outbox` 同事务；后台 Relay 投递到 **NATS JetStream**；消费端 `processed_events` 幂等 + Manual Ack。
- **事件契约** `golang-events`：`Event[T]` 含 `version`（当前 `"1"`）+ subject 常量。

### 5. 共享代码层 (Go workspace)

- `golang-events`：事件契约
- `golang-commons`：logger、httputil、messaging（JetStream）、outbox、database（含事务）、httpclient、apperror

### 6. 可观测性层

- Sidecar 指标 + 应用 `/metrics` → Prometheus → Grafana
- Kiali / Jaeger（`PILOT_TRACE_SAMPLING: 1.0` = **1%**）
- zerolog（DEV console / PROD JSON）+ Envoy access log

## 关键流量链路

### 南北向

```
Client → IngressGateway:443 (TLS + JWT 校验 + 全局限流)
       → VirtualService → sidecar (本地限流→熔断→mTLS) → 业务容器
```

### 东西向

- **同步**：`app2 → item_client → golang-app:8080`（mTLS + LEAST_REQUEST）
- **异步**：业务写 → outbox → JetStream → 对端 durable consumer（Ack/幂等）

## 部署与运维

```bash
./deploy-istio.sh install   # Istio + 微服务 + 治理 + 可观测性
./deploy-istio.sh verify
./deploy-istio.sh test
./deploy-istio.sh cleanup
```

本地依赖：`docker-compose.yml`（PostgreSQL × 2 + NATS JetStream）。

应用环境变量（与 Chart 对齐）：`APP_PORT`、`DB_*`、`NATS_URL`、`QUEUE_GROUP`、`ITEM_SERVICE_URL`（仅 app2）、`AUTH_TRUST_HEADERS`、`AUTH_REQUIRE_JWT`。

联调 JWT：见 `istio-config/jwt/README.md`（`mint-dev-jwt.py`）。生产请把 `jwt-authentication.yaml` 中的 inline JWKS 换成真实 IdP 的 `jwksUri`。

## 已知限制

1. Postgres/NATS/Redis/ratelimit 为 chart/清单内单副本有状态部署，生产建议托管服务或 Operator。
2. Dev JWT 私钥仅供联调，切勿用于生产。

## 目录结构速查

| 路径 | 说明 |
|---|---|
| `golang-app/` | Item 服务 |
| `golang-app2/` | Order 服务 |
| `golang-events/` | 事件契约 |
| `golang-commons/` | 共享库 |
| `golang-system-chart/` | 统一 Helm Chart |
| `istio-config/` | Istio 治理配置（含 JWT / 全局限流） |
| `istio-config/jwt/` | Dev JWKS + 签发脚本 |
| `deploy-istio.sh` | 一键部署脚本 |
| `docker-compose.yml` | 本地依赖 |
