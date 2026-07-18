---
name: dev-workflow
description: "自主开发编排器：从 Issue 到 PR 合并的全闭环流程。串联 issue-track → developer → 门禁 → code-review → PR → CI → 合并，定义何时自主推进、何时停下等人。"
---

# Dev-Workflow: 自主开发编排器

## Quick Reference（核心规则速查）

| 规则 | 内容 |
|------|------|
| commit type | `feat`/`fix`/`docs`/`refactor`/`test`，**禁止 `chore`** |
| commit 格式 | `type(scope): subject` |
| PR 标签 | AI 创建的 PR 必须标注 `🤖 ai-generated` |
| PR body | 必须包含 `Closes #N` + 验收条件 checklist |
| 门禁 | `make lint && make test && make build`（改了 Go）/ `make web-lint && make web-test && make web-build`（改了前端）|
| 合并条件 | CI 全绿 + 门禁通过 + review 通过 |
| 合并方式 | `gh pr merge --squash --delete-branch` |
| 重试上限 | 任何自动修复最多 3 轮，超过则停下求助 |
| 分支保护 | main 已开分支保护，CI 不过无法合并 |

---

## 定位

**编排层**，不替代现有 skill，而是把它们串成闭环：

```
issue-track (理解任务) → developer (编码) → 门禁 → code-review (评审) → PR → CI → 合并
```

每个环节的具体操作规范由对应 skill 定义，本 skill 只负责**编排顺序、决策规则、状态流转**。

---

## 编排铁律

> **主 agent 必须严格按阶段顺序执行，不可跳过任何"必做"阶段。**
>
> "必做"阶段的标志是决策规则表中标记 ✅ 或 🔄 的动作。只有标记"可跳过"的阶段允许跳过。
>
> **违反编排顺序是最严重的错误**——比代码 bug 更严重，因为它破坏了流程可信度。
>
> 具体而言：
> 1. **主 agent 不可将编排责任下放给子 agent**。主 agent 负责决定执行哪些阶段、什么顺序。子 agent 只负责执行被分配的 task。
> 2. **子 agent 不知道 dev-workflow 的存在**。子 agent 是 isolated context，只看到 task 文本。所有流程编排指令必须由主 agent 显式下达。
> 3. **如果主 agent 自己执行开发**：按阶段 0→10 顺序走，每步不漏。
> 4. **如果主 agent spawn 子 agent 执行开发**：主 agent 必须在子 agent 完成开发后、push 前，自己执行门禁检查（阶段 3）和 code review（阶段 4），不可下放给子 agent。

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
- 复杂变更（≥ 5 文件或跨模块）→ spawn 子 agent 处理（子 agent 只负责编码 + 自测，不负责 review）

**决策**：遇到需求不明确 / 设计有歧义 → 停下，列出选项让用户决策。

---

### 阶段 3：门禁检查

编码完成，push 前执行门禁。所有门禁统一通过 Makefile 执行：

```bash
make lint        # 后端静态分析（golangci-lint）
make test        # 后端测试 + 竞态检测（go test -race）
make build       # 后端编译
make web-test    # 前端测试
make web-build   # 前端构建
make web-lint    # 前端 lint
```

**适用规则**：
- 改了 Go 代码：必须通过 `make lint && make test && make build`
- 改了前端代码：必须通过 `make web-lint && make web-test && make web-build`
- 全栈改动：以上全部通过
- 改了 Ent schema：追加 `make ent-gen`，确保生成代码已提交（`git diff --exit-code`）
- 纯 docs 类型跳过门禁（无代码变更）

**决策规则**：
- 全部通过 → 进入阶段 4
- Lint / 编译失败 → **自主修复**，重新检查，最多重试 3 轮
- 测试失败 → **自主修复**，重新检查，最多重试 3 轮
- 3 轮仍未通过 → 停下，展示失败信息让用户介入

> **注意**：如果阶段 2 由子 agent 执行，门禁检查由**主 agent** 在子 agent 完成后执行，不在子 agent task 中下放。子 agent task 中可要求其自行基本编译测试，但正式门禁由主 agent 验证。

---

### 阶段 4：本地 Code Review（feat/fix 必做）

门禁通过后，push 前进行本地评审：

遵循 `code-review` skill：spawn 6 个专业子 agent 并行审查 `git diff origin/main`。

**这是必做步骤，不可跳过，不可下放给开发子 agent。**

