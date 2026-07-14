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
