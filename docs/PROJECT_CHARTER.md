# 服务管理平台立项文档

| 字段 | 内容 |
|------|------|
| 项目名称 | 服务管理平台（暂定名：Platform） |
| 文档版本 | v1.0 |
| 创建日期 | 2026-07-10 |
| 状态 | Draft — 待评审 |

---

## 1. 项目背景

### 1.1 现状

- 微服务 100+ 且持续增长，全部运行在 K8s 上，个别服务部署在物理机
- 服务部署、配置变更、版本管理主要依赖人工操作或零散脚本
- 缺乏统一的服务生命周期管理入口
- 多环境（dev / test / pre / prod）的发布流程不规范，依赖个人经验
- 操作审计和变更追溯能力薄弱

### 1.2 问题

| 问题域 | 具体痛点 |
|--------|---------|
| 部署效率 | 手工操作多、重复劳动、容易出错 |
| 环境管理 | 环境间配置差异不透明，promoting 困难 |
| 版本追踪 | 不知道某个环境跑的什么版本，回滚靠记忆 |
| 权限控制 | 谁能操作 prod 没有统一管控 |
| 审计合规 | 操作无记录，出问题难追溯 |
| 服务可见性 | 架构师看不到全局拓扑和版本分布 |

### 1.3 目标

构建统一的服务管理平台，覆盖服务完整生命周期：

```
构建打包 → 版本管理 → 多环境部署 → 配置变更 → 监控 → 扩缩容 → 服务下线
```

同时提供集群管理、OCI 管理、认证权限、审计、发布审批等平台能力。

---

## 2. 干系人分析

| 角色 | 核心诉求 | 使用场景 |
|------|---------|---------|
| **运维工程师** | 全权限管理所有服务和基础设施 | 集群纳管、部署操作、故障排查、资源治理 |
| **开发工程师** | 自助管理自己的服务 | 部署到 dev/test、查看日志和监控、修改配置 |
| **架构师** | 全局视角掌握系统状态 | 查看服务拓扑、版本分布、依赖关系、技术债识别 |

---

## 3. 功能范围

### 3.1 功能清单

| 模块 | 功能项 | 优先级 | 说明 |
|------|--------|--------|------|
| **构建打包** | CI 触发与跟踪 | P0 | 对接 Argo Workflows，平台触发并跟踪构建状态 |
| | 构建模板管理 | P1 | 管理 WorkflowTemplate，参数化触发 |
| **版本管理** | 镜像版本注册 | P0 | 构建完成后自动注册到平台 |
| | 版本历史与 changelog | P1 | 关联 git commit，记录变更内容 |
| **多环境部署** | Helm 部署（K8s） | P0 | 通过 Argo CD Application 管理 Helm Chart 部署 |
| | Ansible 部署（物理机） | P1 | 通过 Ansible Runner 部署到物理机/VM |
| | 环境配置 override | P0 | 每个环境独立的 values override |
| | 部署状态跟踪 | P0 | 实时获取 Argo CD sync 状态 |
| | 回滚 | P0 | 回滚到上一个成功版本 |
| **配置变更** | Helm values 修改 | P0 | 环境级配置变更，走审批+审计 |
| | 配置版本历史 | P1 | 配置变更记录，支持 diff |
| **监控** | 服务健康状态 | P0 | 对接 Prometheus，展示服务运行状态 |
| | 指标查询 | P1 | 集成 Grafana 或内嵌指标图表 |
| | 告警集成 | P1 | 对接 AlertManager |
| **扩缩容** | 手动扩缩 | P1 | 修改 replicas |
| | HPA 策略管理 | P2 | CRUD HorizontalPodAutoscaler |
| **服务下线** | 安全下线流程 | P2 | 清理 K8s 资源、Argo CD Application、镜像 |
| **集群管理** | 多集群注册 | P0 | 管理多个 K8s 集群 kubeconfig |
| | 集群健康概览 | P1 | 节点状态、资源利用率 |
| **OCI 管理** | 镜像仓库对接 | P0 | 对接 Harbor，查询镜像列表 |
| | 镜像扫描 | P1 | 展示 Trivy 扫描结果 |
| | 镜像清理策略 | P2 | 设置保留策略，自动清理旧版本 |
| **认证** | OIDC 对接 | P0 | 对接企业 SSO |
| **权限** | RBAC | P0 | 用户 × 服务 × 环境 × 操作 权限矩阵 |
| **审计** | 操作日志 | P0 | 所有写操作记录 |
| | 变更追溯 | P1 | 按服务/环境/时间/操作人查询 |
| **发布审批** | 单级审批 | P0 | pre/prod 部署需指定角色审批 |
| | 审批通知 | P0 | 飞书/邮件通知审批人 |

### 3.2 不做的事（Non-Goals）

