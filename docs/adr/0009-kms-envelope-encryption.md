# ADR-0009: 密钥管理采用云 KMS 信封加密

## 状态

Accepted

## 背景

平台需要存储 kubeconfig、Harbor 密码、Git token 等敏感凭证。
ADR-0006 确定了 PostgreSQL 作为存储，但加密主密钥（MEK）的存储方案未定义。

直接将 MEK 存在配置文件或环境变量中，一旦主机被入侵所有凭证暴露。
Non-Goals 明确不做 Vault，但需要一个可接受的安全方案。

## 决策

采用云 KMS（阿里云 KMS / AWS KMS）信封加密（Envelope Encryption）方案：

- MEK（主加密密钥）存放在云 KMS，平台不持有明文 MEK
- DEK（数据加密密钥）由 KMS 生成，加密后缓存在内存中
- 凭证数据通过 DEK + AES-256-GCM 加密后存储在 PostgreSQL
- DEK 定期轮转（30 天）

## 后果

### 正面
- MEK 不落盘，安全性远高于配置文件存储
- KMS 有完整的访问审计日志
- DEK 轮转不影响历史数据（新 DEK 加密新数据，旧 DEK 保留用于解密）
- 不引入 Vault 的运维复杂度

### 负面
- 依赖云 KMS 可用性（KMS 不可用时无法解密新凭证）
- KMS API 调用有成本（虽然很低）

### 中性
- 需要 KMS 访问权限配置

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| HashiCorp Vault | Non-Goals 明确排除，运维复杂度高 |
| Linux keyring + 密钥文件 | 密钥文件仍在磁盘上，安全性不足 |
| 明文环境变量 | 极度不安全 |
