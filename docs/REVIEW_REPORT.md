# 联合评审报告

| 字段 | 内容 |
|------|------|
| 评审日期 | 2026-07-10 |
| 参与角色 | 运维主管、开发工程师、项目主管、安全专家、架构师 |
| 文档版本 | v1.0（评审对象） |

---

## 评审问题汇总

共收集 **38 个问题**，去重合并后为 **22 个独立问题**，按优先级分类：

### 🔴 P0 — 必须在文档定稿前解决（9 个）

| # | 问题 | 提出者 | 根因 |
|---|------|--------|------|
| 1 | 加密主密钥（MEK）存储方案缺失 | 安全专家、运维主管、架构师、PM | SEC-01 只说了加密算法，没说密钥存哪 |
| 2 | OIDC Session/Token 管理设计缺失 | 安全专家 | 缺少 logout/refresh 端点、token 存储策略、过期机制 |
| 3 | 审计日志防篡改能力不足 | 安全专家 | 审计日志和业务数据在同一 DB，DB 管理员可改 |
| 4 | P0 功能与实施计划 Phase 顺序矛盾 | PM | B-01/V-01 标为 P0 但排在 Phase 4，部署链路 Phase 2 需要镜像 |
| 5 | 缺少人力配置假设 | PM | 无法评估排期合理性 |
| 6 | 集群日常运维操作完全缺失 | 运维主管 | 只有注册和健康概览，缺 cordon/drain/pod 排障等 |
| 7 | 平台自身部署架构未定义 | 运维主管、架构师 | 平台跑在哪？PG/Redis 部署在哪？负载均衡？ |
| 8 | 缺少回滚策略详细设计 | 架构师 | 什么条件触发回滚？回滚用 Argo CD rollback 还是 Git revert？prod 回滚需不需要审批？ |
| 9 | 缺少通知实体数据模型 | 架构师 | 通知中心有需求但数据模型没有 Notification/WebhookConfig 实体 |

### 🟡 P1 — 设计阶段必须解决（8 个）

| # | 问题 | 提出者 |
|---|------|--------|
| 10 | API 缺少统一分页/过滤/错误格式约定 | 开发工程师 |
| 11 | 部署锁生命周期未定义（TTL、超时释放、部署与配置变更互斥） | 架构师、PM |
| 12 | Argo CD 状态对账机制缺失（平台状态与 Argo CD 实际状态可能脱漂） | 架构师 |
| 13 | 部署日志内容来源未明确 | 开发工程师 |
| 14 | 配置 dry-run / 预览能力缺失 | 开发工程师 |
| 15 | PG 高可用 failover 方案未明确（自动还是手动） | 运维主管 |
| 16 | Git 并发写入瓶颈未量化（100+ 服务并行部署时 Git push 冲突） | 架构师 |
| 17 | 实施计划缺少里程碑 DoD 和服务迁移阶段 | PM |

### 🟢 P2 — 后续迭代解决（5 个）

| # | 问题 | 提出者 |
|---|------|--------|
| 18 | 灰度发布（蓝绿/金丝雀）架构扩展点预留 | 架构师 |
| 19 | 镜像签名验证（Cosign） | 安全专家 |
| 20 | 物理机部署的 GitOps 一致性 | 架构师 |
| 21 | Webhook 通知投递保障（重试/死信队列） | 运维主管 |
| 22 | 网络安全总体方案（网络分区、mTLS） | 安全专家 |

---

## 解决方案

### 方案 1：密钥管理（解决问题 #1）

**方案**：Phase 1 使用云 KMS 信封加密（Envelope Encryption），不上 Vault 但不裸存密钥。

```
明文凭证 → AES-256-GCM 加密 → 密文存 PostgreSQL
                  ↑
         数据加密密钥 (DEK)
                  ↑
    AES 加密 / KMS API 解密
                  ↑
         主加密密钥 (MEK) 存放在 KMS
```