| 不做 | 原因 |
|------|------|
| 自建 CI 引擎 | 构建由 Argo Workflows 执行，平台只触发和跟踪 |
| 自建监控系统 | 对接现有 Prometheus，不重复造轮子 |
| 工作流引擎 | 审批是单级的，不做复杂流程编排 |
| 日志聚合平台 | 对接现有 Loki/ES，平台只做跳转和查询入口 |
| 代码仓库 | Git 由现有 GitLab 管理 |
| 多租户隔离 | 当前阶段单组织，后续按需演进 |

---

## 4. 技术架构

### 4.1 技术选型

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

### 4.2 系统架构图

```
┌──────────────────────────────────────────────────────────────┐
│                    Next.js SPA (前端)                         │
│   服务目录 · 拓扑图 · 部署中心 · 监控面板 · 审计日志            │
├──────────────────────────────────────────────────────────────┤
│                     Go API Server                             │
│          REST/WebSocket · OIDC · RBAC · 审计拦截              │
├────────┬─────────┬────────┬────────┬───────┬────────┬────────┤
│ 服务   │ 环境     │ 构建    │ 部署   │ 集群  │ 配置   │ 审批   │
│ 目录   │ 管理     │ 触发    │ 编排   │ 纳管  │ 变更   │ 网关   │
├────────┴─────────┴────────┴────────┴───────┴────────┴────────┤
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
│   │ Harbor       │   │ K8s Clusters │   │ 物理机 / VM  │    │
│   │ (镜像制品)    │   │ dev/test/    │   │              │    │
│   │              │   │ pre/prod     │   │              │    │
│   └──────────────┘   └──────────────┘   └──────────────┘    │
├──────────────────────────────────────────────────────────────┤
│                 可观测层 (Observability)                      │
│           Prometheus · AlertManager · Loki/ES               │
└──────────────────────────────────────────────────────────────┘
```

### 4.3 核心设计原则

1. **平台是控制面，不是数据面** — 平台挂了不影响线上服务运行
2. **GitOps 模式** — 部署的期望状态存储在 Git，Argo CD 负责调和
3. **模板化封装** — 服务部署逻辑封装在 Helm Chart / Ansible Role 中，平台只管契约
4. **一切操作可审计** — 所有写操作记录 who/when/what/result

### 4.4 关键流程

#### 构建流程

```
Git Push → Webhook → 平台触发 Argo Workflow → 构建并推镜像到 Harbor → 回调平台注册版本
```

#### K8s 部署流程

```
选择服务+版本+环境
  → 权限校验
  → [prod] 审批流程
  → 平台 Git commit 更新 values
  → 更新 Argo CD Application
  → 触发 Sync
  → 轮询状态 (Healthy/Degraded)
  → 记录结果 + 审计
```

#### 物理机部署流程

```
选择服务+版本+目标主机
  → 权限校验
  → [prod] 审批流程
  → 平台调用 Ansible Runner
  → 收集结果
  → 记录 + 审计
```

#### 配置变更流程

```
修改环境级 Helm values
  → [prod] 审批
  → Git commit 到 environments/<env>/<service>.yaml
  → Argo CD 检测变更 → 自动 Sync
  → 配置生效
```

---

## 5. 数据模型（核心实体）

```
User
  id, name, email, oidc_subject, status

Role
  id, name (admin/operator/developer/viewer)

Permission
  role_id, resource_type (service/cluster/*), resource_id, environment, actions []

Cluster
  id, name, api_server, kubeconfig, environment, labels, status

Service (微服务)
  id, name, description, owner_team, deploy_type (k8s/vm), chart_name, status

ServiceVersion
  id, service_id, version, image_ref, git_commit, built_at, changelog

Environment
  id, name (dev/test/pre/prod), cluster_id, namespace_pattern, approval_required, approver_role

Deployment
  id, service_id, version_id, environment_id, status, params_json, initiated_by, approved_by, started_at, finished_at

ConfigSnapshot
  id, service_id, environment_id, values_json, created_by, created_at

Approval
  id, deployment_id, status (pending/approved/rejected), approver_id, decided_at

AuditLog
  id, user_id, action, resource_type, resource_id, detail_json, ip, created_at

Registry
  id, name, type (harbor), url, credentials_ref
```

---

## 6. API 概览

```
# 认证
POST   /api/v1/auth/login
GET    /api/v1/auth/callback
GET    /api/v1/auth/me

# 服务目录
GET    /api/v1/services
POST   /api/v1/services
GET    /api/v1/services/:id

# 版本
GET    /api/v1/services/:id/versions
POST   /api/v1/services/:id/versions/:ver/deploy

# 环境
GET    /api/v1/environments
GET    /api/v1/environments/:id/services

# 部署
POST   /api/v1/deployments
GET    /api/v1/deployments/:id
POST   /api/v1/deployments/:id/rollback
GET    /api/v1/deployments/:id/logs

# 集群
GET    /api/v1/clusters
POST   /api/v1/clusters
GET    /api/v1/clusters/:id/health

# OCI
GET    /api/v1/registries/:id/images
GET    /api/v1/registries/:id/scans

# 配置
GET    /api/v1/services/:id/config
PUT    /api/v1/services/:id/config

# 审批
GET    /api/v1/approvals
POST   /api/v1/approvals/:id/approve
POST   /api/v1/approvals/:id/reject

# 审计
GET    /api/v1/audit/logs

# 监控
GET    /api/v1/services/:id/health
GET    /api/v1/services/:id/metrics

# 扩缩容
PUT    /api/v1/services/:id/scale
PUT    /api/v1/services/:id/hpa
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
│   │   ├── service-a.yaml
│   │   └── service-b.yaml
│   ├── test/
│   ├── pre/
│   └── prod/
│
├── argocd-apps/              # Argo CD Application 定义
│   ├── dev/
│   ├── test/
│   ├── pre/
│   └── prod/
│
└── ansible/                  # 物理机部署
    ├── inventory/
    │   ├── dev.ini
    │   └── prod.ini
    ├── roles/
    │   └── service-xxx/
    └── playbooks/
```

