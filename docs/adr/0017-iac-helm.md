# ADR-0017: 基础设施即代码采用 Helm

## 状态

Accepted（v2 — 平台托管模式，2026-07-15 更新）

## 背景

Fleet 平台自身的 K8s 资源需要声明式管理。
需要选择 IaC（Infrastructure as Code）方案。

## 决策

采用 **Helm Chart** 管理 Fleet 平台自身的 K8s 部署。

- Fleet 后端、前端、PostgreSQL、Redis 等组件通过 Helm Chart 打包
- 不同环境（dev/test/pre/prod）通过 values override 区分
- Fleet 自身的 Helm Chart 发布为 OCI 制品，通过 Argo CD 部署

## 后果

### 正面
- Helm 是 K8s 包管理事实标准
- 与 Argo CD 部署体系一致（Fleet 管理服务用 Helm，自身也用 Helm）
- values override 支持多环境配置
- 模板化便于参数化部署

### 负面
- Helm 模板语言（Go template）在复杂逻辑下可读性差
- 不如 Kustomize 简洁（但功能更强）

### 中性
- Fleet 自身的 Helm Chart 作为 OCI 制品存储在 Harbor 中

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| Kustomize | 功能不如 Helm（无包管理、无模板化） |
| Carvel ytt | 社区小，学习成本高 |
| Pulumi | 编程式 IaC，对 K8s 声明式理念不匹配 |
