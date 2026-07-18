---
name: developer
description: "Fleet 项目 Git 开发工作流：fetch-before-read、Worktree 分支管理、提交规范、PR 全生命周期。操作仓库时使用。"
---

# Developer — Fleet 项目 Git 工作流

## 触发场景

- 操作仓库前（查看状态、读文件、切分支、创建 PR 等）
- 开始开发某个 Issue
- 查看 PR / Issue 状态
- 同步本地 main 分支

## 铁律：先 Fetch 再判断

**任何 git 查看操作前，必须先 `git fetch origin`。**

本地 `origin/main` 引用是缓存，可能过期。`git status` 显示 "up to date with origin/main" 只代表和**本地缓存的** origin/main 一致，不代表和**远程真实状态**一致。

### 必须执行的操作顺序

```
git fetch origin            ← 第一步，永远第一步
git status                  ← 然后才能看状态
git log origin/main         ← 然后才能看远程历史
git show origin/main:path   ← 然后才能读远程文件
```

### 何时 fetch

- 每次开始仓库相关任务时
- 查看分支状态前
- 创建新分支前
- 读远程文件内容前
- 判断 PR 是否合并前
- 合并 PR 前

不需要重复 fetch 的情况：同一轮操作中已 fetch 过，且间隔 < 60 秒。

---

## 仓库信息

- **路径**：`projects/platform/`
- **远程**：`https://github.com/whg517/fleet.git`

---

## Worktree 工作流

每个 Issue 在独立 worktree 中开发，禁止在主工作区直接切分支。

### 创建 worktree

```bash
cd projects/platform
git fetch origin
git worktree add .worktree/<type>/<issue#>-<desc> -b <type>/<issue#>-<desc> origin/main
```

示例：

```bash
git worktree add .worktree/feat/42-argocd-app -b feat/42-argocd-app origin/main
```

### 在 worktree 中工作

```bash
cd .worktree/feat/42-argocd-app
# 编码、测试、提交
```

### 清理 worktree

PR 合并后：

```bash
cd projects/platform
git worktree remove .worktree/feat/42-argocd-app
git branch -d feat/42-argocd-app
```

> `.worktree/` 已在 `.gitignore` 中。

---

## 分支命名

格式：`<type>/<issue#>-<short-desc>`

| type | 用途 | 示例 |
|------|------|------|
| `feat` | 新功能 | `feat/42-argocd-app` |
| `fix` | Bug 修复 | `fix/58-oidc-token` |
| `docs` | 文档变更 | `docs/19-developer-skill` |
| `refactor` | 重构 | `refactor/55-deploy-lock` |
| `test` | 测试 | `test/72-deploy-e2e` |

> 禁用 `chore`。

---

## 提交规范

```
type(scope): subject
```

示例：

```
feat(deploy): 实现 Argo CD Application CRUD
fix(auth): 修复 OIDC token 刷新失败
docs(skills): 新增 developer skill
```

多个 commit 可保留，squash merge 时压缩为一个。

---

## PR 流程

```bash
# 1. fetch 最新
git fetch origin

# 2. 推送分支
git push -u origin <type>/<issue#>-<desc>

# 3. 创建 PR
gh pr create --title "<type>(<scope>): <subject>" \
  --body "## 关联 Issue

Closes #<N>

## 变更说明
...

## 验收条件
- [ ] ..."

# 4. 合并（确认 Review 通过后）
gh pr merge <N> --squash --delete-branch
```

### AI 创建的 PR

标注 `🤖 ai-generated` 标签，提醒 Reviewer 重点关注。

### 合并权限

**AI 不自行合并 PR**，需用户确认后执行 merge。

---

## 本地 main 同步

```bash
cd projects/platform
git fetch origin
git checkout main
git pull --ff-only origin main
```

如果 main 有未提交改动，先 stash 或确认是否需要迁移到 worktree。

---

## 常见错误防范

| 场景 | 错误做法 | 正确做法 |
|------|---------|---------|
| 查看仓库状态 | 直接 `git status` | 先 `git fetch` 再 `git status` |
| 读远程文件 | `git show origin/main:file`（缓存可能过期） | 先 `git fetch` 再读 |
| 创建分支 | 基于本地 main 切分支 | 先 `fetch`，基于 `origin/main` 创建 worktree |
| 判断 PR 是否合并 | 只看本地分支 | 先 `fetch`，查 `gh pr list --state all` |
| 在主工作区开发 | 直接在 main 上改代码 | 用 worktree 在分支上开发 |
| 新建文件/目录 | 拍脑袋选位置 | 先看项目已有结构，遵循约定 |
