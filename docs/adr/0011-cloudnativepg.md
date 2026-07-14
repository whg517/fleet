# ADR-0011: PostgreSQL 高可用采用 CloudNativePG

## 状态

Accepted

## 背景

平台 SLA 要求 99.5%（月度允许计划外停机 < 3.6h）。PostgreSQL 是平台核心依赖——存储服务元数据、部署记录、审计日志，PG 不可用则平台不可用。

虽然平台是控制面（故障不影响线上服务运行），但快速恢复能力仍然重要。需要选择一个运维成本合理的高可用方案。

### 方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| 单实例 + 备份 | 最简单 | RTO 长（恢复需数十分钟），故障期间平台完全不可用 |
| Patroni + etcd | 成熟、社区大 | 需要额外维护 etcd 集群，组件多，运维复杂 |
| Stolon | K8s 原生 | 社区活跃度下降，架构复杂 |
| **CloudNativePG** | **K8s 原生 Operator，无需 etcd，社区活跃** | 相对较新，但 CNCF 生态成熟 |
| 云托管 RDS | 全托管 | 当前为自建部署，暂不适用 |

## 决策

采用 **CloudNativePG (CNPG)** 管理 PostgreSQL 高可用。

### 架构

```
CloudNativePG Operator (集群级)
  └── Fleet PG Cluster (命名空间级)
        ├── Primary 实例（读写）
        ├── Replica 实例 ×1（同步复制，读）
        └── 备份调度
              ├── WAL 持续归档 → 对象存储 (S3/MinIO)
              └── 定期全量备份 → 对象存储
```

### 关键特性

| 维度 | 策略 |
|------|------|
| 复制 | 流复制（synchronous），Primary + 1 Replica |
| 故障切换 | Operator 自动检测 Primary 故障，提升 Replica（RTO < 30s） |
| 备份 | WAL 持续归档 + 定期全量（barman-cloud），存储到 S3/MinIO |
| 恢复 | 支持 PITR（Point-in-Time Recovery） |
| 连接路由 | 通过 K8s Service（读写指向 Primary，只读指向 Replica） |
| 版本管理 | Operator 管理 PG 大版本升级（in-place /蓝绿） |

## 后果

### 正面

- K8s 原生 Operator 模式，声明式管理，无需额外组件（不依赖 etcd）
- 自动 failover，RTO < 30s，远优于单实例恢复
- WAL 归档 + 全量备份支持 PITR，RPO 可控在秒级
- 读写分离天然支持（读请求可走 Replica）
- 社区活跃，文档完善，CNCF 生态
-备份到对象存储，成本低且可靠

### 负面

- Operator 本身需要维护（但比 Patroni + etcd 简单得多）
- Replica 占用额外资源（1 份 PG 实例资源）
- 对存储性能敏感（同步复制要求低延迟磁盘）
- 相比成熟方案（Patroni）生产案例较少，但快速增长中

## 参考

- CloudNativePG 官方文档：https://cloudnative-pg.io/
- CNPG GitHub：https://github.com/cloudnative-pg/cloudnative-pg
