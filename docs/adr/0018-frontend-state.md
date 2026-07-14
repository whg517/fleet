# ADR-0018: 前端状态管理采用 TanStack Query + Zustand

## 状态

Accepted

## 背景

ADR-0002 确定了 Next.js SPA 前端。管理控制台需要管理两类状态：
- **服务端状态**：API 数据（服务列表、部署状态、审计日志等），需要缓存、失效、重试、乐观更新
- **客户端状态**：UI 交互状态（侧边栏、弹窗、筛选条件、多步表单等）

将两类状态混在一起管理会导致代码复杂、数据不一致。

## 决策

采用 **TanStack Query + Zustand** 组合方案：

- **TanStack Query**：管理所有服务端数据状态
  - API 请求缓存、自动重试、数据失效
  - 乐观更新（部署操作）
  - WebSocket 数据与缓存集成
  - 分页/无限滚动场景

- **Zustand**：管理客户端 UI 状态
  - 全局 UI 状态（侧边栏折叠、主题等）
  - 页面级交互状态（筛选条件、选中项等）
  - 多步表单/向导状态

## 后果

### 正面
- 关注点分离，服务端数据和客户端状态各司其职
- TanStack Query 自动处理缓存/失效/重试，大幅减少模板代码
- Zustand API 极简（create + set/get），无 Provider 嵌套地狱
- 两个库社区都很活跃，TypeScript 支持完善
- 组合灵活：TanStack Query 负责数据获取，Zustand 负责交互编排

### 负面
- 两个库，团队需要理解各自边界
- TanStack Query 的缓存失效策略需要约定规范

### 中性
- 建议约定：API 数据一律走 TanStack Query，UI 状态一律走 Zustand，不混用

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| Redux Toolkit + RTK Query | 模板代码多，管理控制台场景过重 |
| Jotai + TanStack Query | 原子化模型学习曲线高，细粒度更新在管理后台场景优势不大 |
| TanStack Query 单用 | 复杂 UI 状态（多步表单/向导）用 React 内置 useState 会很乱 |
| SWR + Zustand | SWR 功能不如 TanStack Query 全面（ mutations/乐观更新 弱） |
