# ADR-0008: 基于 Casbin 的 RBAC 权限模型

## 状态

Accepted（2026-07-14 重新设计，替换原简单 RBAC 方案）

## 背景

### 原方案问题

原 ADR-0008 采用简单四维矩阵 `(Role, Service, Environment, Action)`，经需求交叉评审发现 6 个缺口：

1. **审批人配置化（AP-01）**：需求要求"按服务/团队配置化指定审批人（非固定角色）"，原模型只有角色级 Permission，无法做到用户级审批人指定
2. **团队维度授权（A-03）**：100+ 服务逐个配权限不可行，需要按团队批量授权
3. **auditor 角色**：用户故事 OPS-E.1 需要，原模型未定义
4. **冻结权限分级（D-07）**：单服务冻结 vs 全局冻结权限未区分
5. **敏感配置查看（US-C.1）**：密钥明文查看需要独立权限控制
6. **权限矩阵视图（A-05）**：需要支持可视化和僵尸权限检测

### 核心诉求

- 按角色 + 团队 + 用户三个维度灵活授权
- 环境隔离（dev/test 开发可操作，pre/prod 仅特定角色）
- 审批人按服务/团队配置化指定
- 操作权限细粒度（冻结分级、敏感配置查看）
- 权限可审计、可可视化、可即时撤销

## 决策

采用 **Casbin** 作为权限引擎，基于 RBAC with domain 模型。

### 为什么选 Casbin

| 维度 | Casbin | 手写 RBAC | GORM-RBAC |
|------|--------|----------|-----------|
| 策略模型 | 支持 RBAC/ABAC/ACL 多种模型 | 单一模型 | 仅 RBAC |
| 域（Domain）支持 | 天然支持多域（环境作为域） | 需自己实现 | 不支持 |
| 角色继承 | 支持 G2 (role hierarchy) | 需自己实现 | 有限 |
| 策略热更新 | Watcher 机制，无需重启 | 需重启或缓存失效 | 需重启 |
| 性能 | 内存匹配，p99 < 1ms | 看实现 | DB 查询慢 |
| Go 生态 | 官方 Go 实现，成熟 | — | — |
| 策略存储适配器 | PostgreSQL/Redis/File 多适配器 | — | — |

### Casbin 模型定义

```
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _
g2 = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && \
    r.dom == p.dom && \
    keyMatch2(r.obj, p.obj) && \
    (p.act == r.act || p.act == "*")
```

**模型说明**：

| 符号 | 含义 | 示例 |
|------|------|------|
| `sub` | 主体（用户ID或角色） | `user:42`, `role:developer` |
| `dom` | 域（环境） | `dev`, `test`, `pre`, `prod`, `*` |
| `obj` | 资源对象（服务/集群/全局） | `service:order-service`, `cluster:prod`, `*` |
| `act` | 操作 | `deploy`, `view`, `config`, `config_secret`, `approve`, `freeze_service`, `freeze_global`, `admin` |

### 角色定义

| 角色 | 说明 | 基础权限 |
|------|------|---------|
| **admin** | 平台管理 | 所有环境所有操作 |
| **operator** | 运维工程师 | 所有环境 deploy/config/view/approve/freeze_service |
| **developer** | 开发工程师 | dev/test: deploy/config/view；pre/prod: view |
| **viewer** | 默认角色 | 所有环境 view |
| **auditor** | 审计员 | 所有环境 view + 审计日志查看 |

### 操作类型（细化）

| 操作 | 说明 | 拥有角色 |
|------|------|---------|
| `view` | 查看服务/部署/配置 | 所有角色 |
| `deploy` | 部署/回滚 | operator, developer(dev/test) |
| `config` | 修改 Helm values | operator, developer(dev/test) |
| `config_secret` | 查看敏感配置明文 | admin, operator |
| `approve` | 审批部署请求 | 按服务/团队配置化指定 |
| `freeze_service` | 冻结/解冻单个服务 | operator, admin |
| `freeze_global` | 批量冻结/环境级冻结 | admin only |
| `scale` | 扩缩容 | operator, developer(dev/test) |
| `admin` | 系统配置/用户管理/集群管理 | admin |

### 团队维度授权

利用 Casbin 的角色继承（g2），实现按团队批量授权：

```
# 团队作为角色组
g2, user:42, team:order-team          # 用户42属于交易团队
g2, user:43, team:order-team
g2, user:44, team:user-team            # 用户44属于用户团队

# 团队级权限策略
p, role:developer, dev, service:order-team:*, deploy    # 交易团队developer可在dev部署
p, role:developer, test, service:order-team:*, deploy
p, role:developer, dev, service:order-team:*, config
p, role:developer, dev, service:order-team:*, config
```

**Service 关联团队**：Service 数据模型已有 `owner_team` 字段，平台启动时自动同步团队→服务的映射关系到 Casbin 策略。

### 审批人配置化

审批人不再是固定角色，而是按服务/团队 + 环境配置：

