# Fleet — 架构文档

| 字段 | 内容 |
|------|------|
| 文档版本 | v1.4 |
| 创建日期 | 2026-07-10 |
| 更新日期 | 2026-07-14（PG 高可用改为 CloudNativePG；移除 DB Migration 管理（SYS-04）） |
| 状态 | Draft — 评审优化中 |

---

## 1. 技术选型

| 层 | 选型 | 理由 |
|----|------|------|
| 前端 | Next.js SPA (React + TypeScript + HeroUI) | 团队选型，HeroUI 基于 React Aria + Tailwind CSS |
| 后端 | Go + Echo (单体起步) | 并发模型适合编排操作，单二进制部署简单，Echo 轻量高性能 |
| ORM | Ent | Schema-as-Code，类型安全，图查询支持复杂关联 |
| 配置 | Viper | 多配置源支持，生态成熟 |
| 日志 | zap | 高性能零分配，生态成熟 |
| 数据库 | PostgreSQL | 关系型数据，事务保证，审计日志 |
| 缓存/队列 | Redis | 部署状态缓存、异步任务 |
| 构建引擎 | Argo Workflows | K8s 原生 CI，与 GitOps 体系一致 |
| 部署引擎 (K8s) | Argo CD | GitOps 模式，声明式，有 diff/sync/rollback |
| 部署引擎 (物理节点) | Ansible | 幂等、模板化、K8s Job 隔离执行 |
| 镜像仓库 | Harbor | 企业级，支持扫描和策略 |
| 认证 | OIDC | 对接企业 SSO |
| 凭证存储 | K8s Secret + AES-256-GCM | 应用层加密；etcd 加密属集群职责，平台建议 |
| 监控 | Prometheus + AlertManager | 现有基础设施 |

---

## 2. 系统架构

```
┌──────────────────────────────────────────────────────────────┐
│                    Next.js SPA (前端)                         │
│   服务目录 · 部署中心 · 监控面板 · 审计日志                  │
├──────────────────────────────────────────────────────────────┤
│                     Go API Server                             │
│          REST/WebSocket · OIDC · RBAC · 审计拦截              │
├────────┬─────────┬────────┬────────┬───────┬────────┬────────┬────────┐
│ 服务   │ 环境     │ 构建    │ 部署   │ 集群  │ 配置   │ 审批   │ 通知   │
│ 目录   │ 管理     │ 触发    │ 编排   │ 纳管  │ 变更   │ 网关   │ 中心   │
├────────┴─────────┴────────┴────────┴───────┴────────┴────────┴────────┤
│                   PostgreSQL · Redis                          │
├──────────────────────────────────────────────────────────────┤
│                   执行面 (Execution Plane)                    │
│                                                              │
│   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐    │
│   │ Argo         │   │ Argo CD       │   │ Ansible      │    │
│   │ Workflows    │   │ (K8s 部署)    │   │ Runner       │    │
│   │ (构建)       │   │               │   │ (物理节点)    │    │
│   └──────┬───────┘   └──────┬───────┘   └──────┬───────┘    │
│          ▼                  ▼                   ▼            │
│   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐    │
│   │ Harbor       │   │ K8s Cluster  │   │ 物理机 / VM  │    │
│   │ (镜像制品)    │   │ (单集群)      │   │              │    │
│   │              │   │ dev/test/    │   │              │    │
│   │              │   │ pre/prod     │   │              │    │
│   └──────────────┘   └──────────────┘   └──────────────┘    │
├──────────────────────────────────────────────────────────────┤
│                 可观测层 (Observability)                      │
│           Prometheus · AlertManager · Loki/ES               │
└──────────────────────────────────────────────────────────────┘
```

### 分层职责

| 层 | 职责 |
|----|------|
| **前端 (Next.js SPA)** | 用户交互、数据展示、WebSocket 实时状态推送 |
| **API Server (Go)** | 业务逻辑、认证授权、审计拦截、编排调度、通知中心 |
| **数据层 (PostgreSQL + Redis)** | 元数据持久化、缓存、异步任务队列、密钥加密存储 |
| **执行面** | Argo Workflows（构建）、Argo CD（K8s 部署）、Ansible（物理机部署） |
| **可观测层** | 对接现有 Prometheus/AlertManager/Loki，不自建 |

