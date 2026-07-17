# 协作开发规范

## GitHub PR 驱动开发流程

所有代码和文档变更通过 **Issue → Branch → PR → Review → Merge** 流程管理。

### 流程概览

```
创建 Issue（描述需求/Bug）
  → Backlog → Todo（分配里程碑 + 优先级）
  → Todo → In Progress（认领 + 创建分支）
  → 开发 + 提交
  → In Progress → Review（提交 PR）
  → Code Review + 验收确认
  → Squash Merge 到 main
  → Issue 自动关闭 → Done
```

---

## 1. Issue 管理

### Issue 模板

创建 Issue 时选择对应模板：

- **需求开发**：从用户故事/需求文档拆分出的开发任务
- **Bug 修复**：线上或测试环境问题
- **文档变更**：需求/架构/设计文档更新

### Issue 命名

```
[模块] 简明描述
```

示例：
```
[deploy] 实现 Argo CD Application 管理 + 状态同步
[auth] 修复 OIDC token 刷新失败
[config] 配置变更 dry-run 预览
[docs] 补充批量发布用户故事
```

### Issue 内容要求

- **用户故事**：用 Given-When-Then 格式描述本次需求的服务场景（谁、做什么、达到什么效果）
- **关联需求**：标注对应的 REQUIREMENTS.md 需求 ID（如 D-01）
- **验收条件**：明确 done 的标准（从用户故事的验收条件复制）
- **技术要点**（可选）：实现思路、涉及文件
- **依赖说明**（可选）：如果依赖其他 Issue 完成，标注 `Blocked by #N`

### 标签体系

| 标签 | 用途 |
|------|------|
| `P0` / `P1` / `P2` | 优先级 |
| `feat` / `fix` / `docs` / `refactor` | 类型 |
| `M1` ~ `M6` | 里程碑（对应架构文档中的功能里程碑） |
| 模块标签：`deploy` `auth` `config` `cluster` `audit` `build` `notify` 等 | 功能模块 |

### Issue 依赖与并行管理

#### 依赖表达

使用 GitHub Issue 的 **description + comment** 表达依赖关系：

```markdown
## 依赖

- Blocked by #12（认证模块必须先完成）
- Blocked by #15（Ent schema 定义必须先合并）
```

#### 依赖规则

| 场景 | 规则 |
|------|------|
| **硬依赖** | `Blocked by #N`，被依赖 Issue 未 Done 前不得创建分支 |
| **软依赖** | 在 Issue 中 @ 相关人员沟通确认接口/数据格式后可并行 |
| **无依赖** | 直接认领开发，互不阻塞 |

#### 并行开发协作

多个 Issue 同时进行时的协作规范：

1. **接口先行**：有交互的模块，先单独提一个 PR 定义接口/类型（如 `store/repos/` 下的 interface），合并后各方并行开发
2. **分支隔离**：每个 Issue 一个分支，禁止在 A 的分支上做 B 的事
3. **冲突预防**：涉及同一文件的 Issue，在 Issue comment 中提前沟通分工
4. **合并顺序**：有依赖关系的按拓扑顺序合并；无依赖的可任意顺序
5. **AI 子 agent 并行**：多个独立 Issue 可 spawn 多个子 agent 同时工作，各自分支独立，PR 分别提交

```
示例：M2 部署链路，3 个 Issue 并行

#12 [deploy] Argo CD Application CRUD   ← 无依赖，先做
#14 [deploy] 部署状态同步（WebSocket）     ← 软依赖 #12（需要 Application 存在才能同步状态）
#15 [deploy] 部署历史 + 回滚              ← 硬依赖 #12（需要 Deployment 记录）

分工：
- #12 先做，合并后 #15 可以开始
- #14 与 #12 并行，先用 mock Application 接口开发前端 WebSocket 订阅
```

---

## 2. 分支管理

### 分支命名

格式：`type/issue-number-short-desc`

```
feat/42-argocd-app-management
fix/58-oidc-token-refresh
docs/61-batch-release-stories
refactor/55-deployment-lock
```

### 分支规则

- **main 分支保护**：禁止直接 push，只能通过 PR 合并
- **分支生命周期短**：合并后立即删除
- 开发前从 main 最新代码切出分支

---

## 3. 提交规范

```
type(scope): subject

# 可选 body
详细说明、关联 Issue
```

| type | 用途 | 示例 |
|------|------|------|
| `feat` | 新功能 | `feat(deploy): 实现 Argo CD Application CRUD` |
| `fix` | Bug 修复 | `fix(auth): 修复 OIDC token 刷新失败` |
| `docs` | 文档更新 | `docs(api): 补充部署 API 接口文档` |
| `refactor` | 重构 | `refactor(config): 重构配置变更锁逻辑` |
| `test` | 测试 | `test(deploy): 添加部署回滚 E2E 测试` |

> **禁用 `chore` 类型。**

多个 commit 在同一个 PR 中可以保留，合并时 squash 成一个。

---

## 4. Pull Request 规范

### PR 标题

与 commit 规范一致：`type(scope): subject`

### PR 描述模板

