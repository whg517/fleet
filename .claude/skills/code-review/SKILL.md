---
name: code-review
description: "Fleet 代码评审：6 个专业子 agent 并行评审 + 三轮机制。支持本地评审（开发完成后）和 PR 评审（提交后）两种模式。"
---

# Code Review: 多 Agent 并行评审

## 快速调用（从 dev-workflow 编排器调用时看这里）

当 dev-workflow 编排器需要执行 code review 时，按以下步骤操作：

### 本地评审（dev-workflow 阶段 4）

1. 主 agent 进入 worktree 目录
2. 获取 diff 概览：`git diff origin/main --stat`
3. 同时 spawn 6 个子 agent（sessions_spawn, mode=run），每个子 agent 使用下方「子 Agent Task 模板 → 本地评审模板」
4. 等待全部完成（sessions_yield）
5. 汇总 6 份报告，形成决策

### PR 评审（dev-workflow 阶段 7）

1. 获取 PR 信息：`gh pr view N`
2. 同时 spawn 6 个子 agent，每个子 agent 使用下方「子 Agent Task 模板 → PR 评审模板」
3. 等待全部完成（sessions_yield）
4. 汇总 6 份报告，形成决策

> **编排器提醒**：spawn review 子 agent 时，task 中必须包含完整的检查清单（不是引用，是内联完整内容）。review 子 agent 是 isolated context，无法读取本 skill 文件。

---

## 核心设计

**6 个专业子 agent 各司其职，并行评审代码。**

每个子 agent 只负责一个维度，带着该领域的专业 checklist 深入审查。主 agent 汇总 6 份报告，形成统一决策。

```
代码变更
 ├─ 🔍 correctness-reviewer  — 代码正确性（Critical）
 ├─ 🏗️ architecture-reviewer — 架构合规（Major）
 ├─ 📡 api-reviewer           — API 规范（Major）
 ├─ 🔒 security-reviewer      — 安全性（Critical）
 ├─ 🧪 test-reviewer          — 测试覆盖（Major）
 └─ 🎨 style-reviewer         — 代码风格（Minor）
       ↓
  主 Agent 汇总 → 决策 → 修复 → 下一轮
```

理由：
1. **专业深度** — 每个 agent 只关注一个领域，审查更深入
2. **无注意力稀释** — 不会被其他维度分散精力
3. **并行高效** — 6 个维度同时审，总耗时 = 最慢的那个
4. **隔离客观** — 独立 context，不受开发过程影响

---

## 触发场景

### 本地评审模式

- 用户要求评审未提交的代码："评审代码" / "review 一下" / "看看我写的代码"
- 本地开发完成，push 前做代码评审
- worktree 中编码完成，想先自查再提交

### PR 评审模式

- 用户要求 review PR："review #N" / "看看这个 PR" / "审查代码"
- PR 创建后需要审查

---

## 评审模式

### 模式一：本地评审

在 worktree 中开发完成后，基于 `git diff` 评审未提交或未 push 的代码。

**适用场景**：push 前自查，发现问题在本地修复，避免往返 CI。

```bash
# 获取变更（在 worktree 中执行）
git diff origin/main          # 与 main 分支的差异
git diff HEAD                 # 未提交的变更
git diff --cached             # 已 staged 的变更
git diff origin/main --stat   # 变更文件列表
```

**输出**：直接在对话中展示评审报告，不创建 PR comment。

### 模式二：PR 评审

基于 `gh pr diff` 评审已提交的 PR。

**适用场景**：PR 提交后，合并前的正式评审。

```bash
gh pr view N --json title,body,labels,files,additions,deletions,baseRefName,headRefName
gh pr diff N
gh pr checks N
```

**输出**：评审报告 + `gh pr comment N` 发布到 PR。

---

## 三轮评审机制

| 轮次 | 目标 | 通过条件 |
|------|------|----------|
| **第一轮** | 全面审查，发现问题 | 修复所有 🔴 Critical + 🟡 Major |
| **第二轮** | 复审修复 + 遗漏检查 | 修复所有 🔴 Critical + 🟡 Major |
| **第三轮** | 终审确认 | 无 🔴 Critical，🟡 Major ≤ 2 且为非阻塞建议 |