---

## 3. 核心设计原则

1. **平台是控制面，不是数据面** — 平台挂了不影响线上服务运行
2. **GitOps 模式** — 部署的期望状态存储在 Git，Argo CD 负责调和
3. **模板化封装** — 服务部署逻辑封装在 Helm Chart / Ansible Role 中，平台只管契约
4. **一切操作可审计** — 所有写操作记录 who/when/what/result
5. **敏感数据加密存储** — 凭证数据经 AES-256-GCM 加密后存储在 PostgreSQL，加密密钥通过 K8s Secret 注入。建议集群启用 etcd encryption-at-rest 提供纵深防御（由集群运维团队负责）
6. **通知统一走 Webhook** — 所有通知（审批/部署/构建/告警）通过 Webhook 统一发送，后续可扩展

## 3.1 Argo CD Sync 策略

| 场景 | Sync 策略 | 说明 |
|------|----------|------|
| dev / test | auto-sync = true | Git commit 后自动部署，开发快速反馈 |
| pre / prod | auto-sync = false (manual sync) | 平台触发 sync，确保审批流程不被绕过 |

平台通过 Argo CD API 触发 manual sync，不依赖 Argo CD 的自动轮询。
部署后通过 Argo CD API 轮询 Application 状态判断 Healthy / Degraded。

---

## 4. 关键流程

### 4.1 构建流程

```
Git Push → Webhook → 平台触发 Argo Workflow
  → Workflow 执行: git clone → build → docker build → push to Harbor
  → 构建完成回调平台 API
  → 平台注册新版本（image ref + git commit + 时间）
```

平台职责：触发构建、接收结果、管理版本。
构建逻辑封装在 Argo WorkflowTemplate 中，平台参数化触发。

### 4.2 K8s 部署流程

```
选择服务 + 版本 + 环境
  → 权限校验
  → [pre/prod] 审批流程
  → 部署锁校验（同一服务+环境不能并发部署）
  → 平台 Git commit 更新 environments/<env>/<service>.yaml
  → 更新 Argo CD Application（targetRevision / values）
  → 平台触发 Argo CD Sync（dev/test auto-sync，pre/prod manual sync）
  → 轮询 Argo CD 状态 (Healthy / Progressing / Degraded)
  → 成功 → 记录 + 审计 + 通知
  → 失败 → 通知 + 可选自动回滚
```

**并发控制**：同一 service + environment 组合加分布式锁（Redis），防止并发部署冲突。
- 锁 Key：`deploy:lock:{service_id}:{environment_id}`
- 锁 TTL：10 分钟（部署通常 < 5min，留 buffer）
- 自动释放：TTL 到期自动释放；部署/配置变更完成后主动释放
- 强制释放：operator 角色可强制释放（记录审计）
- 互斥范围：部署和配置变更共享同一把锁

### 4.3 物理节点部署流程

```
选择服务 + 版本 + 目标主机组
  → 权限校验
  → [prod] 审批流程
  → 部署锁校验
  → 平台创建 K8s Job（Ansible Runner 镜像）
  → Job Pod 从 Secret 挂载 SSH 密钥
  → 执行 ansible-playbook -i <动态inventory> -e @<参数>
  → 平台轮询 Job 状态
  → 收集执行结果 + 日志
  → 记录 + 审计 + 通知
```

**物理节点部署设计**：

| 维度 | 方案 |
|------|------|
| 部署模板 | Ansible Role，含 manifest.yaml 契约（defaults=参数接口，tasks=部署逻辑） |
| 执行环境 | K8s Job Pod，每次部署独立执行，与平台后端隔离 |
| SSH 密钥 | K8s Secret 挂载到 Job Pod，不落平台后端磁盘 |
| Inventory | 平台管理主机组（按环境分组），动态生成 inventory 文件 |
| 版本管理 | 平台记录每次部署的 Role 版本 + 参数快照 |
| 回滚 | 用上一次成功部署的参数重新执行 Ansible Role |
| 幂等性 | Ansible 模块天然幂等，重复执行结果一致 |
| 日志 | Job Pod stdout 实时采集，可在平台查看 |

