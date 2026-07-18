---
name: pr-review
description: "Fleet 平台 PR Review：6 个专业子 agent 并行评审 + 三轮评审机制。所有 PR review 必须走此 skill。"
---

# PR Review: 多 Agent 并行评审

## 核心设计

**6 个专业子 agent 各司其职，并行评审同一 PR。**

每个子 agent 只负责一个维度，带着该领域的专业 checklist 深入审查。主 agent 汇总 6 份报告，形成统一决策。

```
PR #N
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

- 用户要求 review PR："review #N" / "看看这个 PR" / "审查代码"
- PR 创建后需要审查
- "发起评审"

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

### 步骤 1：获取 PR 信息

```bash
gh pr view N --json title,body,labels,files,additions,deletions,baseRefName,headRefName
gh pr diff N > /tmp/pr-N-diff.txt
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

for r in reviewers:
    sessions_spawn(
        task = build_review_task(pr_info, r),
        mode = "run",
        taskName = f"review-{dim}-{pr_number}"
    )

sessions_yield()  # 等待全部完成
```

### 步骤 3：汇总报告

主 agent 收集 6 份报告，合并为统一报告：

1. 汇总所有问题（按严重级别排序）
2. 逐条核对验收条件
3. 形成决策：
   - 有 🔴 Critical → CHANGES REQUESTED
   - 有 🟡 Major → CHANGES REQUESTED
   - 只有 🔵 Minor → APPROVED（附建议）
   - 全部 APPROVED → APPROVED

### 步骤 4：发布报告 + 修复

```bash
gh pr comment N --body-file /tmp/review-report-r{轮次}.md
```

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

**注意**：如果 PR 不涉及 API 变更，直接返回 "N/A — 本 PR 不涉及 API 变更"。

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

**注意**：脚手架/文档类 PR 可适当降低要求，但需明确说明。

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

每个子 agent 收到的 task 格式：

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
   - docs/ARCHITECTURE.md（{相关章节}）
   - docs/CONTRIBUTING.md
   - .claude/skills/api-design/SKILL.md（仅 API 维度需要）
3. 逐文件审查，只关注你负责的维度

## 输出格式

### 维度 {字母} 审查结论：APPROVED / CHANGES REQUESTED

### 问题列表
| # | 严重级别 | 文件:行号 | 问题 | 建议 |
|---|---------|----------|------|------|

### 说明
（如果没有问题，简要说明你检查了什么，为什么认为 OK）

## 安全规则
- 只读 diff 和文件，不修改代码
- 不执行 merge，不 push
```

---

## 汇总报告模板

主 agent 汇总 6 份报告后的输出格式：

```markdown
## 🤖 Code Review Report — PR #{N}（第 {R} 轮）

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

### 验收条件核对

| 条件 | 状态 | 说明 |
|------|------|------|

### 本轮总结

（一段话总结）

---
**审查人：🤖 AI Reviewer Team（第 {R} 轮）**
```

---

## 决策矩阵

| 汇总结果 | 处理 |
|----------|------|
| 任何 🔴 Critical | CHANGES REQUESTED → 修复 → 下一轮 |
| 任何 🟡 Major | CHANGES REQUESTED → 修复 → 下一轮 |
| 只有 🔵 Minor | APPROVED → 在 comment 中附建议 |
| 全部 APPROVED | APPROVED → 可合并 |

### 三轮通过规则

- **第一轮全 APPROVED** → 直接通过（提前结束）
- **第一/二轮有问题** → 修复后进入下一轮
- **第三轮全 APPROVED** → 通过
- **第三轮仍有 Critical** → NEEDS DISCUSSION（人工介入）

---

## 多 PR 并行

多个 PR 需要同时 review 时：

每个 PR 独立执行完整流程（6 子 agent × 可能多轮）。
通过 taskName 区分：`review-{dim}-pr{N}`。

---

## 安全规则

1. 子 agent 只读 diff 和文件，**不修改代码**
2. 子 agent 不执行 merge
3. 子 agent 不 push
4. 审查报告中不包含敏感数据
5. 主 agent 负责 merge（仅在用户确认后）
