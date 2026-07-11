# 数据库 Migration 管理用户故事 — Fleet

| 字段 | 内容 |
|------|------|
| 角色 | 运维工程师 / 开发工程师 |
| 文档版本 | v1.0 |
| 创建日期 | 2026-07-11 |
| 关联文档 | REQUIREMENTS.md v1.3, ARCHITECTURE.md v1.3, ops-stories.md v1.1 |

---

## 写作说明

本文档覆盖**数据库 Migration 管理**场景，对应需求 SYS-04。故事从运维和开发两个视角出发，描述 migration 在发布、回滚和日常运维中的完整生命周期。故事编号以 `DBM` 前缀区分，关联 REQUIREMENTS.md 中的需求 ID。

---

## A. 发布时执行 DB Migration

### DBM-A.1: 发布前查看待执行 Migration 并评估风险

> **作为** 运维工程师，  
> **我希望** 在发布前看到目标版本包含的 DB migration 列表及其风险等级，  
> **以便** 提前评估风险，决定是否需要特殊操作（如提前执行、低峰期执行、通知 DBA 待命）。

**场景描述：**
周二发布日，订单服务（order-service）准备发布 v2.15.0 到 prod。这个版本包含 3 个 DB migration：新增一张表、给已有表加列、修改索引。100+ 服务中，有些 migration 是纯加法（安全），有些涉及数据变更（有锁表风险）。我需要在发布前清楚知道有哪些 migration、各自的风险等级，以及执行顺序，避免发布时卡在 DB 层面。

**操作步骤：**
1. 登录平台，进入 order-service 详情页，选择版本 v2.15.0
2. 平台展示该版本的部署预览，其中包含 **DB Migration 面板**：
   - 待执行 migration 列表（按顺序排列）
   - 每个 migration 的类型（DDL / DML）、风险等级（🟢 安全 / 🟡 注意 / 🔴 高风险）、预估执行时间
   - 是否包含不可逆操作（DROP TABLE / DELETE 等）
   - 是否需要锁表（DDL lock）
3. 查看风险评估详情：
   - 🟢 `add_column_orders_metadata` — 新增可空列，安全，预估 < 1s
   - 🟡 `create_index_orders_status` — CREATE INDEX CONCURRENTLY，不锁表但耗时较长，预估 2-3min
   - 🔴 `backfill_order_status` — 大批量数据更新，锁表风险，预估 5-10min
4. 根据 risk 评估决定执行策略：
   - 🟢🟡 随发布正常执行
   - 🔴 需要安排在低峰期或与 DBA 协调
5. 确认后正常发起部署，migration 在部署流程中自动执行

**验收条件：**
- 平台解析目标版本的 migration 文件，展示待执行列表（SYS-04）
- 每个 migration 标注类型、风险等级、是否可逆、预估执行时间
- 不可逆 migration（DROP / DELETE / TRUNCATE）标红高亮
- 风险评估信息来源于 migration 文件中的元数据标注 + 历史执行统计
- migration 列表按执行顺序排列
- 权限：所有角色（查看），operator/admin（操作）

**异常场景：**
- **Migration 文件解析失败**（格式错误）：平台提示具体错误，部署前校验不通过，拒绝部署（D-10）
- **预估执行时间过长**（如 > 30min）：平台额外警告，建议拆分 migration 或安排维护窗口
- **Migration 与当前 DB schema 版本不匹配**（如跨版本升级、缺少前置 migration）：平台检测到版本断层，拒绝部署并提示缺失的前置 migration
- **Migration 仓库不可达**：无法获取 migration 文件，平台提示运维手动确认后再部署

**关联需求：** SYS-04, D-10, D-01
**优先级：** P0

---

### DBM-A.2: 发布流程中自动执行 DB Migration

> **作为** 运维工程师，  
> **我希望** DB migration 在部署流程中按顺序自动执行，并实时展示执行进度，  
> **以便** 不需要手动登录数据库执行 SQL，降低人为操作风险。

**场景描述：**
order-service v2.15.0 审批通过后开始部署。部署流程分为多个阶段：拉取镜像 → 执行 DB migration → 滚动更新 Pod → 健康检查。其中 DB migration 是关键的前置步骤——如果 migration 失败，整个部署必须中止，不能让新代码连上未迁移的旧 schema。我需要在平台上实时看到 migration 执行到哪一步了。

