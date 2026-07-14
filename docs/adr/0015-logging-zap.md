# ADR-0015: Go 日志库采用 zap

## 状态

Accepted

## 背景

Go 后端需要结构化日志库。需要支持：
- 结构化日志（JSON 格式）
- 日志级别（Debug/Info/Warn/Error）
- 高性能（部署状态轮询场景日志量大）
- 上下文传递（request ID、deployment ID 等）

## 决策

采用 **zap**（Uber）作为日志库。

## 后果

### 正面
- 性能极高（零分配），适合高吞吐日志场景
- 社区最大，生态丰富，生产级验证
- 与 slog 可桥接（zap 提供 slog.Handler 适配）
- 支持多种输出目标（JSON、Console）
- 丰富的字段类型（String/Int/Duration 等），类型安全

### 负面
- API 偏底层，ergonomics 一般
- 与标准库 slog 有风格差异

### 中性
- 推荐封装薄层 logger，注入 request ID 等上下文字段
- 后续如需迁移 slog，可通过 slog bridge 平滑过渡

## 备选方案

| 方案 | 拒绝理由 |
|------|---------|
| log/slog | 标准库方向，但功能基础，日志轮转等需自行封装 |
| zerolog | API 优雅但社区较小 |
