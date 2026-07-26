#!/bin/bash
# =============================================================================
# Istio 服务治理自动化部署脚本
# 功能: 在 rke2 集群上部署 Istio 并配置服务治理
# =============================================================================

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        log_error "$1 未安装，请先安装"
        exit 1
    fi
}

# 检查依赖
check_dependencies() {
    log_info "检查依赖..."
    check_command kubectl
    check_command helm
    check_command istioctl
}

# 安装 Istio
install_istio() {
    log_info "开始安装 Istio..."
    
    # 检查 Istio 是否已安装
    if kubectl get namespace istio-system &> /dev/null; then
        log_warn "Istio 已安装，跳过安装步骤"
        return
    fi
    
    # 安装 Istio
    log_info "使用配置文件安装 Istio..."
    istioctl install -f istio-config/istio-config.yaml -y
    
    # 等待 Istio 组件就绪
    log_info "等待 Istio 组件就绪..."
    kubectl rollout status deployment/istiod -n istio-system
    kubectl rollout status deployment/istio-ingressgateway -n istio-system
    
    # 启用 default namespace 自动注入
    log_info "启用 default namespace Sidecar 自动注入..."
    kubectl label namespace default istio-injection=enabled --overwrite
    
    log_info "Istio 安装完成"
}

# 部署微服务
deploy_microservices() {
    log_info "开始部署微服务..."
    
    # 检查 Helm Chart 依赖
    log_info "更新 Helm Chart 依赖..."
    cd golang-system-chart
    helm dependency update
    cd ..
    
    # 部署或更新微服务
    log_info "部署微服务 (golang-app, golang-app2)..."
    helm upgrade --install golang-system ./golang-system-chart \
        --namespace default \
        --create-namespace \
        --values golang-system-chart/values.yaml \
        --wait \
        --timeout 10m
    
    # 等待 Pod 就绪 (包括 Istio sidecar)
    log_info "等待微服务 Pod 就绪 (包括 Istio sidecar)..."
    kubectl rollout status deployment/golang-app -n default
    kubectl rollout status deployment/golang-app2 -n default
    
    # 验证 Sidecar 注入
    log_info "验证 Sidecar 注入..."
    kubectl get pods -n default
    kubectl describe pod -l app=golang-app -n default | grep "istio-proxy" || log_warn "Sidecar 可能未注入"
    
    log_info "微服务部署完成"
}

# 配置服务治理
configure_service_governance() {
    log_info "开始配置服务治理..."
    
    # 应用熔断配置
    log_info "应用熔断配置..."
    kubectl apply -f istio-config/destination-rule-circuit-breaking.yaml
    
    # 应用限流配置
    log_info "应用限流配置..."
    kubectl apply -f istio-config/envoy-filter-rate-limiting.yaml
    
    # 配置 API 网关
    log_info "配置 API 网关..."
    kubectl apply -f istio-config/gateway.yaml
    kubectl apply -f istio-config/virtual-service-gateway.yaml
    
    log_info "服务治理配置完成"
}

# 安装可观测性工具
install_observability() {
    log_info "开始安装可观测性工具..."
    
    # 安装 Kiali
    log_info "安装 Kiali (服务拓扑可视化)..."
    kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/kiali.yaml
    
    # 安装 Prometheus
    log_info "安装 Prometheus (指标收集)..."
    kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/prometheus.yaml
    
    # 安装 Grafana
    log_info "安装 Grafana (指标可视化)..."
    kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/grafana.yaml
    
    # 安装 Jaeger
    log_info "安装 Jaeger (分布式追踪)..."
    kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/addons/jaeger.yaml
    
    # 等待组件就绪
    log_info "等待可观测性工具就绪..."
    kubectl rollout status deployment/kiali -n istio-system
    kubectl rollout status deployment/prometheus -n istio-system
    kubectl rollout status deployment/grafana -n istio-system
    kubectl rollout status deployment/jaeger -n istio-system
    
    log_info "可观测性工具安装完成"
}

# 验证部署
verify_deployment() {
    log_info "开始验证部署..."
    
    # 检查 Pod 状态
    log_info "检查 Pod 状态..."
    kubectl get pods -n default
    kubectl get pods -n istio-system
    
    # 检查 Service
    log_info "检查 Service..."
    kubectl get svc -n default
    kubectl get svc -n istio-system
    
    # 检查 Istio 配置
    log_info "检查 Istio 配置..."
    kubectl get gateway,virtualservice,destinationrule,envoyfilter -n default
    
    # 获取 Ingress Gateway IP
    log_info "获取 Ingress Gateway 外部访问地址..."
    INGRESS_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "pending")
    INGRESS_HOST=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "pending")
    
    if [ "$INGRESS_IP" != "pending" ] && [ "$INGRESS_IP" != "" ]; then
        log_info "Ingress Gateway IP: $INGRESS_IP"
        log_info "测试命令: curl http://$INGRESS_IP/rest/v1/items -H \"Host: api.example.com\""
    elif [ "$INGRESS_HOST" != "pending" ] && [ "$INGRESS_HOST" != "" ]; then
        log_info "Ingress Gateway Host: $INGRESS_HOST"
        log_info "测试命令: curl http://$INGRESS_HOST/rest/v1/items -H \"Host: api.example.com\""
    else
        log_warn "Ingress Gateway 外部 IP 尚未分配，请稍后检查"
    fi
    
    log_info "验证完成"
}