> **为什么不能下放？**
> 1. 开发子 agent 是 isolated context，不知道 code-review skill 的存在
> 2. Code review 的价值在于独立视角——开发 agent 自审存在盲区
> 3. 主 agent 需要汇总 6 份报告做决策，这是编排职责

**决策规则**：
- 全部 APPROVED → 进入阶段 5
- 有 🔴 Critical 或 🟡 Major → **自主修复**，重新评审（最多 3 轮）
- 3 轮后仍有 Critical → 停下，展示报告让用户介入
- 只有 🔵 Minor → 可直接 push，在 PR 中附注已知改进建议
- 纯 docs 类型可跳过（无代码逻辑变更）

---

### 阶段 5：提交 + Push + 创建 PR

**提交前 checklist（逐条确认）：**
□ commit type 是 `feat`/`fix`/`docs`/`refactor`/`test`（禁止 `chore`）
□ commit message 格式：`type(scope): subject`
□ PR body 包含 `Closes #<N>`
□ 准备好创建 PR 后执行 `gh pr edit <PR#> --add-label "🤖 ai-generated"`

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

### 阶段 8：确认门禁通过后合并

**确认 CI 和门禁全通过后，自行合并。** 检查清单：

```bash
gh pr checks <PR#>    # CI 必须全绿
make lint             # 后端静态分析
make test             # 后端测试 + 竞态检测
make build            # 后端编译
make web-lint         # 前端 lint（改了前端时）
make web-test         # 前端测试（改了前端时）
make web-build        # 前端构建（改了前端时）
```

全部适用的检查通过后：

```
gh pr merge <PR#> --squash --delete-branch
git worktree remove .worktree/<type>/<N>-<desc>
```

**决策**：
- 门禁全通过 → 自行合并 → 清理 worktree → 进入阶段 9
- 门禁失败 → 修复后重试（最多 3 轮）
- 3 轮后仍失败 → 停下，展示失败信息让用户介入

> 合并后通知用户结果，不需要等待确认。

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

### 执行模型

**关键设计：开发并行，review 串行。**

子 agent 只负责阶段 0-2（理解 Issue + worktree + 编码 + 基本自测）。门禁、review、push、PR 由主 agent 串行处理。

```
阶段 0-2（并行）：
主 Agent
 ├─ 子 Agent A (worktree feat/3-xxx) → 编码 + 自测 → 产出代码
 ├─ 子 Agent B (worktree feat/4-yyy) → 编码 + 自测 → 产出代码
 └─ 子 Agent C (worktree feat/5-zzz) → 编码 + 自测 → 产出代码

阶段 3-5（串行，主 Agent 逐个执行）：
主 Agent → 对 A 做门禁 → review → push → PR
主 Agent → 对 B 做门禁 → review → push → PR
主 Agent → 对 C 做门禁 → review → push → PR

阶段 6-8（可重叠）：
主 Agent → poll 各 PR 的 CI → 逐个 PR review → 统一通知用户
```

> **为什么 review 不并行？**
> 1. 每个 PR 的 review 本身已经并行（6 个维度同时审）
> 2. 主 agent 需要逐个确认 review 结果并做修复决策
> 3. 多 PR 同时 review 会导致主 agent context 膨胀，降低质量

### 子 agent task 规范

给开发子 agent 的 task **必须**包含以下结构（参照下方模板），主 agent 不可在模板外自由发挥：

#### 子 Agent Task 模板