- 每轮评审都 spawn 6 个专业子 agent
- 如果某轮全部 APPROVED，可提前结束（不必走满三轮）
- 如果三轮后仍有未修复的 Critical，标记为 NEEDS DISCUSSION

---

## 工作流

### 步骤 1：确定评审模式和范围

```
本地评审：
  cd <worktree-dir>
  git diff origin/main --stat    # 变更概览
  git diff origin/main            # 完整 diff

PR 评审：
  gh pr view N --json ...
  gh pr diff N
  gh pr checks N
```

### 步骤 2：Spawn 6 个专业审查子 Agent

同时 spawn 6 个子 agent，每个负责一个维度：

```
reviewers = [
  {name: "correctness",  dim: "A", severity: "Critical"},
  {name: "architecture", dim: "B", severity: "Major"},
  {name: "api",          dim: "C", severity: "Major"},
  {name: "security",     dim: "D", severity: "Critical"},
  {name: "test",         dim: "E", severity: "Major"},
  {name: "style",        dim: "F", severity: "Minor"},
]
```

本地评审 taskName：`review-{dim}-local`
PR 评审 taskName：`review-{dim}-pr{N}`

### 步骤 3：汇总报告

主 agent 收集 6 份报告，合并为统一报告：

1. 汇总所有问题（按严重级别排序）
2. 逐条核对验收条件（如有）
3. 形成决策：
   - 有 🔴 Critical → CHANGES REQUESTED
   - 有 🟡 Major → CHANGES REQUESTED
   - 只有 🔵 Minor → APPROVED（附建议）
   - 全部 APPROVED → APPROVED

### 步骤 4：处理结果

**本地评审**：
- 直接展示报告
- APPROVED → 可以 push
- CHANGES REQUESTED → 修复后重新评审或直接 push（取决于严重程度）

**PR 评审**：
- 发布报告到 PR comment（`gh pr comment N`）
- APPROVED → 通知用户可以合并
- CHANGES REQUESTED → 修复问题，push，进入下一轮

### 步骤 5：下一轮（如需）

修复后自动进入下一轮，重新 spawn 6 个子 agent 审查。
如果全部 APPROVED，提前结束。

---

## 6 个专业审查员定义

### 🔍 A. 代码正确性审查员（Critical）

**角色**：资深 Go 后端工程师，专注逻辑正确性。

**检查清单**：
- [ ] 业务逻辑是否正确实现（对照需求/Issue）
- [ ] 所有 error path 都处理（无 `_ = err` 忽略）
- [ ] 并发安全：goroutine 生命周期、锁使用、channel 关闭
- [ ] 空指针 / nil map / 越界访问
- [ ] 类型断言安全（comma-ok pattern）
- [ ] context 传播和取消处理
- [ ] 资源泄漏（defer Close、goroutine 泄漏）
- [ ] 数值溢出 / 精度问题
- [ ] 时间处理（UTC vs Local、时区）

### 🏗️ B. 架构合规审查员（Major）

**角色**：系统架构师，关注分层和设计模式。

**检查清单**：
- [ ] 四层分层正确：handler → domain → infra → store
- [ ] Handler 保持薄层（只做解析→调用 service→响应）
- [ ] Handler 无业务逻辑（无 DB 操作、无复杂分支）
- [ ] Domain 通过接口依赖，不直接 import 具体实现
- [ ] 错误使用 sentinel error 模式（domain 层定义，handler 层映射）
- [ ] 包组织合理（无循环依赖、无 god package）
- [ ] import path 统一 `github.com/whg517/fleet`
- [ ] 配置注入方式合理（不硬编码、不全局变量）
- [ ] 中间件链顺序正确

### 📡 C. API 规范审查员（Major）

**角色**：API 设计专家，对照 RESTful 规范检查。

