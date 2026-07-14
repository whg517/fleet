# ADR-0009: 敏感凭证存储策略

## 状态

Accepted（2026-07-12 修订，移除云 KMS 方案）

## 背景

平台需要存储 kubeconfig、Harbor 密码、Git token 等敏感凭证。
ADR-0006 确定了 PostgreSQL 作为存储。

平台部署在 K8s 环境中，需要在安全性和运维复杂度之间取得平衡。

## 决策

采用 **K8s Secret + 应用层 AES-256-GCM 加密** 方案：

- 对接外部系统的凭证（kubeconfig、Harbor 密码、Git token）通过 K8s Secret 挂载或环境变量注入
- 需要持久化到 PostgreSQL 的敏感字段，使用应用层 AES-256-GCM 加密
- 加密密钥通过 K8s Secret 注入，不在代码或配置文件中硬编码
- K8s 集群开启 etcd 加密（encryption at rest），Secret 数据落盘加密

## 后果

### 正面
- 不引入外部 KMS 依赖，运维简单
- K8s Secret 是集群原生机制，团队熟悉
- etcd 加密提供静态数据保护
- 应用层加密保证即使数据库泄露凭证也不可读

### 负面
- 加密密钥存在 K8s Secret 中，集群管理员可访问
- 密钥轮转需要重新部署应用
- 安全性不如独立 KMS，依赖 K8s 集群安全边界

### 中性
- 适用前提：K8s 集群本身是可信环境（RBAC 严格管控）
- 后续如安全要求提升，可引入外部 KMS

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 云 KMS 信封加密 | 引入云厂商依赖，当前阶段运维复杂度过高 |
| HashiCorp Vault | 运维复杂度高，团队无经验 |
| 明文环境变量 | 极度不安全 |