**操作步骤：**
1. order-service v2.15.0 审批通过，平台触发部署
2. 平台展示部署多阶段进度条：
   - ✅ 前置校验完成
   - 🔄 **执行 DB Migration**（当前阶段）
     - ✅ `add_column_orders_metadata` — 完成（0.8s）
     - 🔄 `create_index_orders_status` — 执行中（已 1min 20s...）
     - ⏳ `backfill_order_status` — 等待中
   - ⏳ 滚动更新 Pod
   - ⏳ 健康检查
3. migration 全部执行成功后，自动进入下一阶段（滚动更新）
4. 如某个 migration 失败：部署中止，状态变为 Failed，展示失败 migration 的错误详情
5. 部署完成后，migration 执行记录归档，可在部署历史中查看

**验收条件：**
- migration 在部署流程中作为独立阶段执行，先于应用 Pod 滚动更新（SYS-04）
- 平台实时展示每个 migration 的执行状态（pending / running / succeeded / failed）
- migration 全部成功后才进入 Pod 滚动更新阶段（防止新代码连旧 schema）
- 任一 migration 失败时部署中止，已有 migration 不自动回滚（需人工确认后决定）（D-05）
- migration 执行记录（耗时、SQL 摘要）写入部署历史和审计日志（D-11, AU-01）
- 部署详情页支持查看 migration 执行日志
- 权限：所有角色（查看）

**异常场景：**
- **Migration 执行超时**（如 CREATE INDEX 超过预估时间）：平台标记 warning，但不自动中止（某些 DDL 本身耗时长）；运维根据情况决定是否等待或中止
- **Migration 执行失败**（如语法错误、权限不足、约束冲突）：部署中止，平台展示数据库返回的错误信息（SQL state + error message），运维需判断：修复 migration 文件重新发布 / 手动在 DB 执行修复 / 回滚到上一版本
- **Migration 执行成功但 Pod 滚动更新阶段失败**：migration 已生效但新版本 Pod 启动失败。此时 migration 不可自动撤销，运维需决定：修复应用代码重新部署 / 配合手动 DB 操作回滚
- **数据库连接失败**：migration 阶段无法连接 DB，平台报错并中止部署，运维检查 DB 状态和网络

**关联需求：** SYS-04, D-01, D-04, D-05, D-11, AU-01
**优先级：** P0

---

## B. 回滚时检查 Migration 兼容性

### DBM-B.1: 回滚前检测不可逆 Migration 并警告

> **作为** 开发工程师，  
> **我希望** 回滚服务版本时，平台能检测到不可逆 DB migration 并发出警告或阻止回滚，  
> **以便** 避免新代码回退后 schema 不兼容导致线上故障。

**场景描述：**
用户服务（user-service）v2.0.0 发布到 prod 后发现严重 bug，需要紧急回滚到 v1.9.0。但 v2.0.0 包含一个 migration：给 `users` 表加了 `phone_verified` 列（可空），还执行了一个数据迁移：把 `phone` 字段格式从国内格式统一改成国际格式。如果直接回滚代码到 v1.9.0，旧代码不认识新的手机号格式，会导致业务逻辑错误。更危险的是，如果 migration 里有 DROP COLUMN 操作，回滚后旧代码直接报错。平台需要在回滚前检测这些风险。

**操作步骤：**
1. 在平台 user-service 详情页点击「回滚」→ 选择目标版本 v1.9.0
2. 平台执行**回滚兼容性检查**，对比 v1.9.0 → v2.0.0 之间的 migration：
   - ✅ `add_column_phone_verified` — 新增可空列，旧代码忽略即可，**兼容**
   - 🔴 `migrate_phone_format` — 数据迁移（不可逆），旧代码不兼容新格式，**不兼容**
   - 🔴 `drop_column_legacy_field` — 删除列（不可逆），旧代码引用该列会报错，**不兼容**