### 4.4 配置变更流程

```
修改环境级 Helm values
  → [pre/prod] 审批
  → Git commit 到 environments/<env>/<service>.yaml
  → Argo CD 检测 Git 变更 → 自动 Sync
  → 配置生效
```

配置变更即 Git commit，天然有版本历史和审计。

### 4.5 发布审批流程

```
开发提交部署到 pre/prod 的请求
  → 平台记录 pending approval
  → 通知有审批权限的人（Webhook）
  → 审批人 approve / reject
  → approve → 自动执行部署
  → reject → 记录并通知提交人
  → 超时 24h → 自动 reject 并通知提交人
```

单级审批，非工作流引擎。

### 4.6 服务接入流程

```
运维/开发发起服务接入
  → 创建 Service 记录（名称、deploy_type、owner_team）
  → 关联 Helm Chart（chart_name、chart_repo）或 Ansible Role
  → 关联 Harbor 项目（镜像仓库路径）
  → 初始化环境配置（为每个环境生成默认 values override）
  → 创建 Argo CD Application（每个环境一个）
  → 服务状态标记为 active
  → 记录审计
```

接入完成后服务即可通过平台部署。

### 4.7 服务冻结/解冻流程

```
运维触发冻结
  → Service 状态标记为 frozen
  → 所有新部署请求被拒绝（返回冻结错误）
  → 所有配置变更请求被拒绝
  → 进行中的部署不受影响（等待完成）
  → Webhook 通知相关人员

运维触发解冻
  → Service 状态恢复 active
  → 正常操作恢复
```

---

## 5. 数据模型

### 5.1 核心实体

```
User
  id, name, email, oidc_subject, status, created_at

Role
  id, name (admin / operator / developer / viewer), description

Permission
  role_id, resource_type (service / cluster / *), resource_id,
  environment, actions[] (deploy / view / config / approve / admin)

Cluster
  id, name, api_server, kubeconfig_encrypted, environment,
  labels jsonb, status, created_at

Service
  id, name, description, owner_team, deploy_type (k8s / vm),
  chart_name, chart_repo, status (active / frozen / offline), created_at

ServiceVersion
  id, service_id, version, image_ref, git_commit,
  built_at, changelog, created_at

Environment
  id, name (dev / test / pre / prod), cluster_id,
  namespace_pattern, approval_required bool, approver_role,
  config_overrides jsonb, created_at

PromotionRule
  id, from_environment_id, to_environment_id,
  require_validation bool,        # 必须在源环境验证通过
  cooldown_minutes int default 0, # 晋升冷却期（分钟）
  created_at

Deployment
  id, service_id, version_id, environment_id,
  status (pending_approval / queued / deploying / succeeded / failed),
  params jsonb, initiated_by, approved_by,
  batch_id,                    # 关联批次（可选，单服务部署为 null）
  started_at, finished_at, created_at

DeploymentBatch
  id, environment_id, initiated_by,
  status (pending_approval / deploying / partially_succeeded / succeeded / failed),
  release_note text,             # 统一发布说明
  total_count int, succeeded_count int, failed_count int,
  created_at, finished_at

ConfigSnapshot
  id, service_id, environment_id, values jsonb,
  created_by, created_at

Approval
  id, deployment_id, status (pending / approved / rejected),
  approver_id, comment, decided_at, created_at

AuditLog
  id, user_id, action, resource_type, resource_id,
  detail jsonb, ip, created_at

Registry
  id, name, type (harbor), url, credentials_ref, created_at

WebhookConfig
  id, name, url, secret, events[] (approval/deployment/build/alert),
  is_active, retry_count int default 3, retry_interval_sec int default 30,
  created_at

NotificationLog
  id, webhook_config_id, event_type, payload jsonb,
  status (pending/sent/failed), response_code int, attempts int,
  sent_at, created_at

AuditLog
  id, user_id, action, resource_type, resource_id,
  detail jsonb, ip, prev_hash, created_at
  -- 独立表 INSERT-only，hash chain 防篡改，应用账号无 UPDATE/DELETE 权限
```