```markdown
## 关联 Issue

Closes #42

## 变更说明

（这次改了什么，为什么改）

## 关联需求/故事

- 需求：D-01 (Helm 部署)
- 用户故事：OPS-A.1, US-B.1

## 验收条件

- [ ] 能通过平台部署 Helm Chart 到 dev 环境
- [ ] 部署状态实时可见
- [ ] 能回滚到上一个版本
- [ ] 单元测试通过
- [ ] 文档已更新

## 测试方式

（怎么验证这次改动是 OK 的）

## 截图 / 录屏（如涉及 UI）

（可选）
```

### Review 要求

| 变更类型 | Review 要求 | CI 要求 |
|---------|-----------|---------|
| feat / fix | 至少 1 人 approve | ✅ 全绿 |
| docs | 可自合并（文档类 PR） | 无 |
| refactor | 至少 1 人 approve | ✅ 全绿 |

---

## 5. 合并方式

统一使用 **Squash Merge**：

```bash
gh pr merge 42 --squash --delete-branch
```

- PR 内多个 commit 压缩为一个
- 分支自动删除
- PR 描述中的 `Closes #N` 自动关闭关联 Issue

---

## 6. 里程碑与看板

### GitHub Project Board

使用 GitHub Projects (Beta) 管理开发看板：

| 列 | 说明 |
|----|------|
| **Backlog** | 待拆分的用户故事/需求 |
| **Todo** | 已拆分为 Issue，分配了里程碑和优先级 |
| **In Progress** | 已认领，正在开发 |
| **Review** | PR 已提交，等待 Review + 验收 |
| **Done** | 已合并到 main，验收条件全部满足 |

### Issue 状态流转规则

| 从 → 到 | 触发条件 | 操作人 |
|---------|---------|--------|
| Backlog → Todo | Issue 已分配 **Milestone** + **优先级标签** + **模块标签**，验收条件已填写 | Issue 创建者 / 负责人 |
| Todo → In Progress | 认领 Issue：assign 自己 + 确认无 `Blocked by` 阻塞 + 已创建开发分支 | 开发者 |
| In Progress → Review | 已提交 PR，PR 描述包含 `Closes #N`，CI 通过（或无 CI 要求） | 开发者 |
| Review → In Progress | Review 有修改意见，需要返工 | Reviewer（request changes）|
| Review → Done | Reviewer approve + **验收条件全部打勾** + Squash Merge 完成 | Reviewer |
| Review → Done（拒绝） | 需求变更/方案否决，关闭 Issue 并说明原因 | 负责人 |
| 任意 → Backlog | 需要重新评估（优先级变更、方案调整） | 负责人 |

**流转约束**：
- 禁止跨列跳跃（如 Todo 直接到 Done）
- `Blocked by` 的 Issue 在依赖项 Done 之前，不得从 Todo 移到 In Progress
- In Progress 超过 **5 个工作日** 未提交 PR，在 Issue comment 说明原因或降级回 Todo

### 验收闭环

验收在 **PR Review 阶段** 完成，Reviewer 是验收责任人：

1. **Reviewer 逐条核对** PR 描述中的验收条件 checklist
2. 所有验收条件 ✅ 打勾后，Reviewer approve
3. approve 后执行 Squash Merge：`gh pr merge <N> --squash --delete-branch`
4. Issue 通过 `Closes #N` 自动关闭，看板自动移到 Done
5. 如果验收条件未全部满足，Reviewer request changes 并说明缺什么

> **验收条件不可只看 CI 绿灯**。CI 只验证编译和测试，功能验收需要 Reviewer 确认。

### 里程碑映射

对应架构文档的功能里程碑：

| Milestone | 内容 | 状态 |
|-----------|------|------|
| M1 | 基础设施与认证 | 待开始 |
| M2 | 核心部署链路 | 待开始 |
| M3 | 审批与配置 | 待开始 |
| M4 | 构建链路 | 待开始 |
| M5 | 物理节点与运维 | 待开始 |
| M6 | 稳定化 | 待开始 |

---

## 7. AI 辅助开发协作

使用 OpenClaw 子 agent 辅助开发时：

1. **创建 Issue** 描述任务（人 or agent 均可创建）
2. **子 agent 在分支上工作**：`git checkout -b feat/42-xxx`
3. **子 agent 完成后创建 PR**：`gh pr create`
4. **人工 Review**：PR 是人审查 AI 产出的 checkpoint
5. **合并**：确认无误后 squash merge

> 子 agent 的 PR 应标注 `🤖 ai-generated` 标签，提醒 Reviewer 重点关注。

---

## 快速操作速查

```bash
# 创建 Issue
gh issue create --title "[deploy] 实现 Argo CD Application 管理" --label "feat,P0,M2,deploy"

# 创建分支
git checkout main && git pull
git checkout -b feat/42-argocd-app-management

# 开发完成后推送
git push -u origin feat/42-argocd-app-management

# 创建 PR
gh pr create --title "feat(deploy): 实现 Argo CD Application CRUD + 状态同步" \
  --body-file .github/pull_request_template.md

# Review 后合并
gh pr merge 42 --squash --delete-branch
```
