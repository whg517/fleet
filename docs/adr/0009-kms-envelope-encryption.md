# ADR-0009: 敏感凭证存储策略

## 状态

Accepted（2026-07-12 修订，移除云 KMS 方案）

## 背景

平台需要存储 kubeconfig、Harbor 密码、Git token 等敏感凭证。
ADR-0006 确定了 PostgreSQL 作为存储。

平台部署在 K8s 环境中，需要在安全性和运维复杂度之间取得平衡。

## 决策

采用 **K8s Secret + 应用层 AES-256-GCM 加密** 方案：

### 加密架构

````
敏感凭证（kubeconfig、Harbor 密码、Git token）
  ↓ AES-256-GCM 加密
PostgreSQL（存储密文）

加密密钥（DEK）→ K8s Secret 注入 Pod
  ↑
K8s etcd encryption-at-rest（前置条件，保护 Secret 静态数据）
````

### 硬性前置条件

> **K8s 集群必须启用 etcd encryption-at-rest。**

如果 etcd 未加密，K8s Secret 中的 DEK 会被明文存储，攻击者获取 etcd 数据即可解密 PG 中的所有凭证。验证方式：

```bash
# 检查 kube-apiserver 是否启用 encryption-at-rest
kubectl get --raw=/api/v1/secrets 2>/dev/null | head -1
# 或检查 apiserver 启动参数
grep encryption-provider /etc/kubernetes/manifests/kube-apiserver.yaml
```

### DEK 轮转

- DEK 定期轮转（周期 30 天），通过更新 K8s Secret 触发
- 轮转时读取所有加密凭证 → 用旧 DEK 解密 → 用新 DEK 加密 → 写回 PG
- 轮转操作通过平台管理接口触发，记录审计日志
- 旧 DEK 保留至所有凭证重新加密完成，之后从 Secret 中移除

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
