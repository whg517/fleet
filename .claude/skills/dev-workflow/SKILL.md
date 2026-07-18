---
name: dev-workflow
description: "自主开发编排器：从 Issue 到 PR 合并的全闭环流程。串联 issue-track → developer → 门禁 → code-review → PR → CI → 合并，定义何时自主推进、何时停下等人。"
---

# Dev-Workflow: 自主开发编排器

## 定位

**编排层**，不替代现有 skill，而是把它们串成闭环：

```
issue-track (理解任务) → developer (编码) → 门禁 → code-review (评审) → PR → CI → 合并
```

每个环节的具体操作规范由对应 skill 定义，本 skill 只负责**编排顺序、决策规则、状态流转**。

---

## 触发场景

- 用户说"做 Issue #N" / "推进 #N" / "开发 #N"
- 用户说"并行做 #3 #4 #5"
- code-review 通过后自动进入下一步
- PR CI 通过后自动进入评审环节

---

## 前置检查

每次启动工作流时先执行：

1. `gh auth status` — 确认 GitHub CLI 已认证
2. `git fetch origin` — 同步远程状态
3. `git worktree list` — 检查是否有未清理的 worktree
4. 确认目标 Issue 状态为 OPEN，未被他人认领

---

## 全流程（单 Issue）

### 阶段 0：理解 Issue

```
gh issue view <N>
```

提取关键信息：
- **需求摘要**：Issue 标题 + Body 核心描述
- **验收条件**：Body 中的 checklist
- **依赖**：`Blocked by #M`（如有，确认 #M 已 Done）
- **标签**：P0/P1/P2 + 里程碑 + 模块

**决策**：依赖未满足 → 停下，告知用户先完成依赖 Issue。

---

### 阶段 1：准备工作环境

遵循 `developer` skill 的 worktree 工作流：

```bash
cd projects/platform
git fetch origin
git worktree add .worktree/<type>/<N>-<desc> -b <type>/<N>-<desc> origin/main
cd .worktree/<type>/<N>-<desc>
```

类型由 Issue 标签决定：`feat` / `fix` / `docs` / `refactor` / `test`。

**决策**：worktree 创建失败（分支已存在 / 路径冲突）→ 检查是否已有进行中的工作，询问用户是恢复还是重开。

---

### 阶段 2：编码实现

在 worktree 中编码。遵循项目规范：
- 后端：Go 四层架构（handler → domain → infra → store）
- 前端：React + TypeScript
- 参照 `docs/ARCHITECTURE.md` 和 `docs/DEVELOPMENT.md`

**子 agent 策略**：
- 简单变更（< 5 文件）→ 主 agent 直接写
- 复杂变更（≥ 5 文件或跨模块）→ spawn 子 agent 并行处理独立模块

**决策**：遇到需求不明确 / 设计有歧义 → 停下，列出选项让用户决策。

---

### 阶段 3：门禁检查

编码完成，push 前执行门禁：

| # | 检查项 | 命令 | 适用条件 |
|---|--------|------|----------|
| 1 | 后端 Lint | `make lint` | 改了 Go 代码 |
| 2 | 后端测试 | `make test` | 改了 Go 代码 |
| 3 | Ent 代码生成 | `make ent-gen` | 改了 schema |
| 4 | DB 迁移 | `make db-migrate` | 改了 schema |
| 5 | 前端构建 | `cd web && npm run build` | 改了前端 |
| 6 | 服务启动 | `make dev-server` | 改了后端 |

**决策规则**：
- 全部通过 → 进入阶段 4
- Lint / 编译失败 → **自主修复**，重新检查，最多重试 3 轮
- 测试失败 → **自主修复**，重新检查，最多重试 3 轮
- 3 轮仍未通过 → 停下，展示失败信息让用户介入
- 纯 docs 类型跳过门禁（无代码变更）

---

### 阶段 4：本地 Code Review

门禁通过后，push 前进行本地评审（feat/fix 类型必做，docs 类型可跳过）：

遵循 `code-review` skill：spawn 6 个专业子 agent 并行审查 `git diff origin/main`。

