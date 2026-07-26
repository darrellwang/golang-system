# golang-system 项目长期记忆

## 项目概览
- 六边形架构 + 事件驱动 Go 微服务系统（Go 1.23）
- 模块：golang-commons（共享库）、golang-events（事件契约）、golang-app（Item 服务）、golang-app2（Order 服务）、golang-system-chart（Helm）、istio-config（服务网格）
- 技术栈：PostgreSQL 16、NATS JetStream、Istio、chi router、zerolog、pgx/v5
- 服务间通信：HTTP（同步，httpclient 带 X-Request-ID 透传）+ NATS（异步事件）

## 架构约定（重要）
- 依赖方向：app → commons/events，不可反向；service 通过接口依赖 repository/publisher，不 import adapter
- 组合根在 internal/app/app.go，是唯一同时知道接口与实现的地方
- SQL 全部参数化（$1,$2）；时间统一 UTC；错误用 golang-commons/apperror 结构化
- 事件契约变更须经"架构守门人"评审，向后兼容（字段只增不删，或版本化 subject）

## 代码审查标准（2026-07-02 制定）
- 文档：CODE_REVIEW_STANDARDS.html（20 章节，12 大维度，本项目真实反模式实例）
- 优先级：🔴 Blocker / 🟡 Suggestion / 💭 Nit，每条评论必须标注
- PR SLA：首轮 4h、小 PR 1d、大 PR 2d、再审 1d

## 首次审查与修复（2026-07-02）
- 报告：CODE_REVIEW_REPORT.html
- 发现 10 处问题（3🔴 + 4🟡 + 3💭），已修复 7 处
- 关键修复：事件发布 context.WithoutCancel + 错误日志（6 处）、URL PathEscape、MaxBytesReader、CORS PROD 警告
- 待决策：ListItems 入参校验策略（API 契约）、trackedItems 内存泄漏（需重构）、NATS publisher ctx 透传（待 JetStream 迁移）

## 环境注意
- X: 盘是 WSL drvfs，不支持 Go 文件锁（RLock "Incorrect function"），go build/test 需复制到原生 Windows 路径（如 C:\Users\Darrell\tmp\）或用 GOWORK=off + 单模块构建
- -race 需 cgo，Windows Git Bash 下需 gcc；CI/Linux 下 make test 正常