- MEK 存放在云 KMS（阿里云 KMS / AWS KMS），平台不持有明文 MEK
- 每次 DEK 使用时通过 KMS API 解密
- 新增 ADR-0009 记录此决策

### 方案 2：OIDC Session 管理（解决问题 #2）

补充 API 端点和安全策略：

```
POST   /api/v1/auth/login          # OIDC 重定向（含 PKCE）
GET    /api/v1/auth/callback       # OIDC 回调
POST   /api/v1/auth/refresh        # Refresh Token 刷新
POST   /api/v1/auth/logout         # OIDC RP-Initiated Logout
GET    /api/v1/auth/me             # 当前用户 + 权限
```

| 维度 | 策略 |
|------|------|
| Token 存储 | HttpOnly + Secure + SameSite=Strict Cookie |
| Access Token TTL | 15 分钟 |
| Refresh Token TTL | 7 天，支持 Rotation + Reuse Detection |
| 权限缓存 TTL | Redis 缓存 5 分钟，用户禁用时主动删除 |
| OIDC Provider 不可用 | 降级为只读模式（已有 session 仍可查询，写操作拒绝） |

### 方案 3：审计日志防篡改（解决问题 #3）

**方案**：审计日志写入独立 PostgreSQL 表（`audit_log`），配合应用层限制：

- 表级权限：应用账号对 `audit_log` 只有 INSERT + SELECT 权限，无 UPDATE/DELETE
- Hash chain：每条日志包含前一条记录的 hash（`prev_hash`），可检测篡改
- 定期导出：每日 cron 将审计日志导出到外部存储（对象存储 / SIEM）

### 方案 4：P0 范围与 Phase 顺序调整（解决问题 #4）

**调整**：

1. B-01/V-01 从 P0 降为 P1 — 平台 MVP 不含构建链路，版本通过手动注册或 API 导入
2. Phase 顺序调整：构建链路（Phase 4）移到部署链路（Phase 2）之后，无前置依赖问题
3. P0 范围从 19 项缩减为 15 项

调整后 P0 清单：
- D-01, D-03, D-04, D-05（部署核心）
- C-01（配置变更）
- M-01（健康状态）
- K-01（集群注册）
- A-01, A-02（认证权限）
- AU-01（审计）
- AP-01, AP-02（审批 + 通知）
- N-01, N-02（通知）
- SEC-01, SEC-02（密钥存储）

### 方案 5：人力配置假设（解决问题 #5）

| 角色 | 人数 | 职责 |
|------|------|------|
| 后端工程师 | 2 | Go API Server + 业务逻辑 + 基础设施对接 |
| 前端工程师 | 1 | Next.js SPA 全部 UI |
| DevOps/SRE | 0.5（兼职） | Argo CD/Workflows 部署、GitOps 仓库搭建 |
| 测试工程师 | 0.5（兼职） | 后期介入，端到端测试 |

### 方案 6：集群运维操作（解决问题 #6）

补充需求 K-03 ~ K-06：

| ID | 功能项 | 优先级 | 描述 |
|----|--------|--------|------|
| K-03 | Pod 列表与详情 | P1 | 查看 Pod 状态、events |
| K-04 | Pod 日志查看 | P1 | 在线查看容器 stdout 日志 |
| K-05 | 节点管理 | P2 | cordon / drain / uncordon |
| K-06 | 命名空间管理 | P2 | 创建 / 清理 namespace |

### 方案 7：平台部署架构（解决问题 #7）

新增架构文档章节"平台部署拓扑"：

```
管理集群 (Management Cluster)          业务集群 (Business Cluster)
┌──────────────────────────┐          ┌─────────────────────────┐
│  Go API (2 replicas)     │          │                         │
│  Next.js (静态托管)       │    ───→  │  微服务 (100+)           │
│  PostgreSQL (主从)        │    Argo  │  Argo CD Agent          │
│  Redis (Sentinel × 3)    │    CD    │                         │
│  Argo CD Server          │    API   │                         │
│  Argo Workflows          │          │                         │
└──────────────────────────┘          └─────────────────────────┘
```

