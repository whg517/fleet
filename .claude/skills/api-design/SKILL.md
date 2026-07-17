---
name: api-design
description: "Fleet 平台 HTTP API 设计规范：路由约定、请求/响应格式、错误码、分页、WebSocket。新增或修改 API 时使用。"
---

# API Design: Fleet 平台 HTTP API 设计规范

## 触发场景

- 新增 domain 模块，需要设计 API 路由
- 为现有模块添加新接口
- Review API 设计的一致性
- 编写 API 文档

## 前置阅读

- `docs/ARCHITECTURE.md` §6（现有 API 路由总览）
- `docs/REQUIREMENTS.md`（对应功能需求）

---

## 1. RESTful 约定

### 1.1 URL 规则

- 路径前缀：`/api/v1/`
- 资源用**复数名词**：`/api/v1/services`、`/api/v1/deployments`
- 资源层级用嵌套：`/api/v1/services/:id/versions`
- 标准方法映射：

| HTTP Method | 用途 | 示例 |
|-------------|------|------|
| GET | 列表 / 详情 | `GET /api/v1/services` |
| POST | 创建 / 动作 | `POST /api/v1/services` |
| PUT | 全量更新 | `PUT /api/v1/services/:id` |
| PATCH | 部分更新 | `PATCH /api/v1/services/:id` |
| DELETE | 删除 / 下线 | `DELETE /api/v1/services/:id` |

### 1.2 动作类接口

非 CRUD 操作用动词子路径：

```
POST /api/v1/deployments/:id/rollback      # 回滚
POST /api/v1/services/:id/config/dry-run   # 预览
POST /api/v1/approvals/:id/approve         # 审批通过
POST /api/v1/services/:id/freeze           # 冻结
```

### 1.3 HTTP 状态码

| Status | 场景 |
|--------|------|
| 200 | 成功（GET / PUT / PATCH / POST-action） |
| 201 | 创建成功（POST） |
| 204 | 无内容（DELETE） |
| 400 | 请求参数错误 |
| 401 | 未认证 / Token 过期 |
| 403 | 无权限 / 服务冻结 |
| 404 | 资源不存在 |
| 409 | 资源冲突（部署锁） |
| 422 | 业务校验失败 |
| 500 | 服务器内部错误 |

---

## 2. 统一响应格式

### 2.1 成功 — 单资源

```json
{
  "data": {
    "id": "svc-001",
    "name": "user-service",
    "status": "active"
  }
}
```

### 2.2 成功 — 列表 + 分页

```json
{
  "data": [
    { "id": "svc-001", "name": "user-service" }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100
  }
}
```

### 2.3 错误

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "service_name is required",
    "details": [
      { "field": "name", "issue": "required" }
    ]
  }
}
```

### 2.4 分页参数

查询参数：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `page` | 1 | 页码，从 1 开始 |
| `page_size` | 20 | 每页条数，最大 100 |
| `sort` | `-created_at` | 排序字段，`-` 前缀降序 |
| `q` | - | 搜索关键词（服务列表支持） |

---

## 3. 错误码定义

| Code | HTTP | 说明 | 触发场景 |
|------|------|------|----------|
| `VALIDATION_ERROR` | 400 | 请求参数校验失败 | 必填字段缺失、格式错误 |
| `UNAUTHORIZED` | 401 | 未登录或 Token 过期 | OIDC Token 失效 |
| `FORBIDDEN` | 403 | 无操作权限 | RBAC 拒绝 |
| `NOT_FOUND` | 404 | 资源不存在 | ID 无效 |
| `CONFLICT` | 409 | 资源冲突 | 重复创建 |
| `LOCK_CONFLICT` | 409 | 部署锁冲突 | 同服务同环境并发部署 |
| `SERVICE_FROZEN` | 403 | 服务已冻结 | 冻结期内操作 |
| `INTERNAL` | 500 | 服务器内部错误 | 未捕获异常 |

### Go 错误处理模式

```go
// domain 层定义 sentinel error
var (
    ErrNotFound      = errors.New("deployment not found")
    ErrLockConflict  = errors.New("deployment lock conflict")
    ErrServiceFrozen = errors.New("service is frozen")
)

