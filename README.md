# Fleet

> 企业级 DevOps 部署平台 — 统一管理 Kubernetes 与物理节点的服务部署、配置和运维。

## 技术栈

| 层 | 技术 |
|---|---|
| 前端 | Next.js SPA (React + TypeScript + HeroUI) |
| 后端 | Go + Echo (单体) |
| ORM | Ent |
| 数据库 | PostgreSQL 16 |
| 缓存/队列 | Redis 7 |
| 部署引擎 | Argo CD (K8s) + Ansible (物理节点) |
| 构建引擎 | Argo Workflows |
| 认证 | OIDC + Casbin RBAC |

## 快速启动

### 前置条件

- Go 1.22+
- Node.js 24+
- Docker & Docker Compose
- PostgreSQL 16 (或使用 docker-compose)
- Redis 7 (或使用 docker-compose)

### 1. 启动依赖服务

```bash
docker compose up -d
```

### 2. 启动后端

```bash
go mod download
go run ./cmd/server
```

### 3. 启动前端

```bash
cd web/
npm ci
npm run dev
```

后端默认监听 `:8080`，前端开发服务器监听 `:3000`。

## 项目结构

```
.
├── cmd/
│   └── server/              # Go 后端入口
├── internal/                # 后端业务逻辑
│   ├── api/                 # HTTP handler + middleware
│   ├── domain/              # 业务领域层
│   └── infra/               # 基础设施层
├── web/                     # Next.js 前端
├── deploy/
│   └── charts/
│       └── fleet/           # Helm Chart
├── docs/                    # 项目文档
│   ├── ARCHITECTURE.md      # 架构设计
│   ├── REQUIREMENTS.md      # 需求清单
│   ├── DESIGN-SPEC.md       # 交互规范
│   ├── CONTRIBUTING.md      # PR 流程
│   └── adr/                 # 架构决策记录
├── scripts/                 # 脚本
├── Dockerfile               # 多阶段构建
├── docker-compose.yml       # 本地开发依赖
└── .github/workflows/       # CI 配置
```

## 开发指南

- 📋 [需求清单](docs/REQUIREMENTS.md)
- 🏗️ [架构设计](docs/ARCHITECTURE.md)
- 🎨 [交互规范](docs/DESIGN-SPEC.md)
- 🤝 [贡献指南](docs/CONTRIBUTING.md)
- 📐 [架构决策记录](docs/adr/)

## 容器化

```bash
# 构建镜像
docker build -t fleet:latest .

# 运行
docker run -p 8080:8080 fleet:latest
```

## Helm 部署

```bash
# 渲染模板
helm template fleet deploy/charts/fleet/

# 部署到集群
helm install fleet deploy/charts/fleet/ \
  --set config.database.host=<pg-host> \
  --set secrets.databasePassword=<password>
```

## CI

GitHub Actions 流水线包含 lint、test、frontend build、docker build 四个 Job，详见 [ci.yml](.github/workflows/ci.yml)。

## License

MIT
