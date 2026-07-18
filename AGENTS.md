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
│   ├── REQUIREMENTS.md         # 需求清单
│   ├── ARCHITECTURE.md         # 架构设计
│   ├── DESIGN-SPEC.md          # 交互规范 / 术语对照
│   ├── DEVELOPMENT.md          # 开发指南（环境准备 / 启动 / 规范）
│   ├── CONTRIBUTING.md         # 协作开发规范（PR 流程）
│   ├── argocd-benchmark-plan.md # Argo CD 性能验证方案
│   ├── adr/                    # 架构决策记录
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

## 本地开发流程

所有开发工作（无论单人还是多人、单 Issue 还是并行）都基于 **Git Worktree**：

```bash
# 1. 从 main 创建分支 + worktree
git fetch origin
git worktree add .worktree/feat/42-xxx -b feat/42-xxx origin/main

# 2. 在 worktree 中开发
cd .worktree/feat/42-xxx
# ... 编码、测试、提交 ...

# 3. 提交前门禁检查
make lint && make test
# 改了前端：cd web && pnpm build && cd ..

# 4. 提交 + 推送 + 创建 PR
git add . && git commit -m "type(scope): subject"
git push -u origin feat/42-xxx
gh pr create --title "feat(scope): subject" \
  --label "🤖 ai-generated"

# 5. PR 合并后清理
git worktree remove .worktree/feat/42-xxx
git branch -d feat/42-xxx
```

**规则**：
- 禁止在主工作区直接切分支开发，所有分支开发走 worktree
- worktree 目录 `.worktree/` 已在 `.gitignore` 中
- 每个 Issue 一个 worktree，完成后及时清理

## 开发指南

详见 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)，涵盖：
- 环境准备（工具链、依赖服务）
- 快速启动（docker compose + go run + pnpm dev）
- 后端/前端开发规范
- 数据库与 Schema 管理（Ent + Atlas）
- 测试策略、配置管理、API 设计规范
- 错误处理、日志规范、故障排查

## 协作规范

详见 [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md)，要点：
- 分支命名：`type/issue-number-short-desc`（feat/fix/docs/refactor/test），禁用 chore
- 提交格式：`type(scope): subject`
- 合并用 `gh pr merge <N> --squash --delete-branch`
- Issue 依赖与并行管理：`Blocked by #N`，接口先行，分支隔离
- Issue 状态流转与验收闭环详见 CONTRIBUTING.md §6

## 规则

- 代码和证据说话
- 破坏性操作先问
- 保护隐私数据
- 文档变更走正式 PR 流程
