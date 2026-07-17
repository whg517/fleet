---
name: issue-track
description: "GitHub Issue 驱动的任务追踪与开发流程管理：拆分 Issue、认领开发、PR 提交、Review 合并、进度跟踪。"
---

# Issue-Track: GitHub Issue 驱动开发工作流

## 触发场景

- 用户要求拆分 Milestone / 需求为 Issue
- 用户要求开始开发某个 Issue
- 用户要求并行处理多个 Issue
- 用户要求查看进度 / 合并 PR

## 前置检查

每次执行前先确认：

1. `gh auth status` — 确保 GitHub CLI 已认证
2. 确认目标仓库（从 AGENTS.md 或用户指定）
3. `git fetch origin && git checkout main && git pull` — 确保本地代码最新

---

## 工作流 1：拆分 Milestone 为 Issue

**触发词：** "拆 M1" / "把 M2 拆成 Issue" / "创建开发任务"

### 步骤

1. 读取项目 `docs/REQUIREMENTS.md`，找到对应 Milestone 的需求项
2. 读取 `docs/user-stories/` 中相关用户故事
3. 按**可独立交付**的粒度拆分（一个 Issue = 1-3 天工作量）

### 拆分原则

- 每个 Issue 必须有**用户故事**（As-I-Want-So 格式）
- 每个 Issue 必须有**明确的验收条件**（可复制的 checklist）
- 有依赖关系的拆成独立 Issue，Body 里标注 `Blocked by #N`
- 纯文档变更单独拆，用 `docs` 标签
- 基础设施类（脚手架、CI 配置）单独拆

### Issue 格式

标题：`[模块] 简述`

Body 参考 `assets/issue-body-template.md`，必须包含：
- 需求 ID
- 用户故事（As-I-Want-So）
- 验收条件（checklist）
- 依赖（如有）
- 关联里程碑

### 批量创建

```bash
gh issue create \
  --title "[deploy] 实现 Argo CD Application 管理 + 状态同步" \
  --body-file /tmp/issue-body.md \
  --label "feat,P0,M2,deploy" \
  --milestone "M2: 核心部署链路"
```

创建后输出 Issue 清单供用户确认优先级。

---

## 工作流 2：认领 Issue 并开发

**触发词：** "做 Issue #3" / "开始 #5" / "开发 D-01"

### 步骤

1. 确认 Issue 内容：`gh issue view N`
2. **检查依赖**：确认 `Blocked by` 的 Issue 已 Done
3. **Git Worktree 工作流**（所有开发都走 worktree）：

```bash
git fetch origin
git worktree add .worktree/feat/N-short-desc -b feat/N-short-desc origin/main
cd .worktree/feat/N-short-desc
```

4. 开发（直接写代码 or spawn 子 agent）
5. 提交（遵守 commit 规范）：`git commit -m "feat(scope): subject"`
6. 推送：`git push -u origin feat/N-short-desc`
7. 创建 PR：`gh pr create --title "..." --body-file ...`
   - PR Body 必须包含 `Closes #N` + 验收条件 checklist
   - PR Body 参考 `assets/pr-body-template.md`
8. 标注 AI 生成：`gh pr edit --add-label "🤖 ai-generated"`

---

## 工作流 3：并行开发多个 Issue

**触发词：** "并行做 #3 #4 #5"

### 前置检查

1. 确认 Issue 之间的**依赖关系**（查看 `Blocked by`）
2. 确认无硬依赖冲突（A Blocked by B，不能同时做）
3. 软依赖的 Issue 先沟通接口/数据格式

### 步骤

1. 检查文件冲突可能性（同一 package 下的 Issue 需串行）
2. 为每个 Issue 创建独立 worktree：

```bash
git worktree add .worktree/feat/3-xxx -b feat/3-xxx origin/main
git worktree add .worktree/feat/4-yyy -b feat/4-yyy origin/main
git worktree add .worktree/feat/5-zzz -b feat/5-zzz origin/main
```

3. 为每个 Issue spawn 子 agent（`sessions_spawn mode=run`）
   - 子 agent task 中指定 worktree 路径作为 `cwd`
   - 子 agent 只在自己 worktree 中操作
4. 各子 agent 完成后独立创建 PR
5. 汇总结果

### 注意

- 子 agent 只在自己 worktree 操作，不切 main，不碰其他 worktree
- 有共享文件依赖的改为串行（先合并接口定义 PR，再并行开发）
- 同时创建的 PR 应互不依赖

---

## 工作流 4：Review 与合并

**触发词：** "看看 #5 的 PR" / "合并 #3" / "review #7"

### 查看 PR

```bash
gh pr view N
gh pr diff N
gh pr checks N
```

### PR Review（如项目有 pr-review skill，通过子 agent 执行）

1. spawn 子 agent 执行 PR Review（详见 pr-review skill）
2. 子 agent 输出 review 报告
3. 根据 review 结果决定：approve / request changes

### 处理 Review 意见

1. 读取 review comments
2. 在对应 worktree 中修改代码
3. push 更新（PR 自动更新）
4. 如 worktree 已清理，重新创建：

```bash
git worktree add .worktree/feat/N-short-desc feat/N-short-desc
```

### 验收闭环

1. **Reviewer 逐条核对** PR 描述中的验收条件 checklist
2. 所有验收条件 ✅ 打勾后，Reviewer approve
3. 用户确认后执行 Squash Merge：

```bash
gh pr merge N --squash --delete-branch
```

4. 合并后清理 worktree：

```bash
cd /path/to/platform  # 回到主工作区
git worktree remove .worktree/feat/N-short-desc
git branch -d feat/N-short-desc
```

5. Issue 通过 `Closes #N` 自动关闭

> **关键规则：AI 不自行合并 PR。** 必须用户明确说"合并"后才执行。
> **验收条件不可只看 CI 绿灯**。CI 只验证编译和测试，功能验收需要 Reviewer 确认。

---

## 工作流 5：进度跟踪

**触发词：** "进度" / "看板" / "什么状态"

### Issue 状态

```
Backlog → Todo → In Progress → Review → Done
```

```bash
# 查看 Milestone 下所有 Issue
gh issue list --milestone "M1" --state all

# 查看打开的 PR
gh pr list --state open

# 查看 worktree 列表
git worktree list
```

输出汇总表格：

| Issue | 标题 | 状态 | Worktree | PR |
|-------|------|------|----------|-----|
| #3 | [deploy] Argo CD CRUD | In Progress | .worktree/feat/3-xxx | - |
| #4 | [auth] OIDC 回调 | Review | - | #7 |

### 清理检查

- 已合并但未清理的 worktree
- 已关闭但未删除的远程分支

---

## Git 规范

### 分支命名

`type/issue-number-short-desc`（feat/fix/docs/refactor/test），禁用 chore

### Commit 规范

`type(scope): subject`，禁用 `chore`

### 合并方式

Squash merge，合并后删分支

### Worktree 规则

- 所有开发基于 worktree，禁止在主工作区直接切分支
- worktree 目录 `.worktree/`（已在 `.gitignore`）
- 每个 Issue 一个 worktree，完成后及时清理

---

## 安全规则

1. **AI 不自行合并 PR**
2. **破坏性操作先问**（删分支、force push、重置 main）
3. **敏感数据不入库**（Token、密钥、kubeconfig）
4. **AI 生成的 PR 标注** `🤖 ai-generated`
5. **Worktree 隔离**：子 agent 只在自己 worktree 中操作