3. 平台展示检查结果：
   - ⚠️ **回滚风险警告**：检测到 2 个不可逆 migration，直接回滚代码可能导致服务异常
   - 详情：列出不兼容的 migration 及原因
   - 建议操作：
     - **方案 A**：编写反向 migration 恢复数据格式 → 手动执行 → 再回滚代码
     - **方案 B**：在旧代码中兼容新 schema（热修复旧版本）
     - **方案 C**：admin 强制覆盖回滚（需确认风险，记录审计）
4. 开发工程师根据建议选择方案，优先 A 或 B

**验收条件：**
- 回滚操作前平台自动执行 migration 兼容性检查（SYS-04）
- 检测维度：DDL 变更（DROP / RENAME / TYPE CHANGE）、DML 数据迁移（格式变更、数据删除）
- 兼容性分级：✅ 兼容（可安全回滚）/ ⚠️ 需注意 / 🔴 不兼容（回滚后大概率异常）
- 不兼容时默认阻止回滚，展示具体 migration 和原因
- 提供 admin 强制回滚选项（需二次确认 + 审计记录）
- 检查结果记录到审计日志（AU-01）
- 权限：developer（发起回滚请求），operator/admin（强制回滚）

**异常场景：**
- **兼容性检查无法判定**（如 migration 逻辑复杂、含存储过程）：平台标记为 ⚠️ 需人工评估，提示运维联系开发确认
- **强制回滚后服务异常**：平台告警，运维需紧急修复（编写修复 migration 或重新部署兼容版本）
- **回滚目标版本与当前版本之间有大量 migration**（跨越多个版本）：平台汇总所有不兼容 migration，建议逐步回滚中间版本
- **Migration 历史记录缺失**（如服务接入平台前的 migration 不在记录中）：平台提示"无法完整检查"，建议人工确认

**关联需求：** SYS-04, D-05, D-11, AU-01
**优先级：** P0

---

## C. Migration 执行状态跟踪与失败处理

### DBM-C.1: Migration 执行失败后的诊断与处理

> **作为** 运维工程师，  
> **我希望** DB migration 执行失败时，平台能提供详细的错误信息和处理建议，  
> **以便** 快速定位失败原因，决定修复方案，缩短发布卡顿时间。

**场景描述：**
周三晚上发布商品服务（product-service）v3.0.0 到 prod，部署过程中 migration 阶段失败了。错误信息是 `ERROR: duplicate key value violates unique constraint "products_sku_uq"`。这是因为 migration 尝试给 `sku` 字段加唯一索引，但表中已有重复数据。我需要平台帮我快速看到完整错误信息、判断影响范围、决定下一步操作。

**操作步骤：**
1. 平台部署详情页显示 product-service v3.0.0 状态为 **Failed**，失败阶段为「DB Migration」
2. 进入部署详情，查看 migration 执行结果：
   - ✅ `add_column_products_tags` — 成功
   - ❌ `create_unique_index_sku` — 失败（SQL state: 23505，duplicate key violation）
   - ⏳ `update_product_search_index` — 未执行（已被中止）
3. 平台展示失败详情：
   - **错误类型**：唯一约束冲突
   - **错误 SQL**：`CREATE UNIQUE INDEX products_sku_uq ON products(sku)`
   - **数据库返回信息**：完整 error message + 冲突数据行数（如检测到 47 条重复 SKU）
   - **影响评估**：第一个 migration 已生效（新增列已加），第二个失败，schema 处于中间状态
4. 平台提供处理建议：
   - **当前状态**：schema 处于 partial migration 状态，应用 Pod 未更新（部署已中止）
   - **建议操作**：先在 DB 中清理重复数据 → 在平台点击「重试 Migration」→ 继续部署
5. 运维登录 DB 清理重复 SKU 数据
6. 回到平台点击「重试部署」→ 平台从失败的 migration 处继续执行（跳过已成功的）
7. migration 全部成功 → 部署继续

**验收条件：**
- migration 失败时部署自动中止，阻止 Pod 滚动更新（SYS-04, D-04）
- 平台展示失败 migration 的完整错误信息：SQL state、error message、失败 SQL 语句
- 平台标注当前 schema 状态（哪些 migration 已生效、哪些未执行）
- 支持「重试部署」从失败的 migration 处继续（跳过已成功的 migration）
- migration 失败通过 Webhook 通知发起人和运维团队（N-02）
- 失败记录写入审计日志（AU-01）
- 权限：operator/admin（重试部署），所有角色（查看）

