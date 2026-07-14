# ADR-0013: 数据库 ORM 采用 Ent

## 状态

Accepted

## 背景

ADR-0006 确定了 PostgreSQL 作为主数据库。
Go 后端需要选择数据库访问层方案。
平台数据模型有 15+ 实体，关系较复杂（服务/环境/部署/审批/审计等）。

## 决策

采用 **Ent**（Facebook/Meta 开源的 Go ORM）作为数据访问层。

## 后果

### 正面
- Schema-as-Code，类型安全，代码生成强类型 API
- 图查询能力天然支持复杂关联关系（Service → Deployment → Environment）
- Atlas 集成，平台自身 schema 变更有版本管理
- 与 Go 生态集成好（GraphQL、OpenAPI 等代码生成扩展）
- 查询性能好，支持 eager loading 避免 N+1

### 审计日志 hash chain 的特殊处理

审计日志（AU-01）要求 hash chain 防篡改：每条记录的 `hash = SHA256(prev_hash || content)`，且应用账号无 UPDATE/DELETE 权限（INSERT-only）。

写入流程需要在同一事务内：读上一条 hash（`SELECT ... FOR UPDATE` 行锁）→ 计算当前 hash → INSERT。Ent 不直接支持 `FOR UPDATE` 行锁，因此**审计日志写入采用原生 SQL**，不走 Ent。

```go
// internal/store/postgres/audit_log.go

func (s *AuditStore) Append(ctx context.Context, entry AuditEntry) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 原生 SQL：读最后一条 hash + FOR UPDATE 防并发断裂
    var prevHash *string
    err = tx.QueryRowContext(ctx,
        `SELECT hash FROM audit_logs ORDER BY id DESC LIMIT 1 FOR UPDATE`,
    ).Scan(&prevHash)
    if err != nil && err != sql.ErrNoRows {
        return err
    }

    hash := computeHash(prevHash, entry)

    _, err = tx.ExecContext(ctx,
        `INSERT INTO audit_logs (user_id, action, detail, prev_hash, hash, created_at)
         VALUES ($1, $2, $3, $4, $5, now())`,
        entry.UserID, entry.Action, entry.Detail, prevHash, hash,
    )
    if err != nil {
        return err
    }

    return tx.Commit()
}
```

### ORM 使用边界

| 场景 | Ent | 原生 SQL |
|------|:----:|:-------:|
| 普通 CRUD（Service、Deployment 等） | ✅ | |
| 复杂关联查询（诊断聚合 API） | ✅ | |
| 审计日志查询（多维度过滤） | ✅ | |
| 审计日志写入（hash chain + FOR UPDATE） | | ✅ |
| 部署锁（SELECT FOR UPDATE SKIP LOCKED） | | ✅ |

原则：Ent 覆盖 80% 常规场景，少数需要精细事务控制的用原生 SQL 封装在 store 层。对上层 domain 层透明，不感知底层实现。

### 负面
- 学习曲线比 GORM 高（需要理解 Schema 定义范式）
- 社区不如 GORM / sqlc 庞大
- 复杂查询场景需要写 raw SQL 或 edge traversal

### 中性
- 代码生成步骤需纳入 build 流程

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| GORM | 运行时反射性能差，类型安全弱，复杂查询容易出 bug |
| sqlc | 纯 SQL 方案，关系建模和动态查询不便 |
| sqlx + 原生 SQL | 模板代码多，维护成本高 |
