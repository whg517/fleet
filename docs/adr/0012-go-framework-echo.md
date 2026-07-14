# ADR-0012: Go 后端框架选型 Echo

## 状态

Accepted

## 背景

ADR-0001 确定了 Go 单体后端，需要选择 HTTP 框架。
平台需要 REST API、WebSocket、中间件（认证/审计/RBAC）能力。

## 决策

采用 **Echo** 作为 HTTP 框架。

## 后果

### 正面
- 轻量高性能，路由匹配快
- 中间件生态完善（CORS、JWT、Rate Limiter、Request ID）
- WebSocket 支持良好
- 请求绑定/校验简洁（struct tag 方式）
- 文档清晰，社区活跃

### 负面
- 生态不如 Gin 庞大（但差距不大）
- 部分中间件需要自行实现

### 中性
- 团队需要熟悉 Echo 的中间件链机制

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| Gin | 生态最大，但 WebSocket 支持不如 Echo 原生 |
| Chi | 极简但生态小，中间件少 |
| 标准库 net/http | 功能太少，需要大量自建 |
| Fiber | 基于 fasthttp，与标准库不兼容，生态受限 |