**决策规则**：
- 全部 APPROVED → 进入阶段 5
- 有 🔴 Critical 或 🟡 Major → **自主修复**，重新评审（最多 3 轮）
- 3 轮后仍有 Critical → 停下，展示报告让用户介入
- 只有 🔵 Minor → 可直接 push，在 PR 中附注已知改进建议

---

### 阶段 5：提交 + Push + 创建 PR

```bash
git add -A
git commit -m "<type>(<scope>): <subject>"
git push -u origin <type>/<N>-<desc>
```

创建 PR：

```bash
gh pr create \
  --title "<type>(<scope>): <subject>" \
  --body-file /tmp/pr-body-<N>.md
gh pr edit <PR#> --add-label "🤖 ai-generated"
```

PR Body 必须包含：
- `Closes #<N>`
- 变更说明
- 验收条件 checklist（逐条对照 Issue）

**决策**：PR 创建成功 → 自动进入阶段 6（无需停下）。

---

### 阶段 6：等待 CI 通过

PR 创建后主动 poll CI 状态：

```bash
gh pr checks <PR#>
```

- CI 全绿 → 进入阶段 7
- CI 有失败 → **自主修复**（回到 worktree 改代码 → 门禁 → push），最多 3 轮
- 3 轮后仍失败 → 停下，展示 CI 日志让用户介入

**Poll 策略**：首次等 30s 检查，之后每 60s 检查一次，最多等 10 分钟。

---

### 阶段 7：PR Code Review

CI 通过后，进行 PR 评审：

遵循 `code-review` skill 的 PR 评审模式：spawn 6 个专业子 agent 审查 `gh pr diff <PR#>`。

**决策规则**：
- 全部 APPROVED → 进入阶段 8（通知用户合并）
- 有问题 → **自主修复**（worktree 改 → 门禁 → push → 等 CI → 再评审），最多 3 轮
- 3 轮后仍有 Critical → 停下，发布 review 报告到 PR comment，通知用户

---

### 阶段 8：通知用户合并（唯一需要人介入的点）

**在这里停下。** 向用户报告：

```
✅ Issue #<N> 已完成开发，PR #<PR#> 已就绪：

- CI：全绿 ✅
- Code Review：APPROVED ✅
- 验收条件：全部满足 ✅

确认合并请回复"合并 #<PR#>"。
```

**决策**：
- 用户说"合并" → `gh pr merge <PR#> --squash --delete-branch` → 清理 worktree → 进入阶段 9
- 用户要求修改 → 回到对应阶段
- 用户拒绝 → 停下，询问后续处理方式

---

### 阶段 9：合并后清理

```bash
cd projects/platform
git worktree remove .worktree/<type>/<N>-<desc>
git branch -d <type>/<N>-<desc>
git fetch origin
git checkout main
git pull --ff-only origin main
```

---

### 阶段 10：检查后续 Issue

合并完成后，主动检查是否有可继续的 Issue：

```bash
gh issue list --label "P0" --state open
gh issue list --label "P1" --state open
```

**决策**：
- 有就绪的下一个 Issue → 询问用户是否继续
- 无就绪 Issue → 报告完成，等待指示

---

## 多 Issue 并行策略

### 前提条件

- Issue 之间**无依赖关系**（A 不 Blocked by B）
- Issue 涉及**不同模块/文件**（无硬冲突）
- 每个 Issue 独立 worktree

### 执行方式

为每个 Issue spawn 一个子 agent，各自在独立 worktree 中执行完整的 dev-workflow 流程：

```
主 Agent
 ├─ 子 Agent A (worktree feat/3-xxx) → 全流程 → PR #1
 ├─ 子 Agent B (worktree feat/4-yyy) → 全流程 → PR #2
 └─ 子 Agent C (worktree feat/5-zzz) → 全流程 → PR #3
```

每个子 agent 的 task 中指定：
- `cwd`：对应 worktree 路径
- Issue 编号
- 明确的文件操作范围（避免交叉）

### 并行限制

- 最多 **3 个并行**（避免资源竞争和 review 负担）
- 同一 package 下的 Issue 串行处理
- 有共享文件依赖的串行（先合并接口定义 PR，再并行）