**检查清单**：
- [ ] URL 符合 RESTful 约定（`/api/v1/resources`，复数名词）
- [ ] HTTP Method 正确（GET 查询、POST 创建、PUT 更新、DELETE 删除）
- [ ] HTTP Status Code 正确（200/201/204/400/401/403/404/409/500）
- [ ] 响应格式统一（`{data}` 或 `{data, pagination}` 或 `{error}`）
- [ ] 错误码已定义且符合规范
- [ ] 分页参数标准化（page + page_size）
- [ ] 路由命名一致（kebab-case 或 camelCase）
- [ ] 非 CRUD 动作用 `POST /resource/:id/action`
- [ ] 版本化（URL 路径版本 `/api/v1/`）

**注意**：如果不涉及 API 变更，直接返回 "N/A — 本次变更不涉及 API"。

### 🔒 D. 安全审查员（Critical）

**角色**：应用安全工程师，专注 OWASP Top 10。

**检查清单**：
- [ ] 无敏感数据硬编码（Token、密钥、密码、kubeconfig）
- [ ] 无敏感数据在日志中泄露（password/secret/token/key 字段脱敏）
- [ ] SQL 注入防护（参数化查询、不拼 SQL）
- [ ] 命令注入防护（不直接 shell exec 用户输入）
- [ ] 用户输入校验（长度、格式、范围）
- [ ] 权限检查到位（RBAC enforce 在每个写操作前）
- [ ] CORS 配置不过于宽松（不应 `AllowOrigins: ["*"]` 在生产）
- [ ] 文件路径遍历防护
- [ ] 依赖安全性（已知漏洞的包版本）
- [ ] Dockerfile 非 root 运行
- [ ] 配置文件无敏感默认值

### 🧪 E. 测试审查员（Major）

**角色**：QA 工程师，关注测试有效性和覆盖。

**检查清单**：
- [ ] 有单元测试覆盖（目标 > 80%）
- [ ] 测试不只是 happy path（edge case、error case）
- [ ] 有表驱动测试（Go 惯例）
- [ ] mock 使用合理（不 mock 一切，也不集成测试一切）
- [ ] 测试命名清晰（Test_Function_Scenario）
- [ ] 无 `t.Skip()` 无解释
- [ ] CI 中测试能通过
- [ ] 基准测试（性能敏感场景）

**注意**：脚手架/文档类变更可适当降低要求，但需明确说明。

### 🎨 F. 代码风格审查员（Minor）

**角色**：代码质量守护者，关注可读性。

**检查清单**：
- [ ] 命名清晰一致（Go 命名规范：驼峰、导出大写）
- [ ] 包名简短全小写
- [ ] 无死代码 / 大段注释掉的代码
- [ ] 无 magic number（应有命名常量）
- [ ] 函数长度合理（< 50 行为佳）
- [ ] 文件长度合理（< 300 行为佳）
- [ ] Commit message 符合规范（`type(scope): subject`）
- [ ] 前端代码 TypeScript strict
- [ ] import 分组规范（标准库 / 第三方 / 本项目）

---

## 子 Agent Task 模板

### 本地评审模板

```
你是 Fleet 平台的 {维度名称} 审查员（{角色描述}）。

## 你的审查维度
只负责 **维度 {字母}. {维度名称}**，不要审查其他维度。

## 检查清单
{该维度的完整 checklist}

## 审查目标
- 评审模式：本地评审（未 push 的代码）
- 变更范围：{file_count} 文件

## 审查方法
1. 在 {worktree_path} 下执行 `git diff origin/main` 获取变更
2. 阅读相关项目文档：
   - docs/ARCHITECTURE.md
   - docs/DEVELOPMENT.md
   - .claude/skills/api-design/SKILL.md（仅 API 维度）
3. 逐文件审查，只关注你负责的维度

## 输出格式

### 维度 {字母} 审查结论：APPROVED / CHANGES REQUESTED

### 问题列表
| # | 严重级别 | 文件:行号 | 问题 | 建议 |
|---|---------|----------|------|------|

### 说明

## 安全规则
- 只读 diff 和文件，不修改代码
- 不执行 merge，不 push
```

### PR 评审模板