---

## 8. 后端项目结构

```
cmd/
  server/main.go              # 入口

internal/
  api/                        # HTTP/WS handler
    router.go
    handler/
      service.go
      deployment.go
      cluster.go
      ...
    middleware/
      auth.go                 # OIDC
      rbac.go                 # 权限
      audit.go                # 审计

  domain/                     # 业务逻辑
    service/
    environment/
    deployment/               # 部署编排（核心）
    cluster/
    version/
    config/
    approval/
    audit/
    scaling/

  infra/                      # 基础设施对接
    argocd/                   # Argo CD API client
    argowf/                   # Argo Workflows API client
    harbor/                   # Harbor API client
    ansible/                  # Ansible runner 封装
    git/                      # Git 操作
    prometheus/               # 监控查询
    kube/                     # K8s API

  store/                      # 持久层
    postgres/
    redis/
    models/                   # 数据模型定义

  config/                     # 配置加载
  pkg/                        # 通用工具
```

---

## 9. 技术决策记录 (ADR)

### ADR-01: 后端采用 Go 单体架构

- **状态**：Accepted
- **决策**：后端使用 Go，单体起步，internal 按域模块分包
- **理由**：100+ 微服务的平台自身不需要是微服务架构；Go 并发模型适合编排操作；单二进制部署简单
- **备选**：Java（重）、Python（性能不如 Go）、微服务架构（过度设计）

### ADR-02: 前端采用 Next.js SPA

- **状态**：Accepted
- **决策**：Next.js + React + TypeScript，SPA 模式
- **理由**：团队选型，生态成熟，组件库丰富

### ADR-03: K8s 部署通过 Argo CD

- **状态**：Accepted
- **决策**：平台通过管理 Argo CD Application CRD 驱动部署，不直接操作 K8s API
- **理由**：GitOps 模式，期望状态存 Git；Argo CD 提供 diff/sync/rollback/health 能力；平台挂了线上不受影响
- **备选**：直接调 K8s API + Helm（放弃 GitOps 好处）

### ADR-04: 物理机部署通过 Ansible Role 模板

- **状态**：Accepted
- **决策**：物理机/VM 服务通过 Ansible Role 封装部署逻辑，平台调 Ansible Runner
- **理由**：Ansible 幂等、成熟；Role 模板化封装，平台只管传参数
- **备选**：自研 shell 脚本（不可维护）

### ADR-05: 构建通过 Argo Workflows

- **状态**：Accepted
- **决策**：平台触发 Argo Workflow 执行构建，不自建 CI
- **理由**：构建不是平台核心价值；Argo Workflows 与 K8s/Argo CD 生态一致
- **备选**：对接 Jenkins/GitLab CI（可后续扩展）

### ADR-06: 元数据存储用 PostgreSQL + Redis

- **状态**：Accepted
- **决策**：PostgreSQL 存储元数据、审计日志、部署记录；Redis 做缓存和异步队列
- **理由**：关系型数据需要事务保证；审计需要严格一致性的存储

### ADR-07: 认证采用 OIDC

- **状态**：Accepted
- **决策**：对接企业 SSO（OIDC 协议），不自建用户系统
- **理由**：统一身份管理，避免密码维护

### ADR-08: 权限模型采用 RBAC + 环境维度

- **状态**：Accepted
- **决策**：权限矩阵 `(User, Service, Environment, Action)`，按环境隔离
- **理由**：dev 开发可操作，prod 需审批，符合实际安全需求

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
| 前端工作量被低估 | 中 | 中 | MVP 阶段前端只做核心功能，管理类操作可用 API

---

## 13. 验收标准

| 维度 | 标准 |
|------|------|
| 部署能力 | 能通过平台部署 K8s 服务到 4 个环境，支持 Helm Chart |
| 物理机兼容 | 能通过平台部署 Ansible Role 到物理机 |
| 审批 | prod 环境部署必须经过审批 |
| 权限 | 不同角色只能执行权限范围内的操作 |
| 审计 | 所有部署/配置变更操作可追溯 |
| 回滚 | 能一键回滚到上一个成功版本 |
| 状态可见 | 能实时看到部署进度和服务健康状态 |
| 多集群 | 能纳管多个 K8s 集群并路由部署 |
