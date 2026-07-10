# ADR-0004: 物理节点部署通过 Ansible Role 模板

## 状态

Accepted

## 背景

绝大多数服务在 K8s 上，但部分服务需要部署到物理节点。
平台需要提供统一的部署入口，支持基于物理节点的标准化部署。

核心诉求：
- 部署逻辑模板化封装，平台只管传参数（与 Helm Chart 类比）
- 幂等性（重复执行结果一致）
- 执行环境与平台后端隔离

## 决策

物理节点服务通过 Ansible Role 封装部署逻辑。
每个服务一个 Role，包含 manifest.yaml（契约）、defaults（默认参数）、tasks（部署逻辑）、templates（配置模板）。
平台通过 K8s Job 触发 Ansible Runner 执行，SSH 密钥通过 Secret 挂载，不落平台后端磁盘。

## 后果

### 正面
- Ansible 幂等、成熟、社区生态好
- Role 模板化封装，与 Helm Chart 概念对齐
- K8s Job 隔离执行，平台后端不承担 SSH 风险
- 平台统一入口，底层按 deploy_type 分流（K8s → Argo CD，物理节点 → Ansible）

### 负面
- 需要维护两套模板体系（Helm + Ansible）
- 物理节点部署不走 GitOps，审计依赖平台记录

### 中性
- 物理节点服务占比小，维护成本可控

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 自研 shell 脚本 | 不可维护，无幂等保证 |
| SaltStack | 团队不熟悉，生态不如 Ansible |
