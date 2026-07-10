# ADR-0004: 物理机部署通过 Ansible Role 模板

## 状态

Accepted

## 背景

绝大多数服务在 K8s 上，但有个别老服务部署在物理机/VM 上。
需要一种方式将这些物理机服务纳入统一平台管理。

核心诉求：
- 部署逻辑模板化封装，平台只管传参数（与 Helm Chart 类似）
- 幂等性（重复执行结果一致）
- 幂等操作

## 决策

物理机/VM 服务通过 Ansible Role 封装部署逻辑。
每个服务一个 Role，包含 manifest.yaml（契约）、defaults（默认参数）、tasks（部署逻辑）、templates（配置模板）。
平台通过 Ansible Runner 触发部署。

## 后果

### 正面
- Ansible 幂等、成熟、社区生态好
- Role 模板化封装，与 Helm Chart 概念对齐
- 平台统一入口，底层按 deploy_type 分流
- 新物理机服务接入只需写一个 Role

### 负面
- 需要维护两套模板体系（Helm + Ansible）
- Ansible Runner 调用比 Argo CD API 复杂
- 物理机服务不支持 Argo CD 的 GitOps 能力

### 中性
- 物理机服务占比极小，维护成本可控

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 自研 shell 脚本 | 不可维护，无幂等保证 |
| SaltStack | 团队不熟悉，生态不如 Ansible |
| 让物理机服务迁移到 K8s | 理想但短期不可行 |
