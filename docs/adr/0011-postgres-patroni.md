# ADR-0011: PostgreSQL 自动 failover 采用 Patroni

## 状态

Accepted

## 背景

ADR-0006 确定了 PostgreSQL 作为主数据库。
架构文档 §10 提到"主从复制 + PITR"，但未明确 failover 方案。
平台 SLA 要求 99.5%（< 3.6h/月停机），手动 failover 的 MTTR 可能超出预算。

## 决策

采用 Patroni 实现 PostgreSQL 自动 failover。

- Patroni 管理 PostgreSQL 主从切换
- etcd（3 节点）作为 DCS（Distributed Configuration Store）
- 主库故障时 Patroni 自动将从库提升为主库
- pgBackRest 做物理备份 + WAL 归档，支持 PITR
- 目标 RPO < 1 分钟，RTO < 2 分钟

## 后果

### 正面
- 自动 failover，满足 99.5% SLA
- Patroni 社区成熟，广泛使用
- pgBackRest 备份可靠

### 负面
- 引入 etcd + Patroni，运维复杂度增加
- etcd 需要奇数节点（至少 3 个），资源开销

### 中性
- Patroni 配置需要根据实际负载调优

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| 手动 failover | MTTR 不可控，可能超 SLA |
| Stolon | 社区活跃度不如 Patroni |
| RePMgr | 自动 failover 能力不如 Patroni |
| 云托管 RDS | 成本高，依赖云厂商 |
