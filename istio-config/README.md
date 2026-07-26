# Istio 服务治理配置

本目录包含基于 **rke2 + Istio** 的微服务治理配置文件，实现服务发现、熔断限流、API网关等基础治理能力。

## 目录结构

```
istio-config/
├── README.md                                    # 本文件
├── ISTIO_IMPLEMENTATION.md                      # 完整实施指南 (1000+ 行)
├── istio-config.yaml                            # Istio 安装配置
├── destination-rule-circuit-breaking.yaml       # 熔断配置
├── envoy-filter-rate-limiting.yaml              # 限流配置
├── gateway.yaml                                 # API 网关配置
└── virtual-service-gateway.yaml                 # 路由规则配置
```

## 快速开始

### 1. 完整安装 (推荐)

使用自动化部署脚本:

```bash
# 添加执行权限
chmod +x deploy-istio.sh

# 完整安装 (Istio + 微服务 + 服务治理 + 可观测性)
./deploy-istio.sh install
```

或使用 Make 命令:

```bash
# 完整安装
make istio-all
```

### 2. 分步安装

```bash
# 1. 安装 Istio
make istio-install

# 2. 部署微服务
make istio-deploy

# 3. 配置服务治理
make istio-governance

# 4. 验证部署
make istio-verify
```

## 配置说明

### 服务发现

Istio 通过以下机制自动实现服务发现，无需修改代码:

- ✅ Kubernetes DNS 服务发现
- ✅ Istio Sidecar 代理注入
- ✅ 服务自动注册到 Istio 服务注册表

**验证服务发现:**

```bash
# 进入 golang-app2 pod
kubectl exec -it deployment/golang-app2 -- /bin/sh

# 测试 DNS 解析和服务间调用
curl http://golang-app:8080/rest/v1/items
```

### 熔断 (Circuit Breaking)

配置文件: `destination-rule-circuit-breaking.yaml`

**特性:**

- ✅ 连接池限制 (最大连接数、请求队列)
- ✅ 异常检测 (连续错误熔断)
- ✅ 自动恢复 (基础熔断时间)
- ✅ 熔断百分比限制 (防止全部熔断)

**关键配置:**

```yaml
outlierDetection:
  consecutive5xxErrors: 5        # 连续 5 个 5xx 错误触发熔断
  interval: 30s                   # 检测间隔
  baseEjectionTime: 30s          # 熔断基础时间
  maxEjectionPercent: 50         # 最大熔断实例百分比
```

**测试熔断:**

```bash
# 部署 fortio (负载测试工具)
kubectl run fortio --image=fortio/fortio -- sleep 3600

# 发送大量请求触发熔断
kubectl exec fortio -- fortio load -qps 100 -n 1000 http://golang-app:8080/rest/v1/items

# 查看熔断状态
kubectl exec deployment/golang-app -c istio-proxy -- curl localhost:15000/stats | grep outlier
```

### 限流 (Rate Limiting)

配置文件: `envoy-filter-rate-limiting.yaml`

**特性:**

- ✅ 本地限流 (基于 Envoy Local Rate Limit 过滤器)
- ✅ 令牌桶算法 (可配置容量和补充速率)
- ✅ 限流响应头 (返回限流信息)
- ✅ 429 Too Many Requests 响应

**关键配置:**

```yaml
token_bucket:
  max_tokens: 100              # 令牌桶容量
  tokens_per_fill: 10          # 每次补充令牌数
  fill_interval: 1s            # 补充间隔
```

**测试限流:**

```bash
# 发送超过限量的请求
kubectl exec fortio -- fortio load -qps 200 -n 200 http://golang-app:8080/rest/v1/items

# 检查是否返回 429
kubectl logs deployment/golang-app -c istio-proxy | grep "local_rate_limit"

# 查看限流统计
kubectl exec deployment/golang-app -c istio-proxy -- curl localhost:15000/stats | grep rate_limit
```

### API 网关 (API Gateway)

配置文件: `gateway.yaml` + `virtual-service-gateway.yaml`

**特性:**

- ✅ L7 路由 (基于路径、请求头、参数)
- ✅ TLS 终止 (HTTPS)
- ✅ 强制 HTTPS 跳转
- ✅ CORS 配置
- ✅ 超时和重试
- ✅ 故障注入 (测试用)

**路由规则:**

| 路径前缀 | 目标服务 | 端口 |
|---------|---------|------|
| `/rest/v1/items` | golang-app | 8080 |
| `/rest/v1/orders` | golang-app2 | 8081 |
| `/admin/health` | golang-app | 8080 |

**测试 API 网关:**

```bash
# 获取 Ingress Gateway IP
export INGRESS_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# 测试 HTTP 路由 (自动跳转 HTTPS)
curl -v http://$INGRESS_IP/rest/v1/items -H "Host: api.example.com"

# 测试 HTTPS 路由
curl -v https://api.example.com/rest/v1/items

# 测试 CORS
curl -v http://$INGRESS_IP/rest/v1/items \
  -H "Host: api.example.com" \
  -H "Origin: https://app.example.com" \
  -H "Access-Control-Request-Method: GET" \
  -X OPTIONS
```