**异常场景：**
- **重试时 migration 仍然失败**：平台再次报错，建议运维考虑：手动修复 DB / 修改 migration 文件重新发布 / 回滚已执行的 migration
- **Partial migration 状态下服务已有异常**（如新增列导致旧版本不兼容）：虽然 Pod 未更新，但 DB schema 变化可能影响在跑的旧版本。平台提示运维评估影响
- **数据库在 migration 执行期间故障**（如连接断开）：migration 状态不确定（可能已执行但未确认），平台提示运维手动检查 DB 状态后决定重试或回滚
- **重试部署被部署锁阻止**（如另一人同时在操作同一服务）：平台提示部署锁状态，运维协调后重试

**关联需求：** SYS-04, D-04, D-05, D-11, N-02, AU-01
**优先级：** P0

---

### DBM-C.2: 查看服务 Migration 执行历史

> **作为** 运维工程师，  
> **我希望** 查看某个服务在某环境的完整 migration 执行历史，  
> **以便** 了解 DB schema 的演进过程，排查潜在的 schema 不一致问题。

**场景描述：**
在日常巡检中发现 product-service 在 dev 和 prod 环境的 schema 版本不一致——dev 上跑了一些实验性 migration 没有 sync 到 prod。需要查看完整 migration 历史，确认差异并处理。

**操作步骤：**
1. 登录平台，进入 product-service 详情页 → 「DB Migration」标签
2. 平台展示 migration 历史时间线：
   - 每条记录：migration 文件名、版本、环境、执行时间、耗时、执行人/触发部署、结果
3. 切换环境查看：对比 dev 和 prod 的 migration 执行情况
4. 发现差异：dev 执行了 `experimental_add_search_vector`（v2.9.1-dev），prod 未执行
5. 确认该 migration 是实验性的，不应该到 prod → 标记为 dev-only
6. 确保后续版本发布时不包含该实验性 migration

**验收条件：**
- 每个服务+环境维度展示 migration 执行历史（SYS-04, D-11）
- 历史记录包含：migration 名称、关联版本、执行时间、耗时、结果、触发部署 ID
- 支持按环境切换查看
- 支持按状态过滤（成功 / 失败 / 进行中）
- migration 历史保留与部署历史一致的时间周期
- 权限：所有角色（查看）

**异常场景：**
- **某环境的 migration 历史缺失**（如服务接入平台前的 migration）：平台展示"接入前历史不可查"，建议运维在 DB 中手动确认当前 schema 版本
- **migration 执行记录与实际 DB schema 不一致**（如有人手动改了 DB）：平台检测到版本偏差时标记 warning，建议运维排查
- **跨环境版本差距过大**：平台提示该服务 dev/prod 版本漂移，建议排查原因

**关联需求：** SYS-04, D-11, AU-01
**优先级：** P1

---

## 附录：需求覆盖矩阵

| 需求 ID | 需求名称 | 关联用户故事 |
|---------|---------|-------------|
| SYS-04 | 数据库 Migration 管理 | DBM-A.1, DBM-A.2, DBM-B.1, DBM-C.1, DBM-C.2 |
| D-01 | Helm 部署 | DBM-A.1, DBM-A.2 |
| D-04 | 部署状态跟踪 | DBM-A.2, DBM-C.1 |
| D-05 | 回滚 | DBM-B.1, DBM-C.1 |
| D-10 | 部署前置校验 | DBM-A.1 |
| D-11 | 部署历史查询 | DBM-A.2, DBM-B.1, DBM-C.1, DBM-C.2 |
| AU-01 | 操作日志 | 全部故事 |
| N-02 | 部署结果通知 | DBM-C.1 |

---

## 附录：优先级分布

| 优先级 | 用户故事 | 数量 |
|--------|---------|------|
| P0 | DBM-A.1, DBM-A.2, DBM-B.1, DBM-C.1 | 4 |
| P1 | DBM-C.2 | 1 |
| **总计** | | **5** |