// handler 层转换为 HTTP 响应
func handleError(w http.ResponseWriter, err error) {
    switch {
    case errors.Is(err, deployment.ErrNotFound):
        writeAPIError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
    case errors.Is(err, deployment.ErrLockConflict):
        writeAPIError(w, http.StatusConflict, "LOCK_CONFLICT", err.Error())
    case errors.Is(err, deployment.ErrServiceFrozen):
        writeAPIError(w, http.StatusForbidden, "SERVICE_FROZEN", err.Error())
    default:
        writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
    }
}
```

---

## 4. Handler 编写规范

### 4.1 薄 Handler 原则

Handler 只做四件事：

1. 请求解析 + 参数校验
2. 调用 domain service
3. 错误转换
4. 响应序列化

**禁止在 handler 中写业务逻辑**。

### 4.2 Handler 模板

```go
// internal/api/handler/deployment.go

type DeploymentHandler struct {
    svc *deployment.Service
}

func (h *DeploymentHandler) Create(w http.ResponseWriter, r *http.Request) {
    var req deployment.CreateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
        return
    }

    // 从 context 获取认证信息
    ctx := r.Context()
    userID := auth.UserIDFromContext(ctx)

    result, err := h.svc.Deploy(ctx, userID, req)
    if err != nil {
        handleError(w, err)
        return
    }

    writeAPISuccess(w, http.StatusCreated, result)
}
```

---

## 5. 路由注册

### 5.1 模块化注册

每个 domain 模块注册自己的路由组：

```go
// internal/api/router.go

func RegisterRoutes(mux *http.ServeMux, deps *Dependencies) {
    // 中间件链
    chain := middleware.Chain(
        middleware.Recover,
        middleware.CORS,
        middleware.Auth(deps.OIDC),
        middleware.RBAC(deps.Casbin),
        middleware.Audit(deps.AuditLogger),
    )

    // 认证（不需要认证中间件）
    mux.Handle("POST /api/v1/auth/login", chain(auth.NewLoginHandler(deps.Auth)))

    // 服务目录
    mux.Handle("GET /api/v1/services", chain(service.NewListHandler(deps.Service)))
    mux.Handle("POST /api/v1/services", chain(service.NewCreateHandler(deps.Service)))

    // 部署
    mux.Handle("POST /api/v1/deployments", chain(deploy.NewCreateHandler(deps.Deploy)))
    mux.Handle("POST /api/v1/deployments/{id}/rollback", chain(deploy.NewRollbackHandler(deps.Deploy)))
}
```

---

## 6. WebSocket 规范

### 6.1 连接

```
WS /api/v1/ws?token=<oidc_token>
```

### 6.2 频道订阅

```json
// 客户端订阅
{ "action": "subscribe", "channel": "deployment:svc-001:prod" }

// 服务端推送
{ "channel": "deployment:svc-001:prod", "event": "status_changed", "data": { "status": "healthy" } }
```

### 6.3 频道命名

```
deployment:<service_id>:<environment>
approval:<service_id>:<environment>
notification:<user_id>
```

---

## 7. API 版本化

- URL 路径版本：`/api/v1/...`
- 不兼容变更递增主版本号（`/api/v2/...`）
- 兼容变更在同一版本内追加（新增字段、新增端点）

---

## 8. 新增 API 检查清单

设计新 API 时逐条确认：

- [ ] URL 符合 RESTful 约定（复数名词、正确层级）
- [ ] 使用正确的 HTTP Method 和 Status Code
- [ ] 响应格式遵循统一结构（data / pagination / error）
- [ ] 错误码已定义或补充
- [ ] Handler 是薄层，业务逻辑在 domain service
- [ ] 需要 RBAC 权限检查（中间件或 handler 内）
- [ ] 写操作需要审计日志（中间件自动记录）
- [ ] 分页接口支持 page / page_size / sort 参数
- [ ] WebSocket 事件遵循频道命名规范
- [ ] 更新 ARCHITECTURE.md §6 的路由表