```markdown
## 你的角色
你是 Fleet 平台的 Go 后端工程师。你只负责编码和自测，不负责 code review、push、创建 PR。

## 工作目录
{worktree_path}

## Issue 信息
- Issue #{N}: {title}
- 验收条件：{checklist}

## 技术栈
- Go 1.26 + Echo v4 + Ent ORM + Viper + Redis + zap
- 模块路径：github.com/whg517/fleet
- 分层规范：handler → domain → infra → store

## 你的任务
1. 阅读 Issue 和项目现有代码，理解需求和架构
2. 在 worktree 中编码实现
3. 运行以下自测确保基本质量：
   - `go mod tidy`
   - `golangci-lint run ./...`（修复所有问题）
   - `go test ./...`（确保通过）
   - `go build ./cmd/server/`（确保编译通过）
4. 用规范的 commit message 提交代码：`{type}({scope}): {subject} (#{N})`

## 你的边界（严格遵守）
- ✅ 你可以做：编码、写测试、运行 lint/test/build、提交代码
- ❌ 你不可以做：push 到远程、创建 PR、执行 code review、合并 PR
- ❌ 你不可以做：修改 .gitignore、修改 CI 配置、修改其他 worktree 的代码

## 完成标准
- 所有验收条件对应的功能已实现
- lint 0 issues
- 测试全部通过
- 编译成功
- 代码已 commit（但未 push）
```

> **主 agent 注意事项**：
> 1. 模板中的"你的边界"是铁律，不可删除或弱化
> 2. 子 agent 完成后，主 agent 必须自行验证：`git log --oneline`、`go build`、`go test`
> 3. 然后主 agent 进入阶段 3（门禁）→ 阶段 4（review）→ 阶段 5（push + PR）

### 并行限制

- 最多 **3 个并行**（避免资源竞争和 review 负担）
- 同一 package 下的 Issue 串行处理
- 有共享文件依赖的串行（先合并接口定义 PR，再并行）

### 汇总

所有子 agent 完成开发后，主 agent 逐个执行门禁 → review → push → PR，最后汇总：
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

| 阶段 | 条件 | 动作 | 可跳过？ |
|------|------|------|---------|
| 0 | 依赖未满足 | 🛑 停下，告知用户 | ❌ |
| 2 | 需求不明确 | 🛑 停下，列出选项让用户决策 | ❌ |
| 3 | 门禁失败（≤3 轮） | 🔄 自主修复重试 | ❌（docs 可跳） |
| 3 | 门禁失败（>3 轮） | 🛑 停下，展示失败信息 | ❌ |
| 4 | Review 有 Critical/Major（≤3 轮） | 🔄 自主修复重试 | ❌（docs 可跳） |
| 4 | Review 有 Critical（>3 轮） | 🛑 停下，展示报告 | ❌ |
| 4 | Review 全 APPROVED | ✅ 自主推进到 push | — |
| 6 | CI 失败（≤3 轮） | 🔄 自主修复重试 | ❌ |
| 6 | CI 失败（>3 轮） | 🛑 停下，展示 CI 日志 | ❌ |
| 7 | PR Review 有问题（≤3 轮） | 🔄 自主修复重试 | ❌ |
| 7 | PR Review 全 APPROVED | ✅ 自主推进到通知用户 | — |
| 8 | 门禁全通过 | ✅ 自行合并，通知用户结果 | — |
| 8 | 门禁失败（>3 轮） | 🛑 停下，展示失败信息 | ❌ |
| 9 | 合并完成 | ✅ 自主清理 worktree | ❌ |
| 10 | 有就绪 Issue | 🛑 询问用户是否继续 | — |

**核心原则：编码、测试、评审、修复、合并全部自主完成，但必须确保门禁全通过才能合并。遇到需求不明确或 3 轮修复仍失败时停下求助。**

---

## 与现有 Skill 的衔接

| 阶段 | 调用 Skill | 作用 | 执行者 |
|------|-----------|------|--------|
| 0 | issue-track | 理解 Issue、检查依赖 | 主 agent |
| 1 | developer | Worktree 创建 | 主 agent |
| 2 | developer | 编码 | 主 agent 或子 agent |
| 3 | developer（门禁部分） | Lint / Test / Build 检查 | 主 agent |
| 4 | code-review（本地模式） | Push 前自查 | 主 agent |
| 5 | developer（PR 部分） | Commit / Push / PR 创建 | 主 agent |
| 7 | code-review（PR 模式） | PR 评审 | 主 agent |
| 9 | developer（清理部分） | Worktree 清理 | 主 agent |

> **注意**：阶段 3-9 全部由主 agent 执行。只有阶段 2（编码）可以委托给子 agent。

每个阶段的具体操作规范以对应 skill 的 SKILL.md 为准。本 skill 只负责编排顺序和决策。

---

## 安全规则

1. **门禁全通过才能合并 PR**（阶段 8 确认 CI + 本地检查全绿后自行合并）
2. **主 agent 不可跳过必做阶段**（阶段 3 门禁、阶段 4 review 是 feat/fix 必做）
3. **编排职责不可下放**（子 agent 只编码，主 agent 负责门禁 + review + push + PR）
4. **破坏性操作先问**（删分支、force push、reset main）
5. **敏感数据不入库**
6. **AI 生成的 PR 标注** `🤖 ai-generated`
7. **Worktree 隔离**：并行时各子 agent 只在自己 worktree 操作
8. **重试上限**：任何自动修复最多 3 轮，超过则停下求助