# 运行测试
run_tests() {
    log_info "开始运行测试..."
    
    # 部署 fortio (负载测试工具)
    log_info "部署 fortio (负载测试工具)..."
    kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/sample-client/fortio-deploy.yaml
    kubectl rollout status deployment/fortio
    
    # 等待 fortio 就绪
    sleep 5
    
    # 获取 Ingress Gateway IP
    INGRESS_IP=$(kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "127.0.0.1")
    
    # 测试服务发现
    log_info "测试服务发现..."
    kubectl exec fortio -- curl -s http://golang-app:8080/rest/v1/items | head -20
    
    # 测试 API 网关路由
    log_info "测试 API 网关路由..."
    kubectl exec fortio -- curl -s http://$INGRESS_IP/rest/v1/items -H "Host: api.example.com" | head -20
    
    # 测试熔断 (发送大量请求)
    log_info "测试熔断 (发送 1000 个请求)..."
    kubectl exec fortio -- fortio load -qps 100 -n 1000 -loglevel Debug http://golang-app:8080/rest/v1/items
    
    # 查看熔断状态
    log_info "查看熔断状态..."
    kubectl exec deployment/golang-app -c istio-proxy -- curl -s localhost:15000/stats | grep outlier | head -10
    
    # 测试限流
    log_info "测试限流 (发送超过限量的请求)..."
    kubectl exec fortio -- fortio load -qps 200 -n 200 http://golang-app:8080/rest/v1/items
    
    # 查看限流统计
    log_info "查看限流统计..."
    kubectl exec deployment/golang-app -c istio-proxy -- curl -s localhost:15000/stats | grep rate_limit | head -10
    
    log_info "测试完成"
}

# 清理环境
cleanup() {
    log_warn "开始清理环境..."
    
    # 删除 Istio 配置
    log_info "删除 Istio 配置..."
    kubectl delete -f istio-config/gateway.yaml --ignore-not-found=true
    kubectl delete -f istio-config/virtual-service-gateway.yaml --ignore-not-found=true
    kubectl delete -f istio-config/destination-rule-circuit-breaking.yaml --ignore-not-found=true
    kubectl delete -f istio-config/envoy-filter-rate-limiting.yaml --ignore-not-found=true
    
    # 卸载微服务
    log_info "卸载微服务..."
    helm uninstall golang-system -n default --ignore-not-found=true
    
    # 卸载 Istio
    log_info "卸载 Istio..."
    istioctl uninstall -y --purge
    kubectl delete namespace istio-system --ignore-not-found=true
    
    # 删除 cert-manager (如果安装)
    log_info "删除 cert-manager..."
    kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml --ignore-not-found=true
    
    log_info "清理完成"
}

# 显示帮助信息
show_help() {
    echo "Istio 服务治理自动化部署脚本"
    echo ""
    echo "用法: $0 [命令]"
    echo ""
    echo "命令:"
    echo "  install         完整安装 (Istio + 微服务 + 服务治理 + 可观测性)"
    echo "  istio           仅安装 Istio"
    echo "  microservices   仅部署微服务"
    echo "  governance      仅配置服务治理"
    echo "  observability   仅安装可观测性工具"
    echo "  verify          验证部署"
    echo "  test            运行测试"
    echo "  cleanup         清理环境"
    echo "  help            显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 install      # 完整安装"
    echo "  $0 verify       # 验证部署"
    echo "  $0 test         # 运行测试"
    echo ""
}

# 主函数
main() {
    case "$1" in
        install)
            check_dependencies
            install_istio
            deploy_microservices
            configure_service_governance
            install_observability
            verify_deployment
            log_info "========================================"
            log_info "完整安装完成！"
            log_info "========================================"
            ;;
        istio)
            check_dependencies
            install_istio
            ;;
        microservices)
            check_dependencies
            deploy_microservices
            ;;
        governance)
            check_dependencies
            configure_service_governance
            ;;
        observability)
            check_dependencies
            install_observability
            ;;
        verify)
            check_dependencies
            verify_deployment
            ;;
        test)
            check_dependencies
            run_tests
            ;;
        cleanup)
            check_dependencies
            cleanup
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"
