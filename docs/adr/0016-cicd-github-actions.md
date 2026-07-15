# ADR-0016: Fleet 自身 CI/CD 通过 GitHub Actions

## 状态

Accepted

## 背景

Fleet 平台自身需要 CI/CD 流水线来管理代码质量、构建和发布。
平台不应自举（不依赖自身功能来构建自身）。

## 决策

使用 **GitHub Actions** 管理 Fleet 自身的 CI/CD。

## 流水线设计

| 阶段 | 触发 | 内容 |
|------|------|------|
| CI | PR | lint → test → build |
| 构建 | push to main | 构建镜像 → 推 Harbor |
| 发布 | git tag (v*) | 构建正式镜像 → 生成 Helm Chart OCI 制品 → 推 Harbor |

## 后果

### 正面
- 与 GitHub 仓库深度集成，零运维
- 不依赖 Fleet 自身功能
- PR 驱动开发流程契合
- GitHub Actions 生态丰富（Action Marketplace）

### 负面
- 依赖 GitHub 可用性
- 复杂流水线场景不如 Argo Workflows 灵活
- 并发构建有配额限制（免费额度）

### 中性
- CI 和 Fleet 平台部署分离：CI 用 GitHub Actions，部署通过平台管理（Argo CD API）

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| Argo Workflows 自举 | 鸡生蛋问题，平台未就绪前无法使用 |
| GitLab CI | 仓库在 GitHub，迁移成本高 |
| Jenkins | 运维重，与现代 CI 体系不搭 |
