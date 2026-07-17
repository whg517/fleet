# AGENTS.md - 服务管理平台

## 项目概述

服务管理平台：覆盖微服务全生命周期（构建打包 → 版本管理 → 多环境部署 → 配置变更 → 监控 → 扩缩容 → 服务下线），基于 K8s + Argo CD 平台托管模式。

## 技术栈

- **后端**：Go（单体，按 domain 分包）
- **前端**：Next.js SPA（React + TypeScript）
- **数据库**：PostgreSQL + Redis
- **构建**：Argo Workflows
- **部署**：Argo CD（K8s）+ Ansible（物理节点）
- **镜像仓库**：Harbor
- **认证**：OIDC（企业 SSO）
- **监控**：Prometheus + AlertManager

## 目录结构

```
platform/
├── docs/                       # 项目文档
│   ├── REQUIREMENTS.md         # 需求文档
│   ├── ARCHITECTURE.md         # 架构文档
│   ├── CONTRIBUTING.md         # 协作开发规范
│   ├── adr/                    # 技术决策记录
│   └── user-stories/           # 用户故事（运维/开发/PM）
├── cmd/                        # Go 入口
│   └── server/
├── internal/                   # Go 内部实现
│   ├── api/                    # HTTP/WS handler
│   ├── domain/                 # 业务逻辑
│   ├── infra/                  # 基础设施对接
│   └── store/                  # 持久层
├── web/                        # Next.js 前端
└── .worktree/                  # worktree 开发目录（gitignore）
```

## 开发规范

详见 [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md)

要点：
- **Git Worktree 工作流**：每个 Issue 在独立 worktree 中开发，多 agent 并行互不干扰
  ```bash
  git worktree add .worktree/feat/42-xxx feat/42-xxx
  cd .worktree/feat/42-xxx
  # 完成后
  git worktree remove .worktree/feat/42-xxx
  ```
- 分支命名：`type/issue-number-short-desc`（feat/fix/docs/refactor/test），禁用 chore
- 开发前从 main 最新代码切出分支
- 合并用 `gh pr merge <N> --squash --delete-branch`
- 提交格式：`type(scope): subject`
- Issue 状态流转、依赖管理、验收闭环详见 CONTRIBUTING.md §6

## 规则

- 代码和证据说话
- 破坏性操作先问
- 保护隐私数据
- 文档变更走正式 PR 流程