```
# 服务级审批人
p, user:42, prod, service:order-service, approve     # 张三是order-service的prod审批人
p, user:43, prod, service:order-service, approve     # 李四也是

# 团队级审批人
p, role:team-lead:order-team, prod, service:order-team:*, approve  # 交易团队TL可审批团队所有服务
```

数据模型中新增 `ApproverConfig` 实体管理审批人配置，变更后同步到 Casbin 策略。

### 冻结权限分级

```
# operator 可以冻结单个服务
p, role:operator, *, service:*, freeze_service

# 只有 admin 可以批量冻结
p, role:admin, *, *, freeze_global
```

### 策略存储

- **热数据**：Casbin 策略加载到内存，请求时内存匹配（p99 < 1ms）
- **持久化**：PostgreSQL Casbin Adapter，策略变更写入 DB
- **缓存同步**：策略变更时通过 Redis Pub/Sub 通知所有实例重新加载策略（Enforcer Watcher 机制）
- **启动加载**：Go 后端启动时从 PG 全量加载策略到内存

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│ Go API (x2) │───▶│  Casbin      │───▶│ PostgreSQL   │
│             │    │  Enforcer    │    │ (策略持久化)  │
│  权限校验    │    │  (内存匹配)   │    │              │
└─────────────┘    └──────┬───────┘    └──────────────┘
                          │
                   ┌──────▼───────┐
                   │  Redis       │
                   │  Pub/Sub     │
                   │  (策略变更通知)│
                   └──────────────┘
```

### 权限即时撤销

结合 Redis 黑名单实现即时撤销：

1. 用户被禁用/降权时，其 user_id 加入 Redis 黑名单（TTL = token 剩余有效期）
2. Casbin 中间件在权限校验前先检查黑名单
3. 黑名单命中则直接拒绝，不进入 Casbin 匹配流程
4. 同时更新 Casbin 策略（移除该用户的 g/g2 关联），通过 Redis Pub/Sub 同步到所有实例

### 权限矩阵视图（A-05）

Casbin 的策略本身就是矩阵数据，查询 API：

```
# 查某用户在所有服务×环境的权限
GET /api/v1/permissions/users/:id/matrix

# 查某服务在所有用户×环境的权限
GET /api/v1/permissions/services/:id/matrix

# 僵尸权限检测：关联 AuditLog，找出有权限但 N 天无操作的用户
GET /api/v1/permissions/zombie-check?days=90
```

## 数据模型变更

### 新增/修改实体

```
User
  id, name, email, oidc_subject, status, created_at

Role
  id, name (admin / operator / developer / viewer / auditor), description

# Casbin 策略表（由 Casbin PG Adapter 管理，不手动操作）
CasbinRule
  id, ptype (p/g/g2), v0 (sub), v1 (dom), v2 (obj), v3 (act), v4, v5

# 审批人配置（业务层管理，同步到 Casbin）
ApproverConfig
  id, service_id, team_id, environment_id,
  approver_user_id,                # 指定审批人
  created_at, updated_at

# Service 已有 owner_team 字段
Service
  id, name, ..., owner_team        # 关联团队

# 冻结状态
FreezeRecord
  id, scope (service / environment / global),
  service_id, environment_id,
  reason text, frozen_by, created_at, expires_at
```

### 移除原 Permission 表

原 `Permission(role_id, resource_type, resource_id, environment, actions[])` 表移除，权限策略由 Casbin 管理。

## 后果

### 正面

- Casbin 成熟可靠，Go 生态官方支持
- RBAC with domain 模型天然支持环境隔离
- 角色继承支持团队批量授权，100+ 服务不用逐个配
- 策略热更新（Redis Pub/Sub），权限变更秒级生效
- 审批人配置化，不再绑死固定角色
- 操作类型细化，冻结分级、敏感配置独立控制
- 权限矩阵查询天然支持，策略数据即矩阵数据

### 负面

- 引入 Casbin 依赖，团队需学习模型定义语法
- 策略内存加载占内存（100+ 服务 × 4 环境 × 多角色 ≈ 数万条策略，内存可控 < 50MB）
- 调试复杂度增加（权限不通过时需要排查 Casbin 策略链）

### 中性

- Casbin 策略模型可后续从 RBAC 扩展到 ABAC，无需换框架
- 策略存储适配器可切换（PG → Redis → File），灵活

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 原手写 RBAC 四维矩阵 | 无法支持用户级授权、团队批量授权、审批人配置化 |
| ABAC (基于属性) | 过于复杂，Casbin 可后续平滑扩展 |
| OPA (Open Policy Agent) | 引入独立服务，运维复杂度过高 |
| Auth0/Casdoor 等外部 IAM | 引入外部依赖，当前阶段不需要 |

## 实现要点

1. **M1 阶段**：集成 Casbin Go SDK，定义 RBAC with domain 模型，实现基础 5 角色权限校验
2. **M2 阶段**：团队→服务映射同步，部署权限校验接入
3. **M3 阶段**：审批人配置化管理界面，权限矩阵视图，僵尸权限检测
4. **中间件设计**：Echo 中间件统一拦截，从 JWT 提取 user_id → 检查 Redis 黑名单 → Casbin Enforce → 放行或拒绝