### 5.2 ER 关系

```
User ──< Permission >── Role
User ──< Deployment
User ──< AuditLog
User ──< Approval

Cluster ──< Environment     (一个环境属于一个集群；一个集群可含多个环境命名空间)
Service ──< ServiceVersion
Service ──< Deployment >── Environment
Service ──< ConfigSnapshot >── Environment
Deployment ──< Approval
DeploymentBatch ──< Deployment
```

**Cluster 与 Environment 的关系**：
- 当前阶段单集群，通过 namespace 隔离多环境（dev/test/pre/prod）
- 一个 Cluster 承载多个 Environment
- 后续扩展多集群时，可定义多个 Cluster 关联不同 Environment

### 5.3 平台部署拓扑

```
管理集群 / 管理命名空间 (Management)
┌──────────────────────────┐
│  Go API (2 replicas)     │
│  Next.js (静态托管)       │
│  PostgreSQL (CloudNativePG) │
│  Redis (Sentinel)        │
│  Argo CD Server          │
│  Argo Workflows          │
└─────────────┬────────────┘
              │ Argo CD API
              ▼
业务命名空间 (dev / test / pre / prod)
┌──────────────────────────┐
│  微服务 (100+)            │
└──────────────────────────┘
```

- 平台组件部署在独立管理命名空间，与业务 namespace 资源隔离
- 单集群场景下 Argo CD 通过 namespace 管理多环境
- PostgreSQL 通过 CloudNativePG Operator 管理，主从复制 + 自动 failover
- WAL 持续归档到对象存储（S3/MinIO），支持 PITR
- Redis 采用 Sentinel 3 节点保证可用性

### 5.4 状态对账机制

检测平台状态与 Argo CD 实际状态偏差：
- **实时**：K8s informer/watch 模式监听 Argo CD Application CRD 变更（替代轮询）
- **补偿**：每 5 分钟执行一次全量对账（弥补 watch 事件丢失）
- **偏差处理**：状态不一致时更新平台记录，标记 `sync_drift = true`，通知运维
- **Application 丢失**：Argo CD Application 被删除时平台自动重建

### 5.5 回滚策略

| 维度 | 策略 |
|------|--------|
| 触发条件 | Argo CD Application 状态为 Degraded 持续超过 5 分钟 |
| 触发方式 | 手动触发（默认） / 自动触发（可选，按服务配置） |
| 回滚操作 | `argocd app rollback`（回到上一个 healthy sync），不改 Git |
| auto-sync 暂停 | 回滚后临时关闭 Argo CD auto-sync，防止 Git 中的新版本覆盖回滚 |
| Git 同步 | Git commit 同步期望状态后恢复 auto-sync |
| Prod 审批 | 自动回滚不需要审批；手动回滚到指定历史版本需审批 |
| 成功判定 | Argo CD Application 状态恢复 Healthy |
| 紧急快通道 | 回滚和紧急扩容等时间敏感操作可绕过 GitOps 链路直接调用 K8s/Argo CD API，事后异步补 Git 同步。适用场景：回滚、紧急扩容（S-01）。不适用于配置变更和新部署 |

### 5.6 Git 写入策略

100+ 服务并发部署时，Git commit 可能产生 push 冲突。策略：
- 平台后端维护 **Git 写入队列**，所有 Git commit 操作串行化执行
- 单次 commit 可 batch 多个文件变更（如同时更新多个环境的 values）
- push 冲突时自动 pull-rebase-push 重试（最多 3 次）
- 重试失败时部署标记为 failed，通知运维

### 5.7 配置变更安全防护

配置变更可能导致服务启动失败。防护策略：

