# ADR-0005: 构建通过 Argo Workflows

## 状态

Accepted

## 背景

服务需要从源码构建为 OCI 镜像。平台需要一个构建触发和管理机制。

核心诉求：
- 构建逻辑参数化（传服务名、分支、Dockerfile 路径）
- 构建状态可跟踪
- 与 K8s 生态一致

## 决策

使用 Argo Workflows 作为构建执行引擎。
平台触发 Argo Workflow（参数化传入服务信息），跟踪执行状态。
构建完成后回调平台注册版本。

构建模板（WorkflowTemplate）由平台管理，包含标准构建步骤：
git clone → build → docker build → push to Harbor。

## 后果

### 正面
- K8s 原生，与 Argo CD 生态一致
- 构建逻辑声明式定义，可版本管理
- 平台不碰构建细节，只管触发和跟踪
- 支持构建步骤并行化

### 负面
- 引入 Argo Workflows 组件，增加运维复杂度
- 构建环境配置（Docker-in-Docker / Kaniko）需要额外处理
- Workflow YAML 维护成本

### 中性
- 构建模板可按服务类型分类（Go/Java/Node 等）

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 对接 GitLab CI | 可后续扩展，但优先用 K8s 原生方案 |
| 对接 Jenkins | 过重，与现代 K8s 体系不搭 |
| 平台自建构建引擎 | 构建不是平台核心价值，不应重复造轮子 |
| Tekton | 功能与 Argo Workflows 类似，但 Argo 生态更统一 |