### 汇总

所有子 agent 完成后，主 agent 汇总：
- 各 PR 状态
- 各 PR review 结果
- 统一通知用户确认合并

---

## 中断恢复

### 状态记录

每个 Issue 的开发进度记录在 worktree 目录下的 `.dev-progress.md`：

```markdown
# Dev Progress — Issue #<N>

## 基本信息
- Issue: #<N>
- Branch: <type>/<N>-<desc>
- Worktree: .worktree/<type>/<N>-<desc>
- PR: #<PR#>（创建后填入）

## 当前阶段
- [ ] 阶段 0: 理解 Issue
- [ ] 阶段 1: 准备 worktree
- [ ] 阶段 2: 编码实现
- [ ] 阶段 3: 门禁检查
- [ ] 阶段 4: 本地 Code Review
- [ ] 阶段 5: 提交 + Push + PR
- [ ] 阶段 6: CI 通过
- [ ] 阶段 7: PR Code Review
- [ ] 阶段 8: 等待用户合并
- [ ] 阶段 9: 清理
- [ ] 阶段 10: 后续 Issue

## 备注
（重要决策、遇到的问题、跳过的步骤等）
```

每完成一个阶段，更新 checklist。

### 恢复流程

中断后恢复时：

1. 检查 `.dev-progress.md` 确认当前阶段
2. 检查 worktree 是否存在：`git worktree list`
3. 检查 PR 状态：`gh pr view <PR#>`
4. 从断点阶段继续

**如果 worktree 被删 / 分支丢失**：
- 未 push → 从阶段 2 重新开始
- 已 push → 重新创建 worktree，从 push 后的阶段继续

---

## 决策规则汇总

| 阶段 | 条件 | 动作 |
|------|------|------|
| 0 | 依赖未满足 | 🛑 停下，告知用户 |
| 2 | 需求不明确 | 🛑 停下，列出选项让用户决策 |
| 3 | 门禁失败（≤3 轮） | 🔄 自主修复重试 |
| 3 | 门禁失败（>3 轮） | 🛑 停下，展示失败信息 |
| 4 | Review 有 Critical/Major（≤3 轮） | 🔄 自主修复重试 |
| 4 | Review 有 Critical（>3 轮） | 🛑 停下，展示报告 |
| 4 | Review 全 APPROVED | ✅ 自主推进到 push |
| 6 | CI 失败（≤3 轮） | 🔄 自主修复重试 |
| 6 | CI 失败（>3 轮） | 🛑 停下，展示 CI 日志 |
| 7 | PR Review 有问题（≤3 轮） | 🔄 自主修复重试 |
| 7 | PR Review 全 APPROVED | ✅ 自主推进到通知用户 |
| 8 | 等待合并 | 🛑 **停下，等用户确认** |
| 9 | 合并完成 | ✅ 自主清理 worktree |
| 10 | 有就绪 Issue | 🛑 询问用户是否继续 |

**核心原则：编码、测试、评审、修复全部自主完成。唯一需要人介入的是最终合并确认。**

---

## 与现有 Skill 的衔接

| 阶段 | 调用 Skill | 作用 |
|------|-----------|------|
| 0 | issue-track | 理解 Issue、检查依赖 |
| 1 | developer | Worktree 创建 |
| 3 | developer（门禁部分） | Lint / Test / Build 检查 |
| 4 | code-review（本地模式） | Push 前自查 |
| 5 | developer（PR 部分） | Commit / Push / PR 创建 |
| 7 | code-review（PR 模式） | PR 评审 |
| 9 | developer（清理部分） | Worktree 清理 |

每个阶段的具体操作规范以对应 skill 的 SKILL.md 为准。本 skill 只负责编排顺序和决策。

---

## 安全规则

1. **AI 不自行合并 PR**（阶段 8 必须停下等人）
2. **破坏性操作先问**（删分支、force push、reset main）
3. **敏感数据不入库**
4. **AI 生成的 PR 标注** `🤖 ai-generated`
5. **Worktree 隔离**：并行时各子 agent 只在自己 worktree 操作
6. **重试上限**：任何自动修复最多 3 轮，超过则停下求助