| 阶段 | 防护措施 |
|------|----------|
| 提交前 | Helm values schema 校验 + dry-run 渲染预览 |
| 部署中 | Argo CD sync 后等待健康检查（readiness probe） |
| 失败检测 | Argo CD Application 状态为 Degraded 超过 3min |
| 自动回退 | 回退到上一个 ConfigSnapshot（Git revert + 重新 sync） |
| 通知 | 配置回退时 Webhook 通知发起人和运维 |

### 5.8 部署锁高可用

部署锁主要依赖 Redis，增加 PostgreSQL 兜底：
- **Redis 故障**：锁 TTL（10min）覆盖短暂故障窗口；降级到 PG 行级锁（`SELECT ... FOR UPDATE SKIP LOCKED`）

### 5.9 物理节点部署的 GitOps 补全

物理节点部署虽不走 Argo CD，但通过 Git 补全审计链路：
- 每次部署将 **参数快照 + Role 版本（Git tag/commit hash）** commit 到 `ansible/deployments/` 目录
- 回滚时从 Git 历史读取上一次的 Role 版本和参数，用旧版 Role 执行
- 审计链路：Git history（参数版本） + AuditLog（操作记录）双重追溯

### 5.10 定时任务

平台需要周期性执行的运维任务，通过 Go 后端的内置调度器实现（robfig/cron 或同类库），不依赖外部 CronJob。

| 任务 | 周期 | 说明 |
|------|------|------|
| 版本漂移检测（D-13） | 每日 10:00 | 扫描所有服务版本分布，检测 dev→prod 版本差距过大的异常服务，通过 Webhook + 站内消息通知 |
| 审批超时检查（AP-01） | 每 5min | 扫描 pending 审批，超过 24h 的自动 reject 并通知提交人 |
| 凭证过期检查（SEC-01） | 每日 09:00 | 检查凭证到期时间，< 7 天的告警提醒运维 |
| 服务冻结提醒（D-07） | 每日 09:00 | 冻结超过 N 天的服务提醒运维主管 |
| 状态对账补偿（5.4） | 每 5min | 全量对账平台状态与 Argo CD 实际状态，弥补 watch 事件丢失 |
| DEK 轮转（SEC-01） | 每 30 天 | 重新加密所有凭证数据 |

实现要点：
- 调度器随 Go 后端启动，多副本时通过 Redis 分布式锁保证只执行一次
- 每次执行记录审计日志
- 支持管理员手动触发

### 5.11 测试策略

| 层级 | 策略 |
|------|--------|
| 单元测试 | Go 后端 domain/infra 层全面覆盖，目标覆盖率 > 80% |
| 集成测试 | infra 层与 Argo CD/Harbor/Git 的 mock 集成测试 |
| 端到端测试 | M2 完成后构建 E2E 测试（部署全链路） |
| 前端测试 | 关键交互页面组件测试，E2E 覆盖核心流程 |

> **注**：平台自身的 DB schema 变更通过 ent atlas migrate 管理，**不涉及业务服务的数据库 migration**。业务服务自身的 DB migration 由开发团队和 DBA 负责，平台不介入。

### 5.12 安全加固

| 项目 | 策略 |
|------|------|
| 审计日志脱敏 | 配置变更、凭证操作等写入审计日志前，自动检测 password/secret/token/key 等敏感字段并脱敏 |
| Webhook SSRF 防护 | Webhook URL 限制为白名单域名或内网地址，禁止调用内网敏感地址（如 169.254.x.x metadata） |
| 权限即时撤销 | 被禁用/降权的用户 token 通过 Redis 黑名单即时失效，不依赖 token 自然过期 |
| Token 超时 | Access token 30min（生产操作需合理窗口）；Refresh token 8h（一个工作日） |
| SSO 故障降级 | 不保留永久 admin 账号。配置备用 OIDC Provider，或通过 break-glass 流程（紧急工单 + DB 直接操作） |
| K8s API 限流 | 使用 informer/watch 模式维护本地缓存，减少直接 API 调用。对 events 等高频资源设置 resync 间隔 |
| 镜像来源约束 | prod 环境建议要求镜像来自可信 build pipeline（通过 label/tag 约定 + 准入校验） |

