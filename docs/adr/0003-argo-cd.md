# ADR-0003: K8s 部署通过 Argo CD

## 状态

Accepted

## 背景

平台需要在多个 K8s 集群的多个环境（dev/test/pre/prod）上部署服务。需要选择部署驱动方式。

核心诉求：
- 部署的期望状态可追溯（GitOps）
- 支持回滚
- 平台不可用时已部署服务不受影响
- 与 Helm Chart 生态兼容

## 决策

平台通过管理 Argo CD Application CRD 驱动部署，不直接操作 K8s API。
部署期望状态存储在 Git 仓库，Argo CD 负责调和。

## 后果

### 正面
- GitOps 模式：所有部署变更都有 Git 记录，天然审计
- Argo CD 提供 diff、sync、rollback、health 检查能力
- 平台是控制面，挂了不影响线上服务运行
- Argo CD 社区成熟，K8s 原生

### 负面
- 引入额外组件，增加了运维复杂度
- Argo CD Application 管理需要平台自行封装
- Git 操作增加了部署链路复杂度
- 部署延迟受 Git commit + Argo CD reconcile 周期影响

### 中性
- 需要维护 GitOps 仓库结构规范
- Argo CD 与平台的权限边界需要明确

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 直接调 K8s API + Helm | 放弃 GitOps，期望状态不可追溯，无 diff 能力 |
| Flux CD | 社区小于 Argo CD，UI 和 API 能力较弱 |
| Jenkins X | 已过时，社区活跃度低 |
