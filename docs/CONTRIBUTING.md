# 协作开发规范

## Git Worktree 工作流

使用 git worktree 进行并行开发，所有工作目录放在 `.worktree/` 下。

### 目录结构

```
platform/
├── .worktree/
│   ├── feat/deploy-module/       # 功能分支
│   ├── fix/auth-bug/             # 修复分支
│   └── docs/api-design/          # 文档分支
├── docs/                         # 主分支工作区
└── ...
```

### 分支命名规范

格式：`type/name`

| type | 用途 | 示例 |
|------|------|------|
| `feat` | 新功能开发 | `feat/deploy-module` |
| `fix` | Bug 修复 | `fix/oidc-token-refresh` |
| `docs` | 文档更新 | `docs/api-design` |
| `refactor` | 代码重构 | `refactor/deployment-domain` |
| `test` | 测试相关 | `test/e2e-deployment` |

> **禁用 `chore` 类型。**

---

## 开发流程

### 1. 创建新分支

```bash
# 从 main 创建 worktree
git worktree add .worktree/feat/xxx -b feat/xxx main

# 进入工作目录开发
cd .worktree/feat/xxx
```

### 2. 同步主分支最新代码

每次开始开发前，必须检查并同步 main 最新代码：

```bash
git fetch origin
git rebase main
```

> 如果 rebase 出现冲突，解决后 `git rebase --continue`。

### 3. 提交规范

```bash
git commit -m "<type>(<scope>): <subject>"
```

格式：`type(scope): subject`

示例：

```
feat(deploy): 实现 Argo CD Application CRUD
fix(auth): 修复 OIDC token 刷新失败问题
docs(api): 补充部署 API 接口文档
refactor(config): 重构配置变更锁逻辑
test(deploy): 添加部署回滚端到端测试
```

规则：
- type 必须是 `feat` / `fix` / `docs` / `refactor` / `test` 之一
- **禁用 `chore`**
- scope 为模块名（deploy / auth / config / cluster / audit 等）
- subject 简明描述变更内容，中文或英文均可
- 一个 commit 做一件事，不要混合多个不相关的变更

### 4. 合并到主分支

开发完成后，使用 squash 合并：

```bash
cd <主仓库根目录>

# 确保在 main 分支
git checkout main
git pull origin main

# Squash 合并
git merge --squash feat/xxx

# 提交（用一个规范的 commit message）
git commit -m "feat(deploy): 实现 Argo CD Application CRUD + 状态同步"

# 清理 worktree
git worktree remove .worktree/feat/xxx
git branch -D feat/xxx
```

---

## 要点总结

| 规则 | 要求 |
|------|------|
| 工作目录 | `.worktree/type/name` |
| 分支命名 | `type/name` |
| 开发前 | `git rebase main` 同步最新代码 |
| 提交规范 | `type(scope): subject` |
| 合并方式 | `git merge --squash` |
| 禁用类型 | `chore` |
| 合并后 | 清理 worktree 和分支 |
