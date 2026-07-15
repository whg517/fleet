# ADR-0003: K8s 部署通过 Argo CD

## 状态

Accepted（v2 — 平台托管模式，2026-07-15 更新）

## 背景

平台需要在多个 K8s 集群的多个环境（dev/test/pre/prod）上部署服务。需要选择部署驱动方式。

核心诉求：
- 部署状态可追溯（平台数据库为 source of truth）
- 支持回滚
- 平台不可用时已部署服务不受影响
- 与 Helm Chart 生态兼容

## 决策

平台通过 Argo CD API 直接管理 Application CRD（创建、更新 targetRevision/values、触发 sync），不依赖 Git 仓库。

部署期望状态（values override、image tag 等）存储在平台数据库中，平台通过 Argo CD API 同步到 K8s。

## 后果

### 正面
- 平台完全托管，不引入 Git 作为运行时依赖
- Argo CD 提供 diff、sync、rollback、health 检查能力
- 平台是控制面，挂了不影响线上服务运行
- 配置变更和部署状态统一存数据库，查询和审计更直接
- 无 Git 并发写入问题

### 负面
- 引入额外组件，增加了运维复杂度
- Argo CD Application 管理需要平台自行封装
- 平台数据库是唯一 source of truth，需确保数据库高可用

### 中性
- 系统配置备份可导出到 Git/S3，属 DR 功能
- Argo CD 与平台的权限边界需要明确

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| GitOps 模式（Git + Argo CD 自动 reconcile） | Git 并发写入瓶颈、引入 Git 作为运行时依赖、部署配置与源码仓库耦合 |
| 直接调 K8s API + Helm | 期望状态不可追溯，无 diff 能力 |
| Flux CD | 社区小于 Argo CD，UI 和 API 能力较弱 |
| Jenkins X | 已过时，社区活跃度低 |
