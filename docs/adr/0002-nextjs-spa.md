# ADR-0002: 前端采用 Next.js SPA + HeroUI

## 状态

Accepted

## 背景

需要面向运维、开发、架构师三类的管理控制台。要求：
- 丰富的交互组件（表格、表单、拓扑图、实时状态）
- 团队熟悉的技术栈
- 长期可维护

## 决策

使用 Next.js + React + TypeScript，SPA 模式。
UI 组件库采用 **HeroUI**（基于 React Aria + Tailwind CSS）。

## 后果

### 正面
- 团队选型，已有经验
- HeroUI 基于 React Aria，无障碍支持完善
- HeroUI 基于 Tailwind CSS，与项目样式体系一致
- TypeScript 类型安全
- SSR 能力可后续按需启用

### 负面
- Next.js 对于纯 SPA 场景略重
- Bundle 体积需要优化
- HeroUI 社区规模不如 Ant Design，企业级场景需验证

### 中性
- 需要和 Go 后端约定 API 契约

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| Vue + Ant Design Vue | 团队选型偏好 React |
| 纯 React (CRA/Vite) | Next.js 提供更好的项目结构和路由 |
| Ant Design (React) | 团队偏好 HeroUI 的 Tailwind CSS 方案，避免 CSS-in-JS 运行时开销 |
| Shadcn/ui | 非完整组件库，需手动组装，维护成本高 |
