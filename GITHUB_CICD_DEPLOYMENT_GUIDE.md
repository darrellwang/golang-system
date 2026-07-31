# GitHub Actions CI/CD 部署指南（golang-system）

> 面向 DevOps / 平台工程：基于本仓库现有 Workflow，从零配置到首次上线、再到日常发布的完整步骤。
>
> 适用版本：Go **1.26.1** · Helm **3.15+** · Kubernetes（推荐 RKE2）· Istio · GitHub Container Registry（GHCR）
>
> 文档日期：2026-07-30

---

## 目录

1. [流水线总览](#1-流水线总览)
2. [前置条件检查清单](#2-前置条件检查清单)
3. [一次性准备：GitHub 仓库与权限](#3-一次性准备github-仓库与权限)
4. [一次性准备：Environments 与 Secrets](#4-一次性准备environments-与-secrets)
5. [一次性准备：Kubernetes / 镜像拉取](#5-一次性准备kubernetes--镜像拉取)
6. [一次性准备：Istio 与集群基座](#6-一次性准备istio-与集群基座)
7. [CI 流水线说明与验证](#7-ci-流水线说明与验证)
8. [CD 流水线说明与首次部署](#8-cd-流水线说明与首次部署)
9. [Release（打 Tag 发版）](#9-release打-tag-发版)
10. [日常发布操作手册](#10-日常发布操作手册)
11. [回滚与应急](#11-回滚与应急)
12. [排障手册](#12-排障手册)
13. [已知缺口与建议增强](#13-已知缺口与建议增强)
14. [附录](#14-附录)

---

## 1. 流水线总览

本仓库已包含三套 Workflow：

| Workflow | 文件 | 触发条件 | 作用 |
|---|---|---|---|
| **CI** | `.github/workflows/ci.yml` | `push` → `main`/`develop`；`pull_request` → `main` | 变更检测 → Lint → Test → Build →（仅 main）推送镜像到 GHCR；Chart 变更时 helm lint |
| **CD** | `.github/workflows/cd.yml` | `push` → `main`；或手动 `workflow_dispatch` | `development` → `staging` → `production` 逐级 Helm 发布 |
| **Release** | `.github/workflows/release.yml` | `push` tag `v*` | 多架构二进制 + GitHub Release + Helm Chart 推 OCI |

```
开发者 Push / PR
        │
        ▼
┌────────────────── CI ──────────────────┐
│ paths-filter → lint / test / build     │
│ main + 业务变更 → docker build & push  │
│ chart 变更 → helm lint/template        │
└──────────────────┬─────────────────────┘
                   │ (main 合并后)
                   ▼
┌────────────────── CD ──────────────────┐
│ Environment: development (自动/可审批) │
│ Environment: staging     (建议人工审批) │
│ Environment: production  (必须人工审批) │
│ helm upgrade --install + rollout 冒烟  │
└────────────────────────────────────────┘

打 Tag vX.Y.Z ──► Release（二进制 + OCI Chart）
```

**镜像命名约定（CI 推送）：**

```text
ghcr.io/<github_owner>/<github_repo>/golang-app:<git_sha>
ghcr.io/<github_owner>/<github_repo>/golang-app:latest
ghcr.io/<github_owner>/<github_repo>/golang-app2:<git_sha>
ghcr.io/<github_owner>/<github_repo>/golang-app2:latest
```

示例：仓库为 `darrell/golang-system` 时：

```text
ghcr.io/darrell/golang-system/golang-app:a1b2c3d...
```

---

## 2. 前置条件检查清单

在配置 CI/CD 前，确认以下项全部就绪：

| # | 项 | 要求 |
|---|---|---|
| 1 | GitHub 仓库 | 已推送本仓库代码；Actions 已启用 |
| 2 | 本地/CI Go | **1.26.1**（与 `go.work` / CI 一致） |
| 3 | 集群 | 至少 1 套 K8s（dev）；建议 dev / staging / prod 隔离 |
| 4 | kubectl | 能对目标集群执行 `kubectl get ns` |
| 5 | Helm | ≥ 3.15 |
| 6 | Istio | 已安装或计划用 `./deploy-istio.sh install` 安装 |
| 7 | 域名 / TLS | 生产需准备 `api.example.com`（或替换）与 cert-manager |
| 8 | 权限 | 仓库 Admin（配 Secrets / Environments）；集群有部署权限 |

---

## 3. 一次性准备：GitHub 仓库与权限

### 3.1 启用 Actions 与 Packages

1. 打开仓库 → **Settings → Actions → General**
   - Actions permissions：允许 **Allow all actions and reusable workflows**（或按组织策略放行必要 Action）
   - Workflow permissions：勾选 **Read and write permissions**（CI 推 GHCR、Release 写 Release 需要）
   - 勾选 **Allow GitHub Actions to create and approve pull requests**（Dependabot / 自动化 PR 可选）

2. 打开 **Settings → Packages**
   - 确认 GitHub Packages / GHCR 可用
   - 首次推送镜像后，进入 Package 设置，将可见性设为：
     - 私有仓库：保持 Private，并给集群配置 `imagePullSecrets`
     - 若希望集群无凭证拉取：Package → Change visibility → Public（仅非敏感场景）

### 3.2 确认 Workflow 文件在默认分支

确保以下文件在 `main`（或你用作生产的默认分支）上存在：

```text
.github/workflows/ci.yml
.github/workflows/cd.yml
.github/workflows/release.yml
.github/dependabot.yml
```

### 3.3 （推荐）保护默认分支

**Settings → Branches → Add rule**（`main`）：

- Require a pull request before merging
- Require status checks to pass：至少勾选 CI 的 `test` / `lint`（首次跑通后会出现在列表中）
- Restrict who can push to matching branches

---

## 4. 一次性准备：Environments 与 Secrets

CD 使用三个 GitHub Environment：`development`、`staging`、`production`。

### 4.1 创建 Environment

仓库 → **Settings → Environments → New environment**，分别创建：

| Environment 名称 | 建议审批 | 对应 Secret |
|---|---|---|
| `development` | 可无审批（或仅通知） | `KUBE_CONFIG_DEV` |
| `staging` | **Required reviewers** ≥ 1 | `KUBE_CONFIG_STAGING` |
| `production` | **Required reviewers** ≥ 1（建议 ≥ 2） | `KUBE_CONFIG_PROD` |

对 `staging` / `production`：

1. 进入 Environment → **Protection rules**
2. 勾选 **Required reviewers**，添加负责人
3. （可选）勾选 **Wait timer**（例如 prod 等待 5–30 分钟）
4. （可选）限制部署分支为 `main`

### 4.2 准备 kubeconfig Secret

在能访问目标集群的机器上：

```bash
# 确认当前上下文是目标环境
kubectl config current-context
kubectl get nodes

# 导出完整 kubeconfig（注意：含证书/Token，视为高敏感凭据）
kubectl config view --raw --minify > kubeconfig-dev.yaml

# 快速自检（应能列出命名空间）
KUBECONFIG=./kubeconfig-dev.yaml kubectl get ns
```

对 staging / prod 分别切到对应 context 后重复导出：

```bash
kubectl config view --raw --minify > kubeconfig-staging.yaml
kubectl config view --raw --minify > kubeconfig-prod.yaml
```

**安全建议：**

- 使用最小权限的 ServiceAccount + 有时限的 kubeconfig（而非个人 admin 证书）
- 不要把 kubeconfig 提交进 Git
- 定期轮换；人员离职立即作废

### 4.3 写入 GitHub Secrets

对每个 Environment：

1. Environment → **Environment secrets → Add secret**
2. 名称与内容：

| Secret 名 | 写入位置 | 内容 |
|---|---|---|
| `KUBE_CONFIG_DEV` | Environment `development` | `kubeconfig-dev.yaml` 全文 |
| `KUBE_CONFIG_STAGING` | Environment `staging` | `kubeconfig-staging.yaml` 全文 |
| `KUBE_CONFIG_PROD` | Environment `production` | `kubeconfig-prod.yaml` 全文 |

> 也可用 **Repository secrets**，但 Environment secrets 更安全（仅对应环境 Job 可见）。

### 4.4 仓库级可选 Secrets

当前 Workflow 默认用 `GITHUB_TOKEN` 推 GHCR，**一般不需要**额外 PAT。仅在以下情况需要：

| Secret | 何时需要 |
|---|---|
| `GHCR_TOKEN` / PAT | 跨仓库推包、或组织策略禁用 GITHUB_TOKEN 写 Packages |
| DB 密码覆盖 | 若不想用 Chart 默认 `change-me`，可在 CD 用 `--set postgresApp.password=...`（更推荐 External Secrets） |

---

## 5. 一次性准备：Kubernetes / 镜像拉取

### 5.1 命名空间与 Sidecar 注入

```bash
kubectl create namespace default --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace default istio-injection=enabled --overwrite
```

> Chart 默认装在 `default`。若改用独立 ns（推荐 `golang-system`），需同步改 CD 的 `--namespace` 与 Istio 策略命名空间。

### 5.2 GHCR 私有镜像拉取凭证

若 Package 为 Private，集群必须能拉镜像：

```bash
# 创建只读 PAT：权限勾选 read:packages（组织仓库可能还需 SSO Authorize）
# 然后创建 Secret（在目标 namespace）
kubectl create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<ghcr-read-pat> \
  --docker-email=<email> \
  -n default
```

并在 Helm 部署时挂上（当前 Chart 模板若尚未引用 `imagePullSecrets`，需在模板中补充，或临时用：

```bash
kubectl patch serviceaccount default -n default -p \
  '{"imagePullSecrets":[{"name":"ghcr-pull"}]}'
```

）。

### 5.3 修正镜像仓库地址（必做）

CI 推送的完整路径是 `ghcr.io/<owner>/<repo>/golang-app`，而 Chart 默认 `repository: golang-app`。

**首次 CD 前**，在 `golang-system-chart/values.yaml` 改为真实仓库，或在 CD 中 `--set`：

```bash
# 将 <OWNER>/<REPO> 换成你的 GitHub 仓库路径，例如 darrell/golang-system
export REG=ghcr.io/<OWNER>/<REPO>

helm upgrade --install golang-system ./golang-system-chart \
  --set golangApp.image.repository=$REG/golang-app \
  --set golangApp2.image.repository=$REG/golang-app2 \
  --set golangApp.image.tag=<git_sha> \
  --set golangApp2.image.tag=<git_sha>
```

建议把上述 `repository` 写进各环境的 values 覆盖文件，例如：

```text
golang-system-chart/values-dev.yaml
golang-system-chart/values-staging.yaml
golang-system-chart/values-prod.yaml
```

并在 CD 增加 `-f values-<env>.yaml`（见第 13 节建议）。

### 5.4 数据库密码

Chart 默认 `postgresApp.password: change-me`。上线前务必：

```bash
helm upgrade --install golang-system ./golang-system-chart \
  --set postgresApp.password='<强密码>' \
  --set postgresApp2.password='<强密码>'
```

或使用 Sealed Secrets / External Secrets Operator 注入 `golang-app-db` / `golang-app2-db` Secret。

### 5.5 数据库迁移

应用启动**不会**自动 migrate。首次部署或 schema 变更后执行：

```bash
# 方式 A：本地指向集群内 DB（需 port-forward）
kubectl port-forward svc/postgres-app 5432:5432 &
cd golang-app && go run ./cmd/migrate up

kubectl port-forward svc/postgres-app2 5433:5432 &
# app2 默认本地 DB_PORT=5433
cd golang-app2 && go run ./cmd/migrate up   # 若尚无 migrate CLI，用 app 同款工具或补齐

# 方式 B：在 Job / 一次性 Pod 中跑镜像内 migrate 二进制（Dockerfile 已构建 migrate）
```

> 当前 `golang-app` 镜像含 `migrate`；`golang-app2` 需确认 Dockerfile 是否同样产出 migrate。上线 checklist 必须包含 outbox 相关迁移 `002_outbox_and_processed_events.sql`。

---

## 6. 一次性准备：Istio 与集群基座

CI/CD **不负责**安装 Istio；需在集群侧先完成（或由独立基础设施流水线完成）。

### 6.1 安装 Istio + 治理 + 可观测性

在有 `kubectl` / `helm` / `istioctl` 的运维机上，于仓库根目录：

```bash
./deploy-istio.sh install
# 或分阶段：
./deploy-istio.sh istio
./deploy-istio.sh microservices   # 也可留给 CD 的 Helm 完成
./deploy-istio.sh governance      # DR / PeerAuth / JWT / 限流 / Gateway / VS
./deploy-istio.sh observability
./deploy-istio.sh verify
```

治理清单关键项：

| 能力 | 文件 |
|---|---|
| Gateway / TLS | `istio-config/gateway.yaml` |
| 路由 / 重试 | `istio-config/virtual-service-gateway.yaml` |
| 熔断 LB | `istio-config/destination-rule-circuit-breaking.yaml` |
| mTLS | `istio-config/peer-authentication.yaml` |
| JWT | `istio-config/jwt-authentication.yaml` |
| 本地限流 | `istio-config/envoy-filter-rate-limiting.yaml` |
| 全局限流 | `istio-config/global-rate-limiting.yaml` |

### 6.2 JWT 联调（部署后）

```bash
pip install cryptography
TOKEN=$(python3 istio-config/jwt/mint-dev-jwt.py --sub user-1 --org org-1)

export INGRESS_IP=$(kubectl get svc istio-ingressgateway -n istio-system \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

curl -sk -H "Authorization: Bearer $TOKEN" -H "Host: api.example.com" \
  "https://${INGRESS_IP}/rest/v1/items"
```

生产环境必须把 `jwt-authentication.yaml` 中的 **dev inline JWKS** 换成真实 IdP 的 `jwksUri`。

---

## 7. CI 流水线说明与验证

### 7.1 Job 依赖关系

```text
changes ─┬─► lint (矩阵：commons/events/app/app2)
         ├─► test (同上 + Postgres×2 + NATS)
         ├─► helm (仅 chart 变更)
         └─► build (需 test；仅 app/app2)
                └─► docker (仅 push 到 main，且业务模块有变更)
```

### 7.2 首次验证 CI

1. 开一个小改动 PR 到 `main`（例如改 README）
2. 打开 **Actions** 页，确认 `CI` 运行
3. 若仅改文档，`paths-filter` 可能跳过 lint/test（属预期）；改 `golang-app/**` 应触发完整链路
4. 合并到 `main` 后，确认 `docker` Job 成功，并在  
   `https://github.com/<owner>/<repo>/pkgs/container/golang-app` 看到新 tag

### 7.3 本地等价命令（排障用）

```bash
# Lint
cd golang-app && golangci-lint run

# Test
go test -race ./...

# Build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

# Helm
helm lint golang-system-chart
helm template golang-system golang-system-chart >/tmp/out.yaml

# Docker（上下文必须是仓库根）
docker build -f golang-app/Dockerfile -t golang-app:local .
docker build -f golang-app2/Dockerfile -t golang-app2:local .
```

---

## 8. CD 流水线说明与首次部署

### 8.1 推荐首次上线顺序（人工引导）

不要一上来就全自动打穿到 prod。建议：

```text
第 1 天：只配 development Environment + KUBE_CONFIG_DEV
第 2 天：验证 Helm 发布、探针、镜像拉取、migrate、冒烟 API
第 3 天：再开 staging / production 审批门
```

### 8.2 手动触发 CD（首次推荐）

1. 确认 CI 已在 `main` 上成功推送镜像（记住某个 `github.sha`）
2. Actions → **CD** → **Run workflow** → 选 `main` → Run
3. 在 `development` Job 观察日志：
   - `helm upgrade --install` 成功
   - `kubectl rollout status deployment/golang-app` 成功
4. 若 Environment 配置了审批，在 Actions 页面 **Review deployments** 批准

### 8.3 CD 实际执行的命令（等价）

```bash
cd golang-system-chart
helm upgrade --install golang-system . \
  --set global.environment=dev \
  --set golangApp.image.tag=${GITHUB_SHA} \
  --set golangApp2.image.tag=${GITHUB_SHA} \
  --wait --timeout 10m

kubectl rollout status deployment/golang-app --timeout=5m
kubectl rollout status deployment/golang-app2 --timeout=5m
kubectl get svc golang-app golang-app2
```

**务必同时设置 image.repository**（见 5.3），否则集群会尝试拉 Docker Hub 上的短名镜像而失败。

### 8.4 部署后冒烟清单

```bash
kubectl get pods -l 'app in (golang-app,golang-app2)' -o wide
kubectl describe pod -l app=golang-app | head -80

# 探针与日志
kubectl logs -l app=golang-app -c golang-app --tail=100
kubectl logs -l app=golang-app2 -c golang-app2 --tail=100

# 集群内调用（临时 debug Pod）
kubectl run curl --rm -it --image=curlimages/curl -- \
  curl -sS http://golang-app:8080/admin/health/liveness

# 经网关（需 JWT）
TOKEN=$(python3 istio-config/jwt/mint-dev-jwt.py --sub smoke)
curl -sk -H "Authorization: Bearer $TOKEN" -H "Host: api.example.com" \
  "https://${INGRESS_IP}/rest/v1/items"
```

### 8.5 晋升到 Staging / Production

1. development 冒烟通过
2. CD 自动进入 `staging` Job → Environment 审批人批准
3. staging 冒烟通过后再批准 `production`
4. 生产发布后再次执行 8.4 清单，并观察：
   - HPA / 错误率 / 延迟（Grafana / Kiali）
   - 限流 429 是否符合预期
   - JWT 拒绝无 Token 请求（应为 401/403）

---

## 9. Release（打 Tag 发版）

用于对外发布二进制与 OCI Helm Chart（不等同于集群部署；集群部署仍走 CD）。

```bash
# 确保 main 已包含要发版的提交且 CI 绿
git checkout main
git pull

# 语义化版本
git tag -a v1.0.0 -m "release: v1.0.0"
git push origin v1.0.0
```

然后在 Actions 中观察 **Release**：

1. 多 OS/Arch 编译 `golang-app` / `golang-app2`
2. 创建 GitHub Release 并挂载产物
3. `helm package` + `helm push oci://ghcr.io/<owner>/<repo>`

拉取 Chart：

```bash
helm pull oci://ghcr.io/<owner>/<repo>/golang-system --version 0.1.0
```

---

## 10. 日常发布操作手册

### 10.1 标准功能发布（推荐）

1. 从 `main` 拉功能分支
2. 开发 → 本地 `go test` / `docker compose` 自测
3. 开 PR → 等待 **CI** 绿
4. Code Review → 合并到 `main`
5. CI 在 `main` 推镜像；CD 启动
6. 按 Environment 审批：dev 自动 / staging 批 / prod 批
7. 生产冒烟 + 观察 15–30 分钟

### 10.2 热修（Hotfix）

1. 从生产对应 commit 拉 `hotfix/*` 分支（或直接基于 main）
2. 最小改动 + 测试
3. PR 合并 main → 走完整 CD（不要跳过 staging，除非事故级别并有书面授权）
4. 事故级可临时 `workflow_dispatch` 只对 prod Environment 操作——**需先改 Workflow 或手动 helm**（见回滚）

### 10.3 仅改 Helm / Istio

- 只改 `golang-system-chart/**`：CI 跑 `helm` Job；合并后 CD 仍会用**当前 main 的 sha** 作为 image tag（若镜像未重建，tag 仍应存在于 GHCR）
- 只改 `istio-config/**`：**当前 CD 不会自动 apply Istio 清单**，需：
  ```bash
  kubectl apply -f istio-config/peer-authentication.yaml
  kubectl apply -f istio-config/jwt-authentication.yaml
  # ...或 ./deploy-istio.sh governance
  ```

### 10.4 Dependabot

`.github/dependabot.yml` 已配置每周更新 Go modules / Docker / Actions。合并 Dependabot PR 前确认 CI 通过。

---

## 11. 回滚与应急

### 11.1 Helm 回滚

```bash
helm history golang-system
helm rollback golang-system <REVISION> --wait --timeout 10m
kubectl rollout status deployment/golang-app
kubectl rollout status deployment/golang-app2
```

### 11.2 指定旧镜像 Tag 发布

```bash
# <OLD_SHA> 为已知可用的 Git commit sha
helm upgrade --install golang-system ./golang-system-chart \
  --set golangApp.image.repository=ghcr.io/<OWNER>/<REPO>/golang-app \
  --set golangApp2.image.repository=ghcr.io/<OWNER>/<REPO>/golang-app2 \
  --set golangApp.image.tag=<OLD_SHA> \
  --set golangApp2.image.tag=<OLD_SHA> \
  --wait --timeout 10m
```

### 11.3 暂停自动 CD

- Environment → 打开 **Deployment branches** 限制，或临时 Disable Environment
- 或 Actions → CD → 取消排队中的 run
- 紧急时：仓库 Settings → Actions → Disable actions（影响所有 Workflow，慎用）

---

## 12. 排障手册

| 现象 | 可能原因 | 处理 |
|---|---|---|
| CI `docker` 失败：找不到 go.work | 构建 context 不是仓库根 | 确认 `context: .` 且 `file: ./golang-app/Dockerfile` |
| CI Go 版本不符 | runner 缓存/旧 workflow | 确认 `go-version: "1.26.1"` 在默认分支 |
| CD ImagePullBackOff | repository 短名 / 无私有拉取凭证 | 修正 5.3 / 5.2 |
| Pod CrashLoop：探针失败 | 探针路径或端口错误 | 应为 `/admin/health/*`，端口 8080/8081 |
| Pod Running 但业务 401 | 网关 JWT | 带 Bearer Token；或临时检查 AuthorizationPolicy |
| CD 在 Environment 卡住 | 等待审批 | Actions 页 Approve |
| `helm upgrade` 超时 | 依赖 DB/NATS 未就绪、探针失败 | `kubectl describe pod` / `kubectl get events` |
| 事件不达对端 | 未跑 migrate / JetStream 未开 | 执行迁移；确认 NATS `-js` |
| staging 用了 prod kubeconfig | Secret 贴错 | 轮换 Secret，重跑 CD |

常用调试：

```bash
kubectl get deploy,po,svc,hpa
kubectl describe deploy golang-app
kubectl logs deploy/golang-app -c golang-app --tail=200
kubectl logs deploy/golang-app -c istio-proxy --tail=100
istioctl proxy-status
kubectl get requestauthentication,authorizationpolicy -n istio-system
kubectl get envoyfilter -A
```

---

## 13. 已知缺口与建议增强

以下基于当前 Workflow / Chart 的务实建议（上线前优先处理加粗项）：

1. **CD 应显式设置 `image.repository`**（指向 GHCR 全路径），避免依赖短名。
2. **CD 应等待 CI 镜像构建完成**（例如 `workflow_run` 监听 CI 成功，或把 docker + deploy 合成可编排流水线），避免 sha tag 尚未推送就部署。
3. **增加环境 values 文件**：`values-dev.yaml` / `values-staging.yaml` / `values-prod.yaml`（仓库地址、副本数、密码引用、域名）。
4. **Chart 增加 `imagePullSecrets`** 字段并在模板中渲染。
5. **Istio 治理纳入 CD 或独立 `infra` Workflow**（`kubectl apply -f istio-config/...`）。
6. **迁移 Job**：每次发布前/中跑 migrate（Helm pre-upgrade Hook 或独立 Job）。
7. **CD 冒烟增强**：不只 `rollout status`，增加带 JWT 的 HTTP 探测。
8. **镜像扫描**：在 CI docker 后加 Trivy / Grype；失败阻断合并。
9. **OIDC 替代长期 kubeconfig**：用云厂商 OIDC 或 Kubernetes OIDC 联邦登录，减少静态 kubeconfig Secret。

---

## 14. 附录

### 14.1 Workflow 与密钥速查

| 项 | 值 |
|---|---|
| CI Go | 1.26.1 |
| Helm | v3.15.0 |
| Registry | `ghcr.io` |
| CD Environments | `development` / `staging` / `production` |
| Secrets | `KUBE_CONFIG_DEV` / `KUBE_CONFIG_STAGING` / `KUBE_CONFIG_PROD` |
| 部署名 | `deployment/golang-app`、`deployment/golang-app2` |
| Release 名 | `golang-system` |

### 14.2 首次上线最小路径（Checklist）

- [ ] 仓库 Actions 写权限已开
- [ ] 创建三个 Environments，staging/prod 配置审批人
- [ ] 写入三个 `KUBE_CONFIG_*`
- [ ] 集群已装 Istio + 治理（或计划好安装窗口）
- [ ] GHCR 私有拉取 Secret 已配
- [ ] `image.repository` 已改为 `ghcr.io/<owner>/<repo>/golang-app(|2)`
- [ ] DB 密码已更换
- [ ] main 上 CI 出过至少一组镜像 tag
- [ ] 手动 Run CD → development 成功
- [ ] 完成 migrate + JWT 冒烟
- [ ] 再打通 staging / production 审批

### 14.3 相关文档

| 文档 | 内容 |
|---|---|
| `ARCHITECTURE.md` | 总体架构、Mesh、Outbox、JWT/限流 |
| `SETUP_AND_DEPLOY_GUIDE.md` | 本地开发与手工部署 |
| `istio-config/jwt/README.md` | Dev JWT 签发 |
| `deploy-istio.sh` | Istio / 治理一键脚本 |

---

**维护说明：** 若 Workflow 或 Chart 契约变更（环境变量名、Deployment 名、镜像路径），请同步更新本文档第 1、5、7、8、14 节。
