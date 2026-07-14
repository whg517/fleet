# ADR-0011: PostgreSQL 高可用（已移除）

## 状态

**Deprecated** — 2026-07-12 移除

## 背景

平台 SLA 要求 99.5%，PostgreSQL 高可用方案需要明确。

## 决策变更

当前阶段移除 Patroni + etcd 自动 failover 方案。

**原因：**
- 平台处于初期建设阶段，引入 Patroni + etcd（3 节点）运维成本过高
- 单实例 PostgreSQL + 定期备份足以满足当前需求
- DB 高用方案在后续按需引入

**当前方案：**
- PostgreSQL 单实例部署
- pgBackRest 定期物理备份 + WAL 归档
- 故障时通过备份恢复（RTO 可接受范围内）

## 历史决策（已废弃）

~~采用 Patroni 实现 PostgreSQL 自动 failover~~

## 后续

如果后续有高可用需求，再重新评估 Patroni / Stolon / 云托管 RDS 等方案。
