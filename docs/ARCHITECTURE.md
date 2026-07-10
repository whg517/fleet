# 服务管理平台 — 架构文档

| 字段 | 内容 |
|------|------|
| 文档版本 | v1.0 |
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
| 部署引擎 (物理机) | Ansible | 幂等、模板化、成熟 |
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
│   │ (构建)       │   │               │   │ (物理机部署)  │    │
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
5. **敏感数据加密存储** — kubeconfig、凭证等通过 AES-256-GCM 加密后存储在 PostgreSQL
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

### 4.3 物理机部署流程

```
选择服务 + 版本 + 目标主机组
  → 权限校验
  → [prod] 审批流程
  → 平台调用 Ansible Runner（ansible-playbook -i inventory -e @params.json）
  → 收集执行结果
  → 记录 + 审计
```

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
  → 通知有审批权限的人（飞书/邮件）
  → 审批人 approve / reject
  → approve → 自动执行部署
  → reject → 记录并通知提交人
```

单级审批，非工作流引擎。

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
  chart_name, chart_repo, status, created_at

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

---

## 6. API 设计

### 6.1 认证

```
POST   /api/v1/auth/login              # OIDC 重定向
GET    /api/v1/auth/callback           # OIDC 回调
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
    ansible/                   # Ansible runner 封装
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

### Phase 1: 骨架搭建（2-3 周）

- Go 项目脚手架 + 基础框架（路由、中间件、配置加载）
- PostgreSQL schema 设计 + migration
- OIDC 认证接入
- RBAC 权限模型实现
- Next.js 前端项目初始化

### Phase 2: 核心部署链路（3-4 周）

- 集群注册管理
- 服务注册管理
- Argo CD Application CRUD + 状态同步
- K8s 部署流程（dev/test 直通）
- 部署状态实时跟踪
- 回滚功能

### Phase 3: 审批与审计（2 周）

- 发布审批流程（pre/prod）
- 审批通知（飞书/邮件）
- 审计日志记录与查询
- 配置变更管理

### Phase 4: 构建链路（2-3 周）

- Argo Workflows 对接
- 构建触发与状态跟踪
- Harbor 镜像管理
- 版本注册与 changelog

### Phase 5: 物理机与外围（2-3 周）

- Ansible Runner 封装
- 物理机部署流程
- 监控集成（Prometheus）
- 扩缩容
- 镜像扫描展示

### Phase 6: 稳定化（2 周）

- 端到端测试
- 性能优化
- 文档完善
- 上线

**预计总工期：13-17 周（3-4 个月）**

---

## 12. 风险与对策

| 风险 | 概率 | 影响 | 对策 |
|------|------|------|------|
| Argo CD Application 管理复杂度超预期 | 中 | 中 | 先支持标准 Helm Chart 模式，逐步扩展 |
| 100+ 服务迁移工作量 | 高 | 高 | 提供批量导入工具，优先迁移核心服务 |
| 多集群网络连通性 | 中 | 高 | 提前验证网络方案，平台到集群 API Server 网络可达 |
| 团队对 Argo CD/Workflows 不熟悉 | 中 | 中 | 前期做技术 spike，验证核心链路 |
| 前端工作量被低估 | 中 | 中 | MVP 阶段前端只做核心功能，管理类操作可用 API |
