# Istio 服务治理实施指南

本指南描述如何在 golang-system 项目中集成 Istio 服务网格，实现服务发现、熔断限流、API网关等基础治理能力。

## 目录

1. [架构概述](#1-架构概述)
2. [前置条件](#2-前置条件)
3. [Istio 安装](#3-istio-安装)
4. [服务发现配置](#4-服务发现配置)
5. [熔断限流配置](#5-熔断限流配置)
6. [API 网关配置](#6-api-网关配置)
7. [部署步骤](#7-部署步骤)
8. [验证测试](#8-验证测试)
9. [监控观测](#9-监控观测)

---

## 1. 架构概述

### 1.1 集成 Istio 后的架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Istio Ingress Gateway                      │
│                        (API 网关)                            │
└────────────────────┬────────────────────────────────────────┘
                     │ HTTPS/gRPC
        ┌────────────┴────────────┐
        ▼                         ▼
┌───────────────┐         ┌───────────────┐
│   golang-app   │ ◄─────► │   golang-app2  │
│   (Item Svc)   │         │   (Order Svc)  │
│  + Istio Sidecar│         │  + Istio Sidecar│
└───────┬───────┘         └───────┬───────┘
        │                         │
        └──────────┬──────────────┘
                   ▼
        ┌──────────────────┐
        │  Istio Control    │
        │  Plane (istiod)   │
        └──────────────────┘
                   │
        ┌──────────┴──────────┐
        ▼                     ▼
┌───────────────┐     ┌───────────────┐
│   PostgreSQL   │     │  NATS with     │
│                │     │  mTLS          │
└───────────────┘     └───────────────┘
```

### 1.2 治理能力提升

| 能力 | 实现方式 | 说明 |
|------|----------|------|
| **服务发现** | Istio Sidecar + Kubernetes DNS | 自动服务发现，无需修改代码 |
| **负载均衡** | Istio DestinationRule | 支持轮询、随机、最少连接等算法 |
| **熔断** | Istio Outlier Detection | 自动熔断故障实例 |
| **限流** | Istio Local Rate Limiting | 基于 Envoy 的本地限流 |
| **API 网关** | Istio Ingress Gateway | L7 路由、TLS 终止、认证 |
| **可观测性** | Istio Telemetry | 自动收集 metrics、logs、traces |
| **安全** | Istio mTLS | 服务间自动 mTLS 加密 |

---

## 2. 前置条件

### 2.1 Kubernetes 集群要求

- **Kubernetes 版本**: 1.24+ (rke2 默认提供)
- **rke2 已安装**: 集群正常运行
- **节点资源**: 每个节点至少 4GB RAM, 2 CPU

### 2.2 工具准备

```bash
# 安装 istioctl CLI
curl -L https://istio.io/downloadIstio | sh -
cd istio-* && sudo mv bin/istioctl /usr/local/bin/

# 验证安装
istioctl version
```

### 2.3 网络要求

- **LoadBalancer 支持**: 用于 Ingress Gateway 外部访问
- **CNI 兼容性**: rke2 使用 canal CNI，需要与 Istio CNI 兼容

---

## 3. Istio 安装

### 3.1 使用 istioctl 安装（推荐）

创建 Istio 配置文件 `istio-config.yaml`:

```yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: istio-control-plane
spec:
  profile: default
  
  # 组件配置
  components:
    # Ingress Gateway - API 网关
    ingressGateways:
    - name: istio-ingressgateway
      enabled: true
      k8s:
        service:
          type: LoadBalancer
          ports:
          - port: 80
            targetPort: 8080
            name: http
          - port: 443
            targetPort: 8443
            name: https
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
    
    # Egress Gateway (可选)
    egressGateways:
    - name: istio-egressgateway
      enabled: false
  
  # 全局配置
  values:
    # 全局 mTLS 模式
    global:
      mtls:
        enabled: true
      
      # 网络配置
      network: ""
    
    # Pilot 配置
    pilot:
      traceSampling: 1.0
      
    # Sidecar 注入配置
    sidecarInjectorWebhook:
      injectedAnnotations:
        traffic.sidecar.istio.io/includeInboundPorts: "*"
        traffic.sidecar.istio.io/includeOutboundIPRanges: "*"
    
    # 遥测配置
    telemetry:
      enabled: true
      v2:
        enabled: true
    
    # CNI 配置 (rke2 兼容)
    cni:
      enabled: false  # rke2 使用 canal CNI，暂不启用 Istio CNI
    
    # 网关配置
    gateways:
      istio-ingressgateway:
        autoscaleEnabled: true
        autoscaleMin: 1
        autoscaleMax: 5
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
```

安装 Istio:

```bash
# 安装 Istio
istioctl install -f istio-config.yaml -y

# 验证安装
kubectl get pods -n istio-system
kubectl get svc -n istio-system

# 检查 Ingress Gateway 外部 IP
kubectl get svc istio-ingressgateway -n istio-system
```

### 3.2 启用 Sidecar 自动注入

```bash
# 为 default namespace 启用自动注入
kubectl label namespace default istio-injection=enabled

# 验证
kubectl get namespace -L istio-injection
```

---

## 4. 服务发现配置

### 4.1 原理说明

Istio 通过以下机制实现服务发现：

1. **Kubernetes DNS**: Kubernetes 原生的服务发现
2. **Istio Sidecar**: 每个 Pod 注入 Envoy sidecar 代理
3. **服务注册**: 服务自动注册到 Istio 服务注册表
4. **服务发现**: 通过 Envoy 代理进行服务间通信

### 4.2 配置步骤

**无需修改代码**，只需确保：

1. Kubernetes Service 配置正确
2. Pod 标签正确
3. 启用 Sidecar 自动注入

**验证服务发现**:

```bash
# 进入 golang-app2 pod
kubectl exec -it deployment/golang-app2 -- /bin/sh

# 测试服务发现（通过 Kubernetes DNS）
curl http://golang-app:8080/rest/v1/items

# 查看 Istio 服务注册表
kubectl exec -it deployment/golang-app -c istio-proxy -- curl localhost:15000/config_dump
```

### 4.3 服务条目（ServiceEntry）

如果需要访问集群外部服务，创建 ServiceEntry:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: ServiceEntry
metadata:
  name: external-postgres
spec:
  hosts:
  - external-postgres.example.com
  ports:
  - number: 5432
    name: tcp-postgresql
    protocol: TCP
  location: MESH_EXTERNAL
  resolution: DNS
```

---

## 5. 熔断限流配置

### 5.1 熔断配置（Circuit Breaking）

创建 `destination-rule-circuit-breaking.yaml`:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: golang-app-circuit-breaking
  namespace: default
spec:
  host: golang-app
  trafficPolicy:
    # 连接池配置
    connectionPool:
      tcp:
        maxConnections: 100
        connectTimeout: 30s
        keepAlive:
          time: 7200s
          interval: 75s
      http:
        http1MaxPendingRequests: 50
        http2MaxRequests: 100
        maxRequestsPerConnection: 10
        maxRetries: 3
        consecutiveGatewayErrors: 5
        interval: 30s
        baseEjectionTime: 30s
    
    # 异常检测（熔断）
    outlierDetection:
      consecutiveGatewayErrors: 5      # 连续 5 个网关错误
      consecutive5xxErrors: 5           # 连续 5 个 5xx 错误
      interval: 30s                     # 检查间隔
      baseEjectionTime: 30s             # 熔断基础时间
      maxEjectionPercent: 50            # 最大熔断实例百分比
      minHealthPercent: 30              # 最小健康实例百分比
  
  subsets:
  - name: v1
    labels:
      version: v1
---
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: golang-app2-circuit-breaking
  namespace: default
spec:
  host: golang-app2
  trafficPolicy:
    connectionPool:
      tcp:
        maxConnections: 100
        connectTimeout: 30s
      http:
        http1MaxPendingRequests: 50
        http2MaxRequests: 100
        maxRequestsPerConnection: 10
        maxRetries: 3
    
    outlierDetection:
      consecutiveGatewayErrors: 5
      consecutive5xxErrors: 5
      interval: 30s
      baseEjectionTime: 30s
      maxEjectionPercent: 50
      minHealthPercent: 30
  
  subsets:
  - name: v1
    labels:
      version: v1
```

### 5.2 限流配置（Rate Limiting）

**方式 1: 使用 EnvoyFilter (本地限流)**

创建 `envoy-filter-rate-limiting.yaml`:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: golang-app-rate-limiting
  namespace: default
spec:
  workloadSelector:
    labels:
      app: golang-app
  configPatches:
  - applyTo: HTTP_FILTER
    match:
      context: SIDECAR_INBOUND
      listener:
        filterChain:
          filter:
            name: "envoy.filters.network.http_connection_manager"
    patch:
      operation: INSERT_BEFORE
      value:
        name: envoy.filters.http.local_ratelimit
        typed_config:
          "@type": type.googleapis.com/udpa.type.v1.TypedStruct
          type_url: type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
          value:
            stat_prefix: local_rate_limit
            token_bucket:
              max_tokens: 100        # 令牌桶容量
              tokens_per_fill: 10    # 每次补充令牌数
              fill_interval: 1s      # 补充间隔
            filter_enabled:
              runtime_key: local_rate_limit_enabled
              default_value: true
            filter_enforced:
              runtime_key: local_rate_limit_enforced
              default_value: true
            response_headers_to_add:
            - header:
                key: x-rate-limit-limit
                value: "100"
            - header:
                key: x-rate-limit-remaining
                value: "90"
            - header:
                key: x-rate-limit-reset
                value: "1"
```

**方式 2: 使用 VirtualService (更简单)**

创建 `virtual-service-rate-limiting.yaml`:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: golang-app-rate-limit
spec:
  hosts:
  - golang-app
  http:
  - match:
    - headers:
        x-user-id:
          regex: ".*"
    route:
    - destination:
        host: golang-app
        subset: v1
    # 超时配置
    timeout: 10s
    # 重试配置
    retries:
      attempts: 3
      perTryTimeout: 3s
      retryOn: 5xx,connect-failure,refused-stream
```

### 5.3 部署熔断限流配置

```bash
# 应用熔断配置
kubectl apply -f destination-rule-circuit-breaking.yaml

# 应用限流配置
kubectl apply -f envoy-filter-rate-limiting.yaml

# 验证
kubectl get destinationrule
kubectl get envoyfilter
```

### 5.4 测试熔断

```bash
# 进入 fortio pod (负载测试工具)
kubectl run fortio --image=fortio/fortio -- restart=Never -- sleep 3600

# 发送大量请求触发熔断
kubectl exec fortio -- /usr/bin/fortio load -curl -qps 100 -n 1000 http://golang-app:8080/rest/v1/items

# 查看熔断状态
kubectl exec -it deployment/golang-app -c istio-proxy -- curl localhost:15000/stats | grep outlier
```

---

## 6. API 网关配置

### 6.1 Ingress Gateway 配置

创建 `gateway.yaml`:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: golang-system-gateway
  namespace: default
spec:
  selector:
    istio: ingressgateway
  servers:
  # HTTP 配置
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
    - "api.example.com"
    - "*.example.com"
    tls:
      httpsRedirect: true  # 强制跳转 HTTPS
  
  # HTTPS 配置
  - port:
      number: 443
      name: https
      protocol: HTTPS
    hosts:
    - "api.example.com"
    - "*.example.com"
    tls:
      mode: SIMPLE
      credentialName: golang-system-tls  # Kubernetes Secret 名称
```

### 6.2 VirtualService 配置（路由规则）

创建 `virtual-service-gateway.yaml`:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: golang-system-routes
  namespace: default
spec:
  hosts:
  - "api.example.com"
  gateways:
  - golang-system-gateway
  http:
  # 路由 1: Item 服务
  - match:
    - uri:
        prefix: /rest/v1/items
    route:
    - destination:
        host: golang-app
        port:
          number: 8080
        subset: v1
    # CORS 配置
    corsPolicy:
      allowOrigins:
      - regex: "https://.*\\.example\\.com"
      allowMethods:
      - GET
      - POST
      - PUT
      - DELETE
      - PATCH
      - OPTIONS
      allowHeaders:
      - Content-Type
      - X-Request-ID
      - X-Organization-ID
      - X-User-ID
      - Authorization
      maxAge: 24h
    
    # 超时配置
    timeout: 30s
    
    # 重试配置
    retries:
      attempts: 3
      perTryTimeout: 10s
      retryOn: 5xx,connect-failure,gateway-error,envoy-ratelimited
  
  # 路由 2: Order 服务
  - match:
    - uri:
        prefix: /rest/v1/orders
    route:
    - destination:
        host: golang-app2
        port:
          number: 8081
        subset: v1
    corsPolicy:
      allowOrigins:
      - regex: "https://.*\\.example\\.com"
      allowMethods:
      - GET
      - POST
      - PUT
      - DELETE
      - PATCH
      - OPTIONS
      allowHeaders:
      - Content-Type
      - X-Request-ID
      - X-Organization-ID
      - X-User-ID
      - Authorization
      maxAge: 24h
    timeout: 30s
    retries:
      attempts: 3
      perTryTimeout: 10s
  
  # 路由 3: 健康检查
  - match:
    - uri:
        prefix: /admin/health
    route:
    - destination:
        host: golang-app
        port:
          number: 8080
  
  # 默认路由（404）
  - route:
    - destination:
        host: golang-app
        port:
          number: 8080
```

### 6.3 TLS 证书配置

创建 TLS Secret:

```bash
# 方式 1: 使用 cert-manager (推荐)
kubectl apply -f cert-manager.yaml

# 方式 2: 手动创建 Secret
kubectl create secret tls golang-system-tls \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key \
  -n default
```

创建 `cert-manager.yaml` (使用 Let's Encrypt):

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: golang-system-tls
  namespace: default
spec:
  secretName: golang-system-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
  - api.example.com
  - "*.example.com"
```

### 6.4 部署 API 网关

```bash
# 应用 Gateway 配置
kubectl apply -f gateway.yaml

# 应用 VirtualService 路由配置
kubectl apply -f virtual-service-gateway.yaml

# 验证
kubectl get gateway
kubectl get virtualservice

# 获取 Ingress Gateway 外部 IP
kubectl get svc istio-ingressgateway -n istio-system
```

### 6.5 测试 API 网关

```bash
# 获取外部 IP
export INGRESS_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# 测试 HTTP 路由（自动跳转 HTTPS）
curl -v http://$INGRESS_IP/rest/v1/items -H "Host: api.example.com"

# 测试 HTTPS 路由
curl -v https://api.example.com/rest/v1/items

# 测试 CORS
curl -v https://api.example.com/rest/v1/items \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: GET"
```

---

## 7. 部署步骤

### 7.1 完整部署流程

**步骤 1: 安装 Istio**

```bash
# 1. 安装 istioctl
curl -L https://istio.io/downloadIstio | sh -
cd istio-* && sudo mv bin/istioctl /usr/local/bin/

# 2. 安装 Istio (使用配置文件)
istioctl install -f istio-config.yaml -y

# 3. 启用 namespace 自动注入
kubectl label namespace default istio-injection=enabled
```

**步骤 2: 部署微服务**

```bash
# 1. 更新 Helm values (启用 Istio)
cd golang-system-chart

# 2. 部署基础设施 (PostgreSQL, NATS)
helm dependency update
helm install golang-system . \
  --namespace default \
  --create-namespace \
  --values values.yaml

# 3. 等待 Pod 就绪 (包含 Istio sidecar)
kubectl rollout status deployment/golang-app
kubectl rollout status deployment/golang-app2

# 4. 验证 sidecar 注入
kubectl get pods
kubectl describe pod <golang-app-pod> | grep -A 10 "istio-proxy"
```

**步骤 3: 配置服务治理**

```bash
# 1. 应用熔断配置
kubectl apply -f destination-rule-circuit-breaking.yaml

# 2. 应用限流配置
kubectl apply -f envoy-filter-rate-limiting.yaml

# 3. 配置 API 网关
kubectl apply -f gateway.yaml
kubectl apply -f virtual-service-gateway.yaml

# 4. 配置 TLS (如果使用 cert-manager)
kubectl apply -f cert-manager.yaml
```

**步骤 4: 验证部署**

```bash
# 1. 检查 Pod 状态
kubectl get pods -o wide

# 2. 检查 Service
kubectl get svc

# 3. 检查 Istio 配置
kubectl get gateway,virtualservice,destinationrule

# 4. 查看 Proxy 配置
istioctl proxy-config cluster deployment/golang-app
```

### 7.2 Helm Chart 更新

更新 `golang-system-chart/values.yaml` 以支持 Istio:

```yaml
# 添加 Istio 相关配置
istio:
  # 启用 Istio
  enabled: true
  
  # 自动注入
  injection:
    enabled: true
    namespace: default
  
  # Gateway 配置
  gateway:
    enabled: true
    hosts:
    - "api.example.com"
    tls:
      enabled: true
      secretName: golang-system-tls
  
  # 熔断配置
  circuitBreaking:
    enabled: true
    maxConnections: 100
    maxRequestsPerConnection: 10
    consecutiveErrors: 5
    interval: 30s
    baseEjectionTime: 30s
  
  # 限流配置
  rateLimiting:
    enabled: true
    maxTokens: 100
    tokensPerFill: 10
    fillInterval: 1s

# 更新服务配置
golangApp:
  # ... 原有配置 ...
  
  # 添加 Istio 支持
  istio:
    enabled: true
    port: 8080
    service:
      type: ClusterIP  # Istio 使用 ClusterIP
  
  # 添加 Pod 标签 (用于 Istio 路由)
  podLabels:
    version: v1
    app: golang-app

golangApp2:
  # ... 原有配置 ...
  
  istio:
    enabled: true
    port: 8081
    service:
      type: ClusterIP
  
  podLabels:
    version: v1
    app: golang-app2
```

---

## 8. 验证测试

### 8.1 服务发现测试

```bash
# 1. 进入 golang-app2 pod
kubectl exec -it deployment/golang-app2 -- /bin/sh

# 2. 测试 DNS 解析
nslookup golang-app

# 3. 测试服务间调用
curl http://golang-app:8080/rest/v1/items

# 4. 查看 Istio proxy 日志
kubectl logs deployment/golang-app2 -c istio-proxy
```

### 8.2 熔断测试

```bash
# 1. 部署 fortio (负载测试工具)
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/sample-client/fortio-deploy.yaml

# 2. 发送大量请求
kubectl exec fortio -- fortio load -qps 100 -n 1000 -loglevel Debug http://golang-app:8080/rest/v1/items

# 3. 查看熔断状态
kubectl exec deployment/golang-app -c istio-proxy -- curl localhost:15000/stats | grep outlier

# 4. 查看被熔断的实例
kubectl get pods -o wide
istioctl proxy-config endpoint deployment/golang-app
```

### 8.3 限流测试

```bash
# 1. 发送超过限量的请求
kubectl exec fortio -- fortio load -qps 200 -n 200 http://golang-app:8080/rest/v1/items

# 2. 检查是否返回 429 (Too Many Requests)
kubectl logs deployment/golang-app -c istio-proxy | grep "local_rate_limit"

# 3. 查看限流统计
kubectl exec deployment/golang-app -c istio-proxy -- curl localhost:15000/stats | grep rate_limit
```

### 8.4 API 网关测试

```bash
# 1. 获取 Ingress Gateway IP
export INGRESS_HOST=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export INGRESS_PORT=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.spec.ports[?(@.name=="http")].port}')

# 2. 测试路由
curl -v http://$INGRESS_HOST:$INGRESS_PORT/rest/v1/items -H "Host: api.example.com"

# 3. 测试 HTTPS 跳转
curl -v http://$INGRESS_HOST:$INGRESS_PORT/rest/v1/items -H "Host: api.example.com" -L

# 4. 测试 CORS
curl -v http://$INGRESS_HOST:$INGRESS_PORT/rest/v1/items \
  -H "Host: api.example.com" \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: GET" \
  -X OPTIONS
```

---

## 9. 监控观测

### 9.1 安装 Kiali (服务拓扑可视化)

```bash
# 安装 Kiali
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/kiali.yaml

# 访问 Kiali
istioctl dashboard kiali
```

### 9.2 安装 Prometheus (指标收集)

```bash
# 安装 Prometheus
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/prometheus.yaml

# 访问 Prometheus
istioctl dashboard prometheus
```

### 9.3 安装 Grafana (指标可视化)

```bash
# 安装 Grafana
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/grafana.yaml

# 访问 Grafana
istioctl dashboard grafana
```

### 9.4 安装 Jaeger (分布式追踪)

```bash
# 安装 Jaeger
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/jaeger.yaml

# 访问 Jaeger
istioctl dashboard jaeger
```

### 9.5 查看遥测数据

```bash
# 1. 查看 Proxy 配置
istioctl proxy-config cluster deployment/golang-app
istioctl proxy-config listener deployment/golang-app
istioctl proxy-config route deployment/golang-app

# 2. 查看遥测数据
kubectl logs deployment/golang-app -c istio-proxy | tail -100

# 3. 查看 metrics
kubectl exec deployment/golang-app -c istio-proxy -- curl localhost:15000/stats | grep http

# 4. 访问 Kiali 查看服务拓扑
# 浏览器打开: http://localhost:20001
```

---

## 10. 清理

```bash
# 1. 删除 Istio 配置
kubectl delete -f gateway.yaml
kubectl delete -f virtual-service-gateway.yaml
kubectl delete -f destination-rule-circuit-breaking.yaml
kubectl delete -f envoy-filter-rate-limiting.yaml

# 2. 卸载 Istio
istioctl uninstall -y

# 3. 删除 Istio namespace
kubectl delete namespace istio-system

# 4. 删除 cert-manager (如果安装)
kubectl delete -f cert-manager.yaml
```

---

## 11. 参考资料

- [Istio 官方文档](https://istio.io/latest/docs/)
- [Istio 流量管理](https://istio.io/latest/docs/concepts/traffic-management/)
- [Istio 安全](https://istio.io/latest/docs/concepts/security/)
- [Istio 可观测性](https://istio.io/latest/docs/concepts/observability/)
- [rke2 文档](https://docs.rke2.io/)
- [Envoy 文档](https://www.envoyproxy.io/docs/envoy/latest/)

---

## 附录 A: 完整配置文件

所有配置文件位于 `istio-config/` 目录:

```
istio-config/
├── istio-config.yaml                      # Istio 安装配置
├── destination-rule-circuit-breaking.yaml # 熔断配置
├── envoy-filter-rate-limiting.yaml        # 限流配置
├── gateway.yaml                           # API 网关配置
├── virtual-service-gateway.yaml           # 路由规则
├── cert-manager.yaml                      # TLS 证书管理
└── samples/                               # 测试样本
    ├── test-service-discovery.sh
    ├── test-circuit-breaking.sh
    └── test-rate-limiting.sh
```

---

**文档版本**: 1.0  
**更新日期**: 2026-01-15  
**作者**: golang-system 项目