- 平台组件部署在独立管理集群（或管理 namespace，资源隔离）
- 通过 Argo CD API 管理业务集群（单集群场景下同集群不同 namespace）
- 前端静态文件由 Go 后端托管或 CDN 分发

### 方案 8：回滚策略设计（解决问题 #8）

新增 ADR-0010 记录回滚策略：

| 维度 | 策略 |
|------|------|
| 触发条件 | Argo CD Application 状态为 Degraded 持续超过 5 分钟 |
| 触发方式 | 手动触发（默认） / 自动触发（可选，按服务配置） |
| 回滚操作 | `argocd app rollback`（回到上一个 healthy sync），不改 Git |
| Git 同步 | 回滚后平台自动 Git commit 同步状态（异步） |
| Prod 审批 | 自动回滚不需要审批（紧急恢复优先）；手动回滚到指定历史版本需审批 |
| 成功判定 | Argo CD Application 状态恢复 Healthy |

### 方案 9：通知数据模型（解决问题 #9）

补充实体：

```
WebhookConfig
  id, name, url, secret, events[] (approval/deployment/build/alert),
  is_active, retry_count, retry_interval_sec, created_at

NotificationLog
  id, webhook_config_id, event_type, payload jsonb,
  status (pending/sent/failed), response_code, attempts,
  sent_at, created_at
```

### 方案 10：API 通用约定（解决问题 #10）

新增架构文档"API 通用约定"小节：

```
# 分页
GET /api/v1/services?page=1&page_size=20
响应: { data: [], pagination: { page, page_size, total } }

# 过滤
GET /api/v1/services?team=order&status=active&env=prod

# 排序
GET /api/v1/services?sort=-created_at

# 错误格式
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "service_name is required",
    "details": [{ "field": "service_name", "issue": "required" }]
  }
}
```

### 方案 11：部署锁生命周期（解决问题 #11）

| 维度 | 策略 |
|------|------|
| 锁 Key | `deploy:lock:{service_id}:{environment_id}` |
| TTL | 10 分钟（部署通常 < 5min，留 buffer） |
| 自动释放 | TTL 到期自动释放；部署完成后主动释放 |
| 强制释放 | operator 角色可强制释放（记录审计） |
| 互斥范围 | 部署和配置变更共享同一把锁 |

### 方案 12：Argo CD 状态对账（解决问题 #12）

新增定时对账任务：

- **频率**：每 5 分钟执行一次
- **逻辑**：遍历所有 Deployment 记录，对比平台状态与 Argo CD Application 实际状态
- **偏差处理**：状态不一致时更新平台记录，标记 `sync_drift = true`，通知运维
- **Application 丢失**：检测到 Argo CD Application 被删除，平台自动重建

### 方案 13：实施计划调整（解决问题 #17）

| Phase | 工期 | DoD |
|-------|------|-----|
| Phase 1: 骨架 | 3 周 | OIDC 登录可用，RBAC 生效，DB migration 可运行 |
| Phase 2: 部署链路 | 4 周 | 能部署示例 Helm Chart 到 dev，状态实时可见，能回滚 |
| Phase 3: 审批审计配置 | 3 周 | prod 部署走审批，审计日志完整，配置变更生效 |
| Phase 4: 构建链路 | 3 周 | 能触发 Argo Workflow 构建，镜像推 Harbor，版本注册 |
| Phase 5: 迁移与外围 | 3 周 | 100+ 服务批量导入，Ansible 物理机部署，监控集成 |
| Phase 6: 稳定化 | 3 周 | 端到端测试，性能优化，文档，3 个试点服务上线 |

**总工期：19 周（约 5 个月）**，含 20% buffer。

---

## 执行计划

按以上方案更新三份文档（REQUIREMENTS.md / ARCHITECTURE.md / ADR），然后提交。
