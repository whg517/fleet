# 服务管理平台 — 架构文档

| 字段 | 内容 |
|------|------|
| 文档版本 | v1.1 |
| 创建日期 | 2026-07-10 |
| 状态 | Draft — 待评审 |

---

## 1. 技术选型

| 层 | 选型 | 理由 |
|----|------|------|
| 前端 | Next.js SPA (React + TypeScript) | 团队选型，生态成熟 |
| 后端 | Go (单体起步) | 并发模型适合编排操作，单二进制部署简单 |
| 数据库 | PostgreSQL | 关系型数据，事务保证，审计日志 |
| 缓存/队列 | Redis | 部署状态缓存、异步任务 |
| 构建引擎 | Argo Workflows | K8s 原生 CI，与 GitOps 体系一致 |
| 部署引擎 (K8s) | Argo CD | GitOps 模式，声明式，有 diff/sync/rollback |
| 部署引擎 (物理节点) | Ansible | 幂等、模板化、K8s Job 隔离执行 |
| 镜像仓库 | Harbor | 企业级，支持扫描和策略 |
| 认证 | OIDC | 对接企业 SSO |
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
5. **敏感数据加密存储** — 通过云 KMS 信封加密（Envelope Encryption）保护主密钥，凭证数据经 AES-256-GCM 加密后存储在 PostgreSQL
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
  started_at, finished_at, created_at

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
│  PostgreSQL (主从+Patroni)│
│  Redis (3 Sentinel)      │
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
- PostgreSQL 采用主从复制 + 自动 failover（Patroni）
- Redis 采用 Sentinel 3 节点保证可用性

### 5.4 状态对账机制

定时对账任务，检测平台状态与 Argo CD 实际状态偏差：
- **频率**：每 5 分钟执行一次
- **逻辑**：遍历所有 Deployment，对比平台状态与 Argo CD Application 实际状态
- **偏差处理**：状态不一致时更新平台记录，标记 `sync_drift = true`，通知运维
- **Application 丢失**：Argo CD Application 被删除时平台自动重建

### 5.5 回滚策略

| 维度 | 策略 |
|------|--------|
| 触发条件 | Argo CD Application 状态为 Degraded 持续超过 5 分钟 |
| 触发方式 | 手动触发（默认） / 自动触发（可选，按服务配置） |
| 回滚操作 | `argocd app rollback`（回到上一个 healthy sync），不改 Git |
| Git 同步 | 回滚后平台自动 Git commit 同步状态（异步） |
| Prod 审批 | 自动回滚不需要审批；手动回滚到指定历史版本需审批 |
| 成功判定 | Argo CD Application 状态恢复 Healthy |

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
| PostgreSQL 故障 | 平台不可用 | 主从复制 (streaming replication) + 定期备份 + PITR |
| Redis 故障 | 部署状态缓存失效，降级直查 | AOF 持久化 + Sentinel |
| Git 仓库不可用 | 无法触发新部署/配置变更 | 不影响已部署服务，Argo CD 使用最后已知状态 |

**核心原则**：平台是控制面，数据面（线上服务）不依赖平台运行。

---

## 11. 实施计划

### 人力假设

| 角色 | 人数 | 职责 |
|------|------|------|
| 后端工程师 | 2 | Go API Server + 业务逻辑 + 基础设施对接 |
| 前端工程师 | 1 | Next.js SPA 全部 UI |
| DevOps/SRE | 0.5（兼职） | Argo CD/Workflows 部署、GitOps 仓库搭建 |
| 测试工程师 | 0.5（兼职） | 后期介入，端到端测试 |

### Phase 1: 骨架搭建（3 周）

- Go 项目脚手架 + 基础框架（路由、中间件、配置加载）
- PostgreSQL schema 设计 + migration
- OIDC 认证接入（含 PKCE、Token 刷新、Logout）
- RBAC 权限模型实现
- KMS 信封加密接入
- Next.js 前端项目初始化

**DoD**：用户可通过 OIDC 登录，RBAC 生效，DB migration 可运行，凭证加密存储可用

### Phase 2: 核心部署链路（4 周）

- 集群注册管理
- 服务注册管理（含手动注册/API 导入版本）
- Argo CD Application CRUD + 状态同步
- K8s 部署流程（dev/test 直通）
- 部署状态实时跟踪（WebSocket）
- 回滚功能
- 状态对账机制

**DoD**：能通过平台部署示例 Helm Chart 到 dev 环境，状态实时可见，能回滚

### Phase 3: 审批与审计（3 周）

- 发布审批流程（pre/prod）
- Webhook 通知中心
- 审计日志（hash chain 防篡改）
- 配置变更管理（含 dry-run 预览）
- 部署锁完善

**DoD**：prod 部署走审批，审计日志完整，配置变更生效，dry-run 可用

### Phase 4: 构建链路（3 周）

- Argo Workflows 对接
- 构建触发与状态跟踪
- Harbor 镜像管理
- 版本注册与 changelog

**DoD**：能触发 Argo Workflow 构建，镜像推 Harbor，版本自动注册

### Phase 5: 物理节点与外围（3 周）

- Ansible Runner 封装 + 物理节点部署流程
- 监控集成（Prometheus）
- 扩缩容、Pod 日志、集群运维操作
- 镜像扫描展示

**DoD**：物理节点部署可用，监控集成可用，集群运维操作可用

### Phase 6: 稳定化（3 周）

- 端到端测试
- 性能优化
- 文档完善
- 3 个试点服务正式上线

**DoD**：端到端测试通过，API p95 < 200ms，3 个试点服务稳定运行

> **注**：平台不负责接管现有老服务迁移。物理节点部署能力作为标准化方案提供，新服务按需选择部署方式。

**总工期：19 周（约 5 个月）**，含 20% buffer

---

## 12. 风险与对策

| 风险 | 概率 | 影响 | 对策 |
|------|------|------|------|
| Argo CD Application 管理复杂度超预期 | 中 | 中 | 先支持标准 Helm Chart 模式，逐步扩展 |
| 100+ 服务接入工作量 | 中 | 中 | 平台提供标准化接入流程，服务按需接入 |
| 多集群网络连通性 | 中 | 高 | 当前单集群已规避；后续扩展时提前验证 |
| 团队对 Argo CD/Workflows 不熟悉 | 中 | 中 | Phase 1 前做技术 spike，验证核心链路 |
| 前端工作量被低估 | 中 | 中 | MVP 阶段前端只做核心功能，管理类操作可用 API |
| Git 并发写入瓶颈 | 中 | 中 | 部署锁串行化 Git commit；评估 batch commit 优化 |
| Webhook 通知投递失败 | 中 | 高 | 重试机制（3 次指数退避）；后期引入死信队列 |