## Make 命令

更新后的 Makefile 包含以下 Istio 相关命令:

| 命令 | 说明 |
|------|------|
| `make istio-install` | 安装 Istio 服务网格 |
| `make istio-uninstall` | 卸载 Istio 服务网格 |
| `make istio-deploy` | 部署微服务 (带 Istio sidecar) |
| `make istio-governance` | 应用服务治理配置 |
| `make istio-verify` | 验证部署 |
| `make istio-test` | 运行测试 |
| `make istio-cleanup` | 清理配置和微服务 |
| `make istio-dashboard` | 打开 Istio 仪表板 |
| `make istio-all` | 完整安装 (install + deploy + governance) |
| `make helm-istio` | 使用 Helm 部署 (启用 Istio) |

## 可观测性

### 安装可观测性工具

```bash
# 安装 Kiali (服务拓扑可视化)
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/kiali.yaml

# 安装 Prometheus (指标收集)
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/prometheus.yaml

# 安装 Grafana (指标可视化)
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/grafana.yaml

# 安装 Jaeger (分布式追踪)
kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/jaeger.yaml
```

### 访问仪表板

```bash
# 打开所有仪表板
make istio-dashboard

# 或分别打开
istioctl dashboard kiali      # http://localhost:20001
istioctl dashboard prometheus  # http://localhost:9090
istioctl dashboard grafana     # http://localhost:3000
istioctl dashboard jaeger      # http://localhost:16686
```

## 验证和测试

### 验证部署

```bash
# 检查 Pod 状态
kubectl get pods -n default
kubectl get pods -n istio-system

# 检查 Istio 配置
kubectl get gateway,virtualservice,destinationrule,envoyfilter -n default

# 验证 Sidecar 注入
kubectl describe pod -l app=golang-app -n default | grep "istio-proxy"
```

### 查看 Proxy 配置

```bash
# 查看集群配置
istioctl proxy-config cluster deployment/golang-app

# 查看监听器配置
istioctl proxy-config listener deployment/golang-app

# 查看路由配置
istioctl proxy-config route deployment/golang-app
```

### 查看日志

```bash
# 查看 Ingress Gateway 访问日志
kubectl logs -l app=istio-ingressgateway -n istio-system -c istio-proxy --tail=100

# 查看 Sidecar 代理日志
kubectl logs deployment/golang-app -c istio-proxy --tail=100
```

## 清理

```bash
# 方式 1: 使用部署脚本
./deploy-istio.sh cleanup

# 方式 2: 使用 Make 命令
make istio-cleanup

# 方式 3: 手动清理
kubectl delete -f istio-config/
helm uninstall golang-system -n default
istioctl uninstall -y --purge
kubectl delete namespace istio-system
```

## 参考资料

- [Istio 官方文档](https://istio.io/latest/docs/)
- [Istio 流量管理](https://istio.io/latest/docs/concepts/traffic-management/)
- [Istio 安全](https://istio.io/latest/docs/concepts/security/)
- [Istio 可观测性](https://istio.io/latest/docs/concepts/observability/)
- [rke2 文档](https://docs.rke2.io/)
- [Envoy 文档](https://www.envoyproxy.io/docs/envoy/latest/)

## 故障排查

### 问题 1: Istio Sidecar 未注入

**症状:** Pod 只有一个容器 (没有 istio-proxy)

**解决:**

```bash
# 检查 namespace 标签
kubectl get namespace -L istio-injection

# 添加标签
kubectl label namespace default istio-injection=enabled --overwrite

# 重启 Pod
kubectl rollout restart deployment/golang-app
kubectl rollout restart deployment/golang-app2
```

### 问题 2: 服务间调用失败

**症状:** 服务间 HTTP 调用返回 503 或超时

**解决:**

```bash
# 检查 mTLS 配置
kubectl get peerauthentication -A

# 检查服务端口名称 (必须是 <protocol>[-<suffix>] 格式)
kubectl get svc golang-app -o yaml

# 查看 Sidecar 日志
kubectl logs deployment/golang-app -c istio-proxy --tail=100
```

### 问题 3: Ingress Gateway 无法访问

**症状:** 外部无法访问 Ingress Gateway

**解决:**

```bash
# 检查 Ingress Gateway Service
kubectl get svc istio-ingressgateway -n istio-system

# 检查外部 IP
kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[*]}'

# 检查 Gateway 和 VirtualService
kubectl get gateway,virtualservice -n default

# 查看 Ingress Gateway 日志
kubectl logs -l app=istio-ingressgateway -n istio-system --tail=100
```

## 更新日志

- **2026-01-15**: 初始版本
  - 创建 Istio 安装配置
  - 创建熔断配置 (DestinationRule)
  - 创建限流配置 (EnvoyFilter)
  - 创建 API 网关配置 (Gateway + VirtualService)
  - 创建自动化部署脚本
  - 更新 Makefile

---

**维护者:** golang-system 项目  
**许可证:** MIT