---

## 6. API 设计

### 6.1 认证

```
POST   /api/v1/auth/login              # OIDC 重定向（含 PKCE）
GET    /api/v1/auth/callback           # OIDC 回调
POST   /api/v1/auth/refresh            # Refresh Token 刷新
POST   /api/v1/auth/logout             # OIDC RP-Initiated Logout
GET    /api/v1/auth/me                 # 当前用户信息 + 权限
```

### 6.2 服务目录

```
GET    /api/v1/services                # 服务列表（分页/过滤）
POST   /api/v1/services                # 注册新服务
GET    /api/v1/services/:id            # 服务详情（含当前各环境状态）
PUT    /api/v1/services/:id            # 更新服务信息
DELETE /api/v1/services/:id            # 下线服务
```

### 6.3 版本管理

```
GET    /api/v1/services/:id/versions           # 版本列表
POST   /api/v1/services/:id/versions/:ver/deploy   # 触发部署
```

### 6.4 环境

```
GET    /api/v1/environments                     # 环境列表
GET    /api/v1/environments/:id/services        # 该环境下所有服务及状态
```

### 6.5 部署

```
POST   /api/v1/deployments                      # 创建部署（K8s/物理机统一入口）
GET    /api/v1/deployments/:id                  # 部署状态
POST   /api/v1/deployments/:id/rollback         # 回滚
GET    /api/v1/deployments/:id/logs             # 部署日志
```

### 6.6 集群

```
GET    /api/v1/clusters                         # 集群列表
POST   /api/v1/clusters                         # 注册集群
GET    /api/v1/clusters/:id/health              # 集群健康
```

### 6.7 OCI

```
GET    /api/v1/registries/:id/images            # 镜像列表
GET    /api/v1/registries/:id/scans             # 镜像扫描结果
```

### 6.8 配置

```
GET    /api/v1/services/:id/config              # 当前配置
PUT    /api/v1/services/:id/config              # 修改配置（触发 Argo CD sync）
POST   /api/v1/services/:id/config/dry-run      # Helm template 渲染预览
```

### 6.9 审批

```
GET    /api/v1/approvals                        # 待审批列表
POST   /api/v1/approvals/:id/approve            # 批准
POST   /api/v1/approvals/:id/reject             # 拒绝
```

### 6.10 审计

```
GET    /api/v1/audit/logs                       # 审计日志查询
```

### 6.11 监控

```
GET    /api/v1/services/:id/health              # 服务健康状态
GET    /api/v1/services/:id/metrics             # 服务指标
```

### 6.12 扩缩容

```
PUT    /api/v1/services/:id/scale               # 手动扩缩
PUT    /api/v1/services/:id/hpa                 # HPA 策略
```

### 6.13 集群运维

```
GET    /api/v1/clusters/:id/pods                # Pod 列表与状态
GET    /api/v1/clusters/:id/pods/:pod/logs       # Pod 日志
POST   /api/v1/clusters/:id/nodes/:node/drain   # 节点排水
```

### 6.14 通知管理

```
GET    /api/v1/webhooks                         # Webhook 配置列表
POST   /api/v1/webhooks                         # 创建 Webhook
PUT    /api/v1/webhooks/:id                     # 更新 Webhook
DELETE /api/v1/webhooks/:id                     # 删除 Webhook
GET    /api/v1/notifications/logs                # 通知发送记录
```

### 6.15 诊断聚合 API

```
GET    /api/v1/services/:id/diagnosis           # 故障排查聚合视图（一次返回部署历史+Pod状态+最近Events+健康状态+配置变更）
GET    /api/v1/services/:id/matrix               # 服务跨环境版本矩阵（避免 N+1 查询）
GET    /api/v1/deployments/:id/progress          # 部署多阶段进度（提交→校验→配置→部署→健康检查→完成）
```

---

## 6A. API 通用约定

### 分页

```
GET /api/v1/services?page=1&page_size=20

响应: { data: [], pagination: { page, page_size, total } }
```

### 过滤与排序

