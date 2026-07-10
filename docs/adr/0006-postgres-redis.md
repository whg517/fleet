# ADR-0006: 元数据存储用 PostgreSQL + Redis

## 状态

Accepted

## 背景

平台需要存储服务元数据、部署记录、审计日志、权限配置等结构化数据。
同时需要缓存部署状态和异步任务队列。

## 决策

- **PostgreSQL**：存储所有元数据、部署记录、审计日志
- **Redis**：部署状态缓存、异步任务队列、session

## 后果

### 正面
- PostgreSQL ACID 事务保证，审计日志写入可靠
- 关系型数据模型天然匹配服务/环境/部署的关联关系
- PostgreSQL jsonb 支持灵活的配置存储
- Redis 单线程模型避免并发问题
- Redis Sentinel 保证可用性

### 负面
- PostgreSQL 需要主从复制和备份策略
- Redis 内存成本，大量数据不适合放 Redis

### 中性
- 审计日志量大时需要考虑分区或归档策略

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| MySQL | 功能不如 PostgreSQL（jsonb、CTE、全文搜索） |
| MongoDB | 审计日志需要严格一致性，文档型不适合 |
| etcd | 不适合存储大量业务数据 |
| SQLite | 不支持并发写入 |