```
你是 Fleet 平台的 {维度名称} 审查员（{角色描述}）。

## 你的审查维度
只负责 **维度 {字母}. {维度名称}**，不要审查其他维度。

## 检查清单
{该维度的完整 checklist}

## PR 信息
- PR #{number}：{title}
- 分支：{head_ref} → {base_ref}
- 变更：{file_count} 文件，+{additions} / -{deletions}

## 审查方法
1. 获取 diff：`gh pr diff {number}`
2. 阅读相关项目文档：
   - docs/ARCHITECTURE.md
   - docs/DEVELOPMENT.md
   - .claude/skills/api-design/SKILL.md（仅 API 维度）
3. 逐文件审查，只关注你负责的维度

## 输出格式

### 维度 {字母} 审查结论：APPROVED / CHANGES REQUESTED

### 问题列表
| # | 严重级别 | 文件:行号 | 问题 | 建议 |
|---|---------|----------|------|------|

### 说明

## 安全规则
- 只读 diff 和文件，不修改代码
- 不执行 merge，不 push
```

---

## 汇总报告模板

### 本地评审报告

```markdown
## 🤖 Code Review Report — 本地评审（第 {R} 轮）

### 评审范围
- 模式：本地评审
- 分支：{branch}
- 变更：{file_count} 文件，+{additions} / -{deletions}

### 结论：APPROVED / CHANGES REQUESTED / NEEDS DISCUSSION

### 各维度审查结果

| 维度 | 审查员 | 结论 | 问题数 |
|------|--------|------|--------|
| A. 代码正确性 | 🔍 | ✅ APPROVED | 0 |
| B. 架构合规 | 🏗️ | ✅ APPROVED | 0 |
| C. API 规范 | 📡 | N/A | - |
| D. 安全性 | 🔒 | ⚠️ CHANGES REQUESTED | 2 |
| E. 测试 | 🧪 | ✅ APPROVED | 0 |
| F. 代码风格 | 🎨 | ✅ APPROVED | 1 (Minor) |

### 问题详情

#### 🔴 Critical（必须修复）

| # | 维度 | 文件:行号 | 问题 | 建议 |
|---|------|----------|------|------|

#### 🟡 Major（强烈建议修复）

| # | 维度 | 文件:行号 | 问题 | 建议 |
|---|------|----------|------|------|

#### 🔵 Minor（可选改进）

| # | 维度 | 文件:行号 | 问题 | 建议 |
|---|------|----------|------|------|

### 本轮总结

（一段话总结）
```

### PR 评审报告

同上格式，标题改为 `PR #{N}`，可附加验收条件核对表格。

---

## 决策矩阵

| 汇总结果 | 处理 |
|----------|------|
| 任何 🔴 Critical | CHANGES REQUESTED → 修复 → 下一轮 |
| 任何 🟡 Major | CHANGES REQUESTED → 修复 → 下一轮 |
| 只有 🔵 Minor | APPROVED → 在报告中附建议 |
| 全部 APPROVED | APPROVED → 本地评审可 push / PR 评审可合并 |

### 三轮通过规则

- **第一轮全 APPROVED** → 直接通过（提前结束）
- **第一/二轮有问题** → 修复后进入下一轮
- **第三轮全 APPROVED** → 通过
- **第三轮仍有 Critical** → NEEDS DISCUSSION（人工介入）

---

## 安全规则

1. 子 agent 只读 diff 和文件，**不修改代码**
2. 子 agent 不执行 merge
3. 子 agent 不 push
4. 审查报告中不包含敏感数据
5. PR 评审通过且门禁全绿后，主 agent 可自行 merge

---

## 编排器调用备忘

当本 skill 被 dev-workflow 编排器调用时，主 agent 容易犯以下错误（历史教训）：

| 错误 | 后果 | 正确做法 |
|------|------|----------|
| 跳过 review 直接 push | 未发现的 bug 进入 PR | 阶段 4 是 feat/fix 必做步骤 |
| 让开发子 agent 自审 | 盲区未发现 | 主 agent spawn 独立 review 子 agent |
| spawn review agent 时只给引用不给内容 | 子 agent 不知道检查什么 | task 中内联完整 checklist |
| 不等 6 个 review 全部完成就推进 | 汇总不完整 | 用 sessions_yield 等全部完成 |