```
GET /api/v1/services?team=order&status=active&env=prod&sort=-created_at
```

### 错误格式

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "service_name is required",
    "details": [{ "field": "service_name", "issue": "required" }]
  }
}
```

---

## 7. GitOps 仓库结构

```
platform-repo/
├── helm-charts/              # 所有服务的 Helm Chart
│   ├── service-a/
│   │   ├── Chart.yaml
│   │   ├── values.yaml       # 基础 values
│   │   └── templates/
│   └── service-b/
│
├── environments/             # 环境覆盖配置
│   ├── dev/
│   │   ├── service-a.yaml    # dev 环境的 values override
│   │   └── service-b.yaml
│   ├── test/
│   ├── pre/
│   └── prod/
│
├── argocd-apps/              # Argo CD Application 定义
│   ├── dev/
│   │   ├── service-a.yaml
│   │   └── service-b.yaml
│   ├── pre/
│   └── prod/
│
└── ansible/                  # 物理机部署
    ├── inventory/
    │   ├── dev.ini
    │   └── prod.ini
    ├── roles/
    │   └── service-xxx/      # Ansible Role 模板
    └── playbooks/
```

平台对 Git 的操作：
- **读**：读 Chart/values 结构做参数校验
- **写**：部署时写 Application CRD 或更新 values override

---

## 8. 后端项目结构

```
cmd/
  server/
    main.go                    # 入口

internal/
  api/                         # HTTP/WS handler 层
    router.go
    handler/
      service.go
      deployment.go
      cluster.go
      ...
    middleware/
      auth.go                  # OIDC 认证
      rbac.go                  # 权限拦截
      audit.go                 # 审计日志

  domain/                      # 业务逻辑层
    service/                   # 服务管理
    environment/               # 环境管理
    deployment/                # 部署编排（核心）
    cluster/                   # 集群管理
    version/                   # 版本管理
    config/                    # 配置管理
    approval/                  # 审批
    audit/                     # 审计查询
    scaling/                   # 扩缩容
    notification/              # 通知中心（Webhook 统一发送）
    topology/                  # 版本分布总览

  infra/                       # 基础设施对接层
    argocd/                    # Argo CD API client
    argowf/                    # Argo Workflows API client
    harbor/                    # Harbor API client
    ansible/                   # Ansible Runner 封装（K8s Job 触发）
    git/                       # Git 操作（读 chart、写 values）
    prometheus/                # 监控查询
    kube/                      # K8s API（集群健康检查）
    secrets/                   # 密钥加密存储（AES-256-GCM）
    notify/                    # 通知渠道（通用 Webhook）

  store/                       # 持久层
    postgres/                  # PostgreSQL 实现
    redis/                     # Redis 实现
    models/                    # 数据模型定义

  config/                      # 配置加载
  pkg/                         # 通用工具
