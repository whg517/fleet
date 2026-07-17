---
name: pr-review
description: "Fleet 平台 PR Review：通过子 agent 执行自动化代码审查，输出结构化 review 报告。所有 PR review 必须走子 agent。"
---

# PR Review: 子 Agent 代码审查

## 触发场景

- 用户要求 review PR
- PR 创建后需要审查
- "review #N" / "看看这个 PR" / "审查代码"

## 核心原则

**所有 PR Review 必须通过子 agent 执行。** 理由：

1. **隔离性**：子 agent 在隔离 context 中工作，不受主 agent 的上下文偏见影响
2. **可追溯**：子 agent 输出完整的 review 报告，存档可查
3. **并行能力**：多个 PR 可同时 review，互不阻塞
4. **客观性**：子 agent 不参与开发过程，审查更中立

---

## 工作流

### 步骤 1：获取 PR 信息

```bash
gh pr view N --json title,body,labels,files,additions,deletions,baseRefName,headRefName
gh pr diff N
gh pr checks N
```

### 步骤 2：Spawn 子 Agent 执行 Review

```python
sessions_spawn(
    task=review_task_brief,  # 见下方模板
    mode="run",
    taskName=f"pr-review-{pr_number}",
    # 不需要 fork context，子 agent 独立审查
)
```

### 步骤 3：Review 报告处理

1. 子 agent 返回结构化 review 报告
2. 主 agent 根据报告决定下一步：
   - **APPROVED**：通知用户可以合并
   - **CHANGES REQUESTED**：通知用户需要修改，列出问题
   - **NEEDS DISCUSSION**：标记需要用户决策的问题

---

## 子 Agent Task 模板

Spawn 子 agent 时，task 内容按以下模板构造：

```
你是 Fleet 平台的 Code Reviewer。请对 PR #N 进行严格的代码审查。

## PR 信息
- 标题：{pr_title}
- 分支：{head_ref} → {base_ref}
- 变更文件：{file_count} 个（+{additions} / -{deletions}）
- 标签：{labels}
- 关联 Issue：{linked_issues}

## 审查范围

请阅读以下文件的完整 diff：
{changed_files_list}

同时阅读以下项目文档获取上下文：
- docs/ARCHITECTURE.md（架构设计）
- docs/CONTRIBUTING.md（协作规范）
- .claude/skills/api-design/SKILL.md（API 设计规范，如涉及 API 变更）

## 审查维度

### A. 代码正确性（Critical）
- 业务逻辑是否正确实现
- 错误处理是否完整（所有 error path 都处理）
- 并发安全（goroutine、锁、channel）
- 空指针 / 边界条件 / 类型断言

### B. 架构合规（Major）
- 分层是否正确：handler（薄层）→ domain（业务）→ store（持久层）
- Handler 是否只做请求解析和响应，不含业务逻辑
- Domain service 是否通过接口依赖，不直接依赖具体实现
- 错误是否使用 sentinel error 模式

### C. API 规范（Major，涉及 API 变更时）
- URL 是否符合 RESTful 约定
- HTTP Method 和 Status Code 是否正确
- 响应格式是否统一（data/pagination/error）
- 错误码是否已定义

### D. 安全性（Critical）
- 是否有敏感数据泄露（Token、密钥、kubeconfig）
- 是否有 SQL 注入 / 命令注入风险
- 用户输入是否做了校验
- 权限检查是否到位（RBAC）

### E. 测试（Major）
- 是否有单元测试覆盖
- 测试是否有效（不只是 happy path）
- 是否有表驱动测试

### F. 代码风格（Minor）
- 命名是否清晰一致
- 包组织是否合理
- 是否有死代码 / 注释掉的代码
- Commit message 是否符合规范

## 验收条件核对

PR 描述中的验收条件：
{acceptance_criteria}

逐条核对代码是否满足所有验收条件。

## 输出格式

请按以下格式输出 review 报告：

### Review 结论：APPROVED / CHANGES REQUESTED / NEEDS DISCUSSION

### 问题列表

| # | 维度 | 严重级别 | 文件:行号 | 问题 | 建议 |
|---|------|---------|----------|------|------|

严重级别：🔴 Critical（必须修改）/ 🟡 Major（强烈建议修改）/ 🔵 Minor（可选）

### 验收条件核对

| 条件 | 状态 | 说明 |
|------|------|------|

### 总结

（一段话总结这个 PR 的整体质量）
```

---

## Review 决策矩阵

| 子 Agent 结论 | 严重级别 | 处理方式 |
|---------------|---------|---------|
| 有 Critical 问题 | 🔴 | request changes，列出所有 🔴 问题 |
| 只有 Major 问题 | 🟡 | request changes，列出 🟡 问题 |
| 只有 Minor 问题 | 🔵 | approve，在 comment 中列出建议 |
| 无问题 | - | approve |

---

## 多 PR 并行 Review

多个 PR 需要同时 review 时：

1. 为每个 PR spawn 一个独立子 agent
2. 各子 agent 并行执行，互不干扰
3. 各自返回 review 报告
4. 主 agent 汇总结果

```python
for pr_number in pr_list:
    sessions_spawn(
        task=build_review_task(pr_number),
        mode="run",
        taskName=f"pr-review-{pr_number}"
    )
sessions_yield()  # 等待所有子 agent 完成
```

---

## Review 报告存档

Review 完成后，将报告作为 PR comment 发布：

```bash
gh pr comment N --body "$(cat review-report.md)"
```

---

## 安全规则

1. 子 agent 只读 PR diff 和相关文件，**不修改代码**
2. 子 agent 不执行 `gh pr merge`
3. 子 agent 不 push 任何分支
4. Review 报告中不包含敏感数据
