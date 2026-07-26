# golang-system 整体技术架构

> 微服务架构 + RKE2 + Istio Service Mesh 治理层
>
> 生成日期: 2026-07-22

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
│  │              └──────────┘                       └──────────┘                 ││
│  │                   │                                  │                       ││
│  │                   └──────────►  ┌──────────┐  ◄──────┘                       ││
│  │              item.created/...   │   NATS   │  order.created/...              ││
│  │                    pub ────────►│JetStream │◄──────── sub                    ││
│  │                                 │  :4222   │                                 ││
│  │                                 └──────────┘                                 ││
│  └──────────────────────────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────────────────────────┘
```

## 分层说明

### 1. 基础设施层 — RKE2

- RKE2 作为 Kubernetes 发行版（默认 Canal CNI，所以 `istio-config.yaml` 中 `cni.enabled: false`，流量劫持走 sidecar initContainer iptables 方案）。
- 工作负载用 Helm Chart (`golang-system-chart`) 统一部署，两个服务均配 HPA（2~10 副本，CPU 80%）。

### 2. 服务治理层 — Istio Service Mesh

治理核心，全部配置在 `istio-config/`：

| 治理能力 | 实现方式 | 对应配置 |
|---|---|---|
| 入口网关 | `Gateway` + `istio-ingressgateway` (LoadBalancer) | `gateway.yaml` |
| 路由/灰度 | `VirtualService`（路径前缀路由、subset v1/v2、weight、超时 30s、重试 3 次、故障注入、默认 404） | `virtual-service-gateway.yaml` |
| 熔断 | `DestinationRule` outlierDetection（连续 5 次 5xx/网关错误，熔断 30s，最大驱逐 50%） | `destination-rule-circuit-breaking.yaml` |
| 连接池 | TCP 100 连接 / HTTP2 100 并发 / 每连接 10 请求 | 同上 |
| 负载均衡 | `LEAST_CONN`（v1 subset 用 ROUND_ROBIN） | 同上 |
| 限流 | `EnvoyFilter` 本地令牌桶（app: 100 桶/10QPS 补充；app2: 50/5QPS，返回 429）；预留 Redis + ratelimit 全局限流 | `envoy-filter-rate-limiting.yaml` |
| mTLS | 全局开启（`global.mtls.enabled: true`），sidecar 间自动双向 TLS | `istio-config.yaml` |
| TLS 证书 | cert-manager + Let's Encrypt（HTTP-01/DNS-01），Gateway 443 端口 SIMPLE 模式，80 强制跳 HTTPS | `gateway.yaml` |
| CORS | 网关层 VirtualService corsPolicy + 应用内 `CORSMiddleware` 双保险 | 同上 |

### 3. 业务服务层 — 六角架构微服务

两个服务同构，均为 `domain → service → adapter (http handler / repository / messaging)`：

- **golang-app** (Item 服务，:8080)：REST `/rest/v1/items`，发布 `item.created/updated/deleted` 事件，提供 `item.get_by_id` request-reply。
- **golang-app2** (Order 服务，:8081)：REST `/rest/v1/orders`，通过 `item_client` (HTTP) 同步查 Item，通过 NATS 订阅/发布订单事件。
- 服务内自带治理能力作为 Mesh 的补充：`/metrics` Prometheus 端点、CORS 中间件、TLS 启动支持、`migrate` CLI 做数据库迁移。

### 4. 数据与消息层

- **每服务独立数据库**：PostgreSQL 16 × 2（`app_db` / `app2_db`），符合 Database-per-Service。
- **NATS JetStream** 做事件总线：异步事件不阻塞主流程，事件契约集中在 `golang-events`（泛型 `Event[T]` 信封 + subject 常量）。

### 5. 共享代码层 (Go workspace)

- `golang-events`：事件契约（subject + payload）。
- `golang-commons`：`logger`(zerolog)、`httputil`(metrics/CORS 中间件)、`messaging`、`database`、`httpclient`、`apperror`。

### 6. 可观测性层

- **指标**：Sidecar 指标 + 应用 `/metrics` → Prometheus → Grafana。
- **拓扑**：Kiali 实时服务依赖图。
- **追踪**：Jaeger（`PILOT_TRACE_SAMPLING: 1.0`，采样 1%，生产建议调高）。
- **日志**：应用 zerolog（DEV console / PROD JSON）+ Envoy access log JSON 输出到 stdout。

## 关键流量链路

### 南北向（外部进入）

```
Client → IngressGateway:443 (TLS 终止) → VirtualService 按路径路由
       → 目标 Pod sidecar (限流→熔断→mTLS) → 业务容器
```

### 东西向

- **同步**：`app2 → item_client (HTTP) → golang-app:8080`，全程 sidecar mTLS + LEAST_CONN 负载均衡。
- **异步**：`app 发布 item.* → NATS → app2 消费`，NATS 流量默认不走 sidecar HTTP 治理（TCP 级别仍有 mTLS）。

## 部署与运维

`deploy-istio.sh` 一条命令完成全流程：

```bash
./deploy-istio.sh install   # 完整安装 (Istio + 微服务 + 治理 + 可观测性)
./deploy-istio.sh verify    # 验证部署
./deploy-istio.sh test      # 运行测试 (fortio 压测熔断/限流)
./deploy-istio.sh cleanup   # 清理环境
```

分阶段执行：`istio` → `microservices` → `governance` → `observability`。

本地开发依赖（PostgreSQL × 2 + NATS）由 `docker-compose.yml` 提供。

## 已知问题与建议

1. **双入口冲突**：`golang-system-chart/values.yaml` 里 `ingress.enabled: true, className: nginx`，与 Istio Gateway 是两套入口。建议生产环境只用 Istio Gateway，把 chart 里的 `ingress.enabled` 置为 `false`，避免流量绕过 Mesh 治理。
2. **NATS 出站端口排除**：`istio-config.yaml` 中 sidecar 注入注解 `excludeOutboundPorts: "443"` 会排除 443 出站劫持，如果后续有服务通过 443 访问外部 API 需要注意这会绕过 Mesh。
3. **追踪采样率**：`PILOT_TRACE_SAMPLING: 1.0`（1%）适合联调，生产环境建议按流量调整。

## 目录结构速查

| 路径 | 说明 |
|---|---|
| `golang-app/` | Item 服务 (六角架构, :8080) |
| `golang-app2/` | Order 服务 (六角架构, :8081) |
| `golang-events/` | 事件契约 (subject + payload) |
| `golang-commons/` | 共享工具 (logger/httputil/messaging/database/httpclient/apperror) |
| `golang-system-chart/` | 统一 Helm Chart |
| `istio-config/` | Istio 治理配置 + 实施文档 |
| `deploy-istio.sh` | 一键部署/验证/测试/清理脚本 |
| `docker-compose.yml` | 本地开发依赖 (PostgreSQL × 2, NATS) |
