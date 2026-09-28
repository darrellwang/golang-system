# syntax=docker/dockerfile:1
# =============================================================================
# golang-app (Item Service) 多阶段构建
# 构建上下文必须是仓库根目录（go.work 与 replace 指令依赖兄弟模块）:
#   docker build -f golang-app/Dockerfile -t golang-app:latest .
# =============================================================================

# ---------- 构建阶段 ----------
FROM golang:1.27.1-alpine AS builder
WORKDIR /src

# GOPROXY 可通过 --build-arg 覆盖（如 GOPROXY=https://goproxy.cn,direct）
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

# 先拷贝工作区与模块描述文件，利用 Docker 层缓存加速依赖下载
COPY go.work go.work.sum ./
COPY golang-commons/go.mod golang-commons/go.sum ./golang-commons/
COPY golang-events/go.mod ./golang-events/
COPY golang-app/go.mod golang-app/go.sum ./golang-app/
COPY golang-app2/go.mod golang-app2/go.sum ./golang-app2/
RUN cd golang-app && go mod download

# 拷贝全部模块源码（go.work 的 use 指令引用全部四个模块，缺一不可）
COPY golang-commons ./golang-commons
COPY golang-events ./golang-events
COPY golang-app ./golang-app
COPY golang-app2 ./golang-app2

# 静态编译服务与迁移 CLI（注入版本号）
ARG VERSION=dev
RUN cd golang-app && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags="-s -w -X github.com/darrell/golang-system/golang-app/internal/config.Version=${VERSION}" \
      -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o /out/migrate ./cmd/migrate

# ---------- 运行阶段 ----------
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 appuser
WORKDIR /home/appuser

COPY --from=builder /out/server ./server
COPY --from=builder /out/migrate ./migrate
# 迁移 SQL（容器内执行: DATABASE_URL=... ./migrate up，MIGRATE_PATH 默认 ./db/migrations）
COPY golang-app/db/migrations ./db/migrations

USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${APP_PORT:-8080}/admin/health/liveness" || exit 1
ENTRYPOINT ["./server"]