```

---

## 9. WebSocket 实时推送

### 频道设计

| 频道 | 格式 | 推送内容 |
|------|------|----------|
| 部署状态 | `deployment:{id}` | status 变更、进度更新、完成/失败事件 |
| 服务健康 | `service:{id}:health` | 健康检查状态变化 |
| 审批通知 | `user:{id}:approvals` | 新的审批请求、审批结果 |
| 集群状态 | `cluster:{id}` | 集群健康状态变化 |

### 消息格式

```json
{
  "type": "deployment.status",
  "data": {
    "deployment_id": "xxx",
    "status": "deploying",
    "progress": "syncing",
    "timestamp": "2026-07-10T08:00:00Z"
  }
}
```

### 实现机制

- 前端建立 WebSocket 连接，按权限订阅频道
- 后端通过 Redis Pub/Sub 在多实例间同步消息
- 连接断开后自动重连 + 增量补全（通过 deployment_id 查询最新状态）

---

## 10. 失败模式与容灾

| 故障场景 | 影响范围 | 缓解策略 |
|---------|---------|---------|
| 平台 Go 后端宕机 | 无法发起变更操作，**线上服务不受影响** | 多副本部署，健康检查自动重启 |
| Argo CD 不可用 | 无法发起新部署，**已部署服务不受影响** | Argo CD 多副本，CrashBackoff 自动恢复 |
| Argo Workflows 不可用 | 无法构建新镜像 | 不影响已部署服务，恢复后重试 |
| Harbor 不可用 | 无法推送/拉取镜像 | 多副本部署，定期备份 |
| PostgreSQL 故障 | 平台不可用 | CloudNativePG 自动 failover（RTO < 30s），WAL 归档支持 PITR |
| Redis 故障 | 部署状态缓存失效，降级直查 | AOF 持久化 + Sentinel |
| Git 仓库不可用 | 无法触发新部署/配置变更 | 不影响已部署服务，Argo CD 使用最后已知状态 |

**核心原则**：平台是控制面，数据面（线上服务）不依赖平台运行。

---

## 11. 功能里程碑

项目基于 AI 辅助开发，不设固定人力配置和工期排期。按功能模块组织交付里程碑，每个里程碑定义交付物和验收标准。

### M1: 基础设施与认证

- Go 项目脚手架（路由、中间件、配置加载）
- PostgreSQL schema（ent atlas 管理平台自身表结构）
- OIDC 认证（PKCE、Token 刷新、Logout）
- RBAC 权限模型
- K8s Secret + 应用层 AES-256-GCM 加密
- Next.js 前端项目初始化

**验收**：OIDC 登录可用，RBAC 生效，凭证加密存储可用

### M2: 核心部署链路

- 集群注册 + 服务接入流程（D-08）
- Argo CD Application 管理 + 状态同步（informer/watch 模式）
- K8s 部署流程（dev/test 直通）
- 部署状态实时跟踪（WebSocket）
- 回滚 + 状态对账
- 部署前置校验（D-10）

**验收**：能部署 Helm Chart 到 dev 环境，状态实时可见，能回滚

### M3: 审批与配置

- 发布审批流程（pre/prod）+ 24h 超时
- Webhook 通知中心 + 通知合并策略
- 审计日志（hash chain 防篡改）
- 配置变更管理（dry-run + 安全防护）
- 部署锁完善（PG 兜底）

**验收**：prod 部署走审批，审计完整，配置变更生效

### M4: 构建链路

- Argo Workflows 对接
- 构建触发与状态跟踪
- Harbor 镜像管理 + 版本注册

**验收**：能触发构建，镜像推 Harbor，版本自动注册

### M5: 物理节点与运维

- Ansible Runner（K8s Job 隔离执行）
- 监控集成（Prometheus）
- 扩缩容、Pod 日志、集群运维操作
- 镜像扫描展示

**验收**：物理节点部署可用，监控集成可用

### M6: 稳定化

- 端到端测试
- 性能优化
- 试点服务上线

**验收**：API p95 < 200ms，试点服务稳定运行

> **注**：平台不负责接管现有老服务迁移。物理节点部署能力作为标准化方案提供，新服务按需选择部署方式。

---

## 12. 风险与对策

| 风险 | 概率 | 影响 | 对策 |
|------|------|------|------|
| Argo CD 管理 400+ Application 性能 | 中 | 中 | M2 阶段做性能验证；必要时调优 Argo CD 参数 |
| 100+ 服务接入工作量 | 中 | 中 | 平台提供标准化接入流程，服务按需接入 |
| Git 并发写入瓶颈 | 中 | 中 | 平台层 Git 写入队列串行化；评估按团队分仓库 |
| Webhook 通知投递失败 | 中 | 高 | 重试（3 次指数退避）+ 通知合并策略 + 站内消息兜底 |
| OIDC/SSO 对接延迟 | 高 | 高 | 跨团队协调，提前启动对接 |
| 生产 K8s RBAC 权限审批 | 中 | 高 | 提前规划所需权限清单，与集群管理员协调 |
| Git 仓库写权限/分支保护 | 中 | 中 | 提前打通平台 Git 账号权限和分支保护规则 |
| 配置变更导致服务启动失败 | 中 | 高 | Helm values schema 校验 + Argo CD 健康检查失败自动回退 ConfigSnapshot |
