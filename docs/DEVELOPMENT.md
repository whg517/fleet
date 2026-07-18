# Fleet — 开发指南

| 字段 | 内容 |
|------|------|
| 文档版本 | v1.0 |
| 创建日期 | 2026-07-16 |
| 状态 | Active |
| 关联文档 | REQUIREMENTS.md, ARCHITECTURE.md, CONTRIBUTING.md, DESIGN-SPEC.md |

---

## 目录

- [1. 环境准备](#1-环境准备)
- [2. 项目结构](#2-项目结构)
- [3. 快速启动](#3-快速启动)
- [4. 开发工作流](#4-开发工作流)
- [5. 后端开发规范](#5-后端开发规范)
- [6. 前端开发规范](#6-前端开发规范)
- [7. 数据库与 Schema 管理](#7-数据库与-schema-管理)
- [8. 测试策略](#8-测试策略)
- [9. 配置管理](#9-配置管理)
- [10. API 设计规范](#10-api-设计规范)
- [11. 错误处理规范](#11-错误处理规范)
- [12. 日志规范](#12-日志规范)
- [13. 部署与发布](#13-部署与发布)
- [14. 故障排查](#14-故障排查)

---

## 1. 环境准备

### 1.1 必需工具

| 工具 | 最低版本 | 推荐版本 | 说明 |
|------|---------|---------|------|
| Go | 1.26+ | 1.26+ | 后端语言（go.mod 要求） |
| Node.js | 22 LTS+ | 24+ | 前端构建 |
| npm | 10+ | latest | 前端包管理（项目使用 npm） |
| Docker | 24+ | latest | 容器化构建 |
| Kubectl | 1.28+ | 1.30+ | K8s 集群操作 |
| Helm | 3.14+ | latest | K8s 包管理 |
| Git | 2.40+ | latest | 版本控制 |
| Ent CLI | latest | latest | ORM 代码生成 |
| Atlas | latest | latest | DB schema 迁移工具 |
| GitHub CLI (gh) | 2.40+ | latest | PR/Issue 管理 |

### 1.1.1 前端核心依赖

| 依赖 | 版本 | 说明 |
|------|------|------|
| Next.js | ^16.2 | React 框架（SPA 模式） |
| React | ^19.2 | UI 库 |
| TypeScript | 5.9 | 类型安全 |
| Tailwind CSS | ^4.3 | 原子化 CSS |
| HeroUI | ^2.8 | UI 组件库（基于 React Aria） |
| ESLint | ^10.7 | 代码检查 |
| golangci-lint | v2 | Go 代码检查 |

### 1.2 可选工具

| 工具 | 用途 |
|------|------|
| k9s | K8s 终端 UI |
| lazygit | Git 终端 UI |
| jq | JSON 处理 |
| grpcurl | gRPC 调试（如后续引入） |

### 1.3 本地依赖服务

开发环境需要以下服务运行：

| 服务 | 用途 | 本地启动方式 |
|------|------|-------------|
| PostgreSQL 16 | 主数据库 | `docker compose up postgres` |
| Redis 7 | 缓存/队列/分布式锁 | `docker compose up redis` |

> 生产环境的 Argo CD、Argo Workflows、Harbor 通过远程连接，本地不需要安装。

### 1.4 环境检查

```bash
# 验证工具链
go version          # >= 1.26
node --version      # >= 22
npm --version       # >= 10
docker --version    # >= 24
kubectl version --client
helm version
ent version
atlas version
gh --version
```

---

## 2. 项目结构

```
platform/
├── cmd/                          # Go 入口
│   └── server/
│       └── main.go               # 应用入口
│
├── internal/                     # Go 内部实现（不对外暴露）
│   ├── api/                      # HTTP/WebSocket 层
│   │   ├── router.go             # 路由注册
│   │   ├── handler/              # 请求处理器（薄层，只做参数解析和响应）
│   │   │   ├── service.go
│   │   │   ├── deployment.go
│   │   │   ├── cluster.go
│   │   │   └── ...
│   │   ├── middleware/           # 中间件
│   │   │   ├── auth.go           # OIDC 认证
│   │   │   ├── rbac.go           # Casbin 权限拦截
│   │   │   ├── audit.go          # 审计日志
│   │   │   ├── cors.go           # CORS
│   │   │   └── recover.go        # Panic 恢复
│   │   └── ws/                   # WebSocket 处理
│   │       └── hub.go            # 连接管理 + 频道订阅
│   │
│   ├── domain/                   # 业务逻辑层（核心）
│   │   ├── service/              # 服务目录管理
│   │   ├── environment/          # 环境管理
│   │   ├── deployment/           # 部署编排（最核心模块）
│   │   ├── cluster/              # 集群管理
│   │   ├── version/              # 版本管理
│   │   ├── config/               # 配置变更管理
│   │   ├── approval/             # 审批流程
│   │   ├── audit/                # 审计查询
│   │   ├── scaling/              # 扩缩容
│   │   ├── notification/         # 通知中心
│   │   ├── template/             # 模板管理
│   │   ├── auth/                 # 认证/会话管理
│   │   └── topology/             # 版本分布/拓扑
│   │
│   ├── infra/                    # 基础设施对接层
│   │   ├── argocd/               # Argo CD API Client
│   │   ├── argowf/               # Argo Workflows API Client
│   │   ├── harbor/               # Harbor API Client
│   │   ├── oci/                  # OCI 制品仓库操作
│   │   ├── ansible/              # Ansible Runner 封装
│   │   ├── prometheus/           # 监控查询
│   │   ├── kube/                 # K8s API 操作
│   │   ├── secrets/              # AES-256-GCM 加解密
│   │   ├── casbin/               # Casbin 权限引擎
│   │   └── notify/               # Webhook 通知发送
│   │
│   ├── store/                    # 持久层
│   │   ├── ent/                  # Ent 生成的代码（不手动编辑）
│   │   ├── postgres/             # PostgreSQL 仓储实现
│   │   ├── redis/                # Redis 仓储实现
│   │   └── repos/                # 仓储接口定义（domain 层依赖接口）
│   │
│   ├── config/                   # 配置加载（Viper）
│   │   └── config.go
│   │
│   └── pkg/                      # 通用工具（可被外部引用）
│       ├── errors/               # 错误类型定义
│       ├── logger/               # zap 封装
│       ├── crypto/               # 加密工具
│       └── pagination/           # 分页工具
│
├── ent/schema/                   # Ent Schema 定义（数据库模型）
│   ├── user.go
│   ├── service.go
│   ├── deployment.go
│   └── ...
│
├── web/                          # Next.js 前端
│   ├── src/
│   │   ├── app/                  # Next.js App Router（或 Pages Router）
│   │   ├── components/           # React 组件
│   │   ├── hooks/                # 自定义 Hooks
│   │   ├── stores/               # 状态管理（Zustand）
│   │   ├── services/             # API 调用封装
│   │   ├── types/                # TypeScript 类型
│   │   └── utils/                # 工具函数
│   │   ├── public/               # 静态资源
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── tailwind.config.ts
│   ├── next.config.js
│   └── package-lock.json
│
├── deploy/                       # 部署配置
│   ├── docker/                   # Dockerfile
│   │   ├── Dockerfile.server     # 后端镜像
│   │   └── Dockerfile.web        # 前端镜像
│   ├── helm/                     # 平台自身 Helm Chart
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   └── docker-compose.yaml       # 本地开发依赖
│
├── docs/                         # 项目文档
├── .github/                      # GitHub 配置
│   ├── ISSUE_TEMPLATE/
│   └── workflows/                # GitHub Actions CI
│
├── .gitignore
├── go.mod
├── go.sum
├── Makefile                      # 构建/测试/开发命令
└── AGENTS.md
```

### 分层原则

```
api (handler)  →  domain (业务逻辑)  →  store/repos (接口)
                                          ↓
                                       infra (外部系统对接)
                                       store/postgres (DB 实现)
                                       store/redis (缓存实现)
```

- **api/handler**：薄层，只做请求解析、参数校验、调用 domain、返回响应
- **domain**：业务核心，不依赖具体基础设施实现，通过接口交互
- **infra**：外部系统对接（Argo CD、Harbor、K8s 等）
- **store**：数据库访问层，实现 store/repos 定义的接口

---

## 3. 快速启动

### 3.1 克隆仓库

```bash
git clone git@github.com:whg517/fleet.git
cd fleet
```

### 3.2 启动本地依赖

```bash
# 启动 PostgreSQL + Redis + MinIO
docker compose -f deploy/docker-compose.yaml up -d

# 验证服务状态
docker compose -f deploy/docker-compose.yaml ps
```

### 3.3 后端启动

```bash
# 安装依赖
go mod download

# 生成 Ent 代码
go generate ./ent

# 执行数据库迁移
go run ./cmd/server migrate

# 启动后端服务（默认 :8080）
go run ./cmd/server serve

# 或使用 Makefile
make dev-server
```

### 3.4 前端启动

```bash
cd web
npm ci
npm run dev        # 默认 :3000
```

### 3.5 验证

```bash
# 后端健康检查
curl http://localhost:8080/healthz

# 前端页面
open http://localhost:3000
```

### 3.6 Makefile 速查

```bash
make help           # 查看所有可用命令
make dev-server     # 启动后端（热重载）
make dev-web        # 启动前端
make dev-all        # 同时启动前后端
make test           # 运行所有测试
make test-cover     # 测试覆盖率报告
make lint           # 代码检查
make build          # 构建生产二进制
make ent-gen        # Ent 代码生成
make db-migrate     # 数据库迁移
make docker-build   # 构建容器镜像
```

---

## 4. 开发工作流

### 4.1 Git Worktree 工作流

项目推荐使用 Git Worktree 进行并行开发，避免频繁切换分支：

```bash
# 创建 worktree（在项目根目录执行）
git worktree add .worktree/feat/42-argocd-app-management feat/42-argocd-app-management

# 进入 worktree 工作
cd .worktree/feat/42-argocd-app-management

# 完成后回到主工作区
cd -
git worktree remove .worktree/feat/42-argocd-app-management
```

### 4.2 完整开发流程

```
1. 认领 Issue / 创建 Issue
2. git fetch origin && git checkout main && git pull --ff-only origin main
3. git worktree add .worktree/<type>/<issue#>-<desc> -b <type>/<issue#>-<desc> origin/main
4. cd .worktree/<type>/<issue#>-<desc>
5. 开发 + 本地测试
6. → 提交前门禁检查（见 §4.3）
7. git add . && git commit -m "type(scope): subject"
8. git push -u origin <type>/<issue#>-<desc>
9. gh pr create
10. 等待 CI 通过
11. 人工 Code Review + 验收条件逐条核对
12. gh pr merge <pr#> --squash --delete-branch
13. 清理 worktree：git worktree remove .worktree/<type>/<issue#>-<desc>
```

详见 [CONTRIBUTING.md](./CONTRIBUTING.md)。

### 4.3 提交前门禁检查

**push 之前必须通过以下检查，确保代码质量在本地就有保障：**

| # | 检查项 | 命令 | 必须通过 |
|---|--------|------|---------|
| 1 | 后端 Lint | `make lint` | ✅ |
| 2 | 后端单元测试 | `make test` | ✅ |
| 3 | Ent 代码生成（改了 schema 时） | `make ent-gen` | ✅ |
| 4 | DB 迁移（改了 schema 时） | `make db-migrate` | ✅ |
| 5 | 前端构建（改了前端时） | `cd web && npm run build` | ✅ |
| 6 | 本地启动验证 | `make dev-server` | ✅ |

```bash
# 一键执行核心检查
make lint && make test

# 改了前端
cd web && npm run build && cd ..
```

> **规则**：门禁检查未全部通过，禁止 push。CI 会再跑一遍，但本地先过能节省往返时间。

### 4.4 分支命名

| 类型 | 格式 | 示例 |
|------|------|------|
| 功能 | `feat/<issue#>-<desc>` | `feat/42-argocd-app-management` |
| 修复 | `fix/<issue#>-<desc>` | `fix/58-oidc-token-refresh` |
| 文档 | `docs/<issue#>-<desc>` | `docs/61-api-spec` |
| 重构 | `refactor/<issue#>-<desc>` | `refactor/55-deployment-lock` |
| 测试 | `test/<issue#>-<desc>` | `test/72-deploy-e2e` |

> 禁用 `chore` 类型。

### 4.5 提交规范

```
type(scope): subject

# 示例
feat(deploy): 实现 Argo CD Application CRUD
fix(auth): 修复 OIDC token 刷新竞态条件
docs(api): 补充部署 API 接口文档
refactor(config): 重构配置变更锁逻辑
test(deploy): 添加部署回滚单元测试
```

| type | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `refactor` | 代码重构（不改变外部行为） |
| `test` | 测试相关 |

---

## 5. 后端开发规范

### 5.1 目录与分包

按 domain 模块组织，每个 domain 模块内部结构：

```
domain/deployment/
├── service.go          # 业务逻辑（Service struct + 方法）
├── dto.go              # 数据传输对象（请求/响应结构）
├── errors.go           # 业务错误定义
└── service_test.go     # 单元测试
```

### 5.2 依赖注入

使用接口 + 构造函数注入，不使用 wire/dig 等框架：

```go
// store/repos/deployment.go — 接口定义
package repos

type DeploymentRepository interface {
    Create(ctx context.Context, d *Deployment) (*Deployment, error)
    GetByID(ctx context.Context, id string) (*Deployment, error)
    ListByService(ctx context.Context, serviceID string, page, size int) ([]*Deployment, int, error)
    UpdateStatus(ctx context.Context, id string, status DeploymentStatus) error
}

// domain/deployment/service.go — 业务逻辑
package deployment

type Service struct {
    repo    repos.DeploymentRepository
    argocd  argocd.Client
    locker  LockManager
    logger  *zap.Logger
}

func NewService(repo repos.DeploymentRepository, argocd argocd.Client, locker LockManager, logger *zap.Logger) *Service {
    return &Service{repo: repo, argocd: argocd, locker: locker, logger: logger}
}
```

### 5.3 错误处理

使用 sentinel errors + `errors.Is` / `errors.As`，不用 `errors.New` 做业务错误：

```go
// domain/deployment/errors.go
package deployment

import "errors"

var (
    ErrNotFound          = errors.New("deployment not found")
    ErrInvalidStatus     = errors.New("invalid deployment status")
    ErrLockConflict      = errors.New("deployment lock conflict")
    ErrServiceFrozen     = errors.New("service is frozen")
    ErrInsufficientPerms = errors.New("insufficient permissions")
)

// handler 中转换为 HTTP 响应
func handleError(w http.ResponseWriter, err error) {
    switch {
    case errors.Is(err, deployment.ErrNotFound):
        writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
    case errors.Is(err, deployment.ErrLockConflict):
        writeError(w, http.StatusConflict, "LOCK_CONFLICT", err.Error())
    case errors.Is(err, deployment.ErrServiceFrozen):
        writeError(w, http.StatusForbidden, "SERVICE_FROZEN", err.Error())
    default:
        writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
    }
}
```

### 5.4 Context 使用

- 所有 service 方法第一个参数必须是 `context.Context`
- 使用 context 传递 trace ID（中间件注入），不传业务参数
- 设置合理的 timeout（HTTP handler 层默认 30s）

```go
func (s *Service) Deploy(ctx context.Context, req DeployRequest) (*DeployResult, error) {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    // ...
}
```

### 5.5 并发安全

- Service struct 设计为无状态，可安全并发调用
- 共享状态通过 Redis 分布式锁保护
- 避免在 struct 中存储可变状态

---

## 6. 前端开发规范

### 6.1 技术栈

| 技术 | 说明 |
|------|------|
| Next.js 14+ | React 框架（SPA 模式） |
| TypeScript | 类型安全 |
| HeroUI | UI 组件库（基于 React Aria + Tailwind CSS） |
| Tailwind CSS | 原子化 CSS |
| Zustand | 轻量状态管理 |
| TanStack Query | 服务端状态管理（数据获取/缓存） |
| WebSocket | 实时推送（部署状态/审批通知） |

### 6.2 目录结构

```
web/src/
├── app/                    # 页面路由
│   ├── (auth)/             # 认证相关页面
│   ├── services/           # 服务目录
│   ├── deployments/        # 部署中心
│   ├── approvals/          # 发布审批
│   ├── cluster/            # 集群运维
│   ├── audit/              # 审计分析
│   └── settings/           # 系统设置
├── components/
│   ├── ui/                 # 通用 UI 组件
│   ├── layout/             # 布局组件
│   ├── service/            # 服务相关组件
│   ├── deployment/         # 部署相关组件
│   └── ...
├── hooks/                  # 自定义 Hooks
│   ├── useDeployment.ts
│   ├── useWebSocket.ts
│   └── ...
├── stores/                 # Zustand stores
│   ├── auth.ts             # 认证状态
│   └── notification.ts     # 通知状态
├── services/               # API 调用
│   ├── api.ts              # Axios/Fetch 实例
│   ├── deployment.ts       # 部署相关 API
│   └── ...
├── types/                  # TypeScript 类型定义
└── utils/                  # 工具函数
```

### 6.3 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 组件文件 | PascalCase | `DeploymentCard.tsx` |
| 普通文件 | kebab-case | `api-client.ts` |
| Hooks | camelCase + use 前缀 | `useDeployment.ts` |
| 类型/接口 | PascalCase | `DeploymentStatus` |
| 常量 | UPPER_SNAKE | `API_BASE_URL` |
| CSS 类 | Tailwind 优先 | — |

### 6.4 API 调用

统一通过 `services/` 层封装，组件不直接调用 `fetch`/`axios`：

```typescript
// services/deployment.ts
import { api } from './api'

export const deploymentApi = {
  list: (params: DeploymentListParams) =>
    api.get<DeploymentListResponse>('/deployments', { params }),
  get: (id: string) =>
    api.get<Deployment>(`/deployments/${id}`),
  create: (req: CreateDeploymentRequest) =>
    api.post<Deployment>('/deployments', req),
  rollback: (id: string, targetVersion?: string) =>
    api.post(`/deployments/${id}/rollback`, { targetVersion }),
}
```

### 6.5 WebSocket 连接

```typescript
// hooks/useWebSocket.ts
export function useWebSocket(channel: string) {
  const [connected, setConnected] = useState(false)
  
  useEffect(() => {
    const ws = new WebSocket(`${WS_BASE}/${channel}`)
    ws.onopen = () => setConnected(true)
    ws.onclose = () => {
      setConnected(false)
      // 自动重连（指数退避）
    }
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      // 根据 msg.type 分发
    }
    return () => ws.close()
  }, [channel])
  
  return { connected }
}
```

---

## 7. 数据库与 Schema 管理

### 7.1 Ent Schema 定义

数据库模型通过 Ent Schema 定义，位于 `ent/schema/`：

```go
// ent/schema/service.go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/edge"
    "entgo.io/ent/schema/index"
)

type Service struct {
    ent.Schema
}

func (Service) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().Immutable(),
        field.String("name").NotEmpty(),
        field.Text("description").Optional(),
        field.String("owner_team").Optional(),
        field.Enum("deploy_type").Values("k8s", "vm"),
        field.Enum("status").Values("active", "frozen", "offline"),
        // 外键通过 edge 定义
    }
}

func (Service) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("versions", ServiceVersion.Type),
        edge.To("deployments", Deployment.Type),
        edge.From("cluster", Cluster.Type).Ref("services").Unique(),
    }
}

func (Service) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("name"),
        index.Fields("owner_team"),
        index.Fields("status"),
    }
}
```

### 7.2 代码生成

```bash
# 修改 schema 后生成代码
make ent-gen     # 即 go generate ./ent

# 生成后检查变更
git diff ent/
```

### 7.3 数据库迁移

使用 Atlas（Ent 内置集成）管理 schema 变更：

```bash
# 生成迁移文件
make db-migrate-dry    # 预览变更

# 执行迁移
make db-migrate        # 即 atlas migrate apply

# 开发环境重置
make db-reset          # 清空 + 重新迁移（仅开发环境！）
```

迁移文件位于 `ent/migrate/migrations/`，提交到 Git。

### 7.4 规则

- Schema 变更必须通过 Ent Schema → 代码生成 → Atlas 迁移，**禁止手写 SQL DDL**
- 迁移文件一旦提交不可修改，新变更追加新迁移文件
- 破坏性变更（删列/删表）需在 PR 中说明影响和兼容方案
- 测试库可随时 reset，开发库变更前先备份

---

## 8. 测试策略

### 8.1 测试分层

| 层级 | 范围 | 工具 | 目标覆盖率 | 运行命令 |
|------|------|------|-----------|---------|
| 单元测试 | domain/ 层业务逻辑 | Go testing + testify | > 80% | `make test-unit` |
| 集成测试 | infra/ 层外部系统对接 | Go testing + testcontainers | 关键路径覆盖 | `make test-integration` |
| API 测试 | handler 层 HTTP 请求 | Go testing + httptest | 核心接口覆盖 | `make test-api` |
| E2E 测试 | 全链路 | Playwright | 核心流程覆盖 | `make test-e2e` |
| 前端测试 | 组件 + Hooks | Vitest + Testing Library | 关键组件覆盖 | `npm test` |

### 8.2 单元测试规范

```go
// domain/deployment/service_test.go
package deployment

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// 使用 mock 对象，不依赖真实数据库
func TestDeploy_Success(t *testing.T) {
    // Arrange
    mockRepo := new(MockDeploymentRepo)
    mockArgocd := new(MockArgocdClient)
    mockLocker := new(MockLockManager)
    
    svc := NewService(mockRepo, mockArgocd, mockLocker, testLogger())
    
    mockLocker.On("Acquire", mock.Anything, "deploy:lock:svc-1:env-1").
        Return(nil)
    mockArgocd.On("Sync", mock.Anything, "app-svc-1-dev").
        Return(nil)
    
    // Act
    result, err := svc.Deploy(context.Background(), DeployRequest{
        ServiceID: "svc-1",
        EnvID:     "env-1",
        VersionID: "ver-1",
    })
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, StatusDeploying, result.Status)
    mockLocker.AssertExpectations(t)
    mockArgocd.AssertExpectations(t)
}
```

### 8.3 测试命名

```go
// 好的命名
func TestDeploy_LockConflict_ReturnsError(t *testing.T)
func TestDeploy_ServiceFrozen_ReturnsError(t *testing.T)
func TestDeploy_PreProd_GoesThroughApproval(t *testing.T)

// 避免的命名
func TestDeploy1(t *testing.T)
func TestDeployCase(t *testing.T)
```

### 8.4 CI 中的测试

```bash
# CI 自动执行（.github/workflows/ci.yml）
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # 检查覆盖率阈值
```

---

## 9. 配置管理

### 9.1 配置层级

```
默认配置 (config.default.yaml)
  ↓ 覆盖
环境变量 (FLEET_DB_HOST, FLEET_REDIS_URL, ...)
  ↓ 覆盖
本地配置文件 (config.local.yaml，gitignore)
```

### 9.2 配置结构

```go
// internal/config/config.go
type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    DB       DBConfig       `mapstructure:"db"`
    Redis    RedisConfig    `mapstructure:"redis"`
    OIDC     OIDCConfig     `mapstructure:"oidc"`
    ArgoCD   ArgoCDConfig   `mapstructure:"argocd"`
    ArgoWF   ArgoWFConfig   `mapstructure:"argowf"`
    Harbor   HarborConfig   `mapstructure:"harbor"`
    Kube     KubeConfig     `mapstructure:"kube"`
    Secrets  SecretsConfig  `mapstructure:"secrets"`
    Log      LogConfig      `mapstructure:"log"`
}

type ServerConfig struct {
    Port            int           `mapstructure:"port" default:"8080"`
    ReadTimeout     time.Duration `mapstructure:"read_timeout" default:"10s"`
    WriteTimeout    time.Duration `mapstructure:"write_timeout" default:"30s"`
    ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" default:"15s"`
}

type DBConfig struct {
    Host         string `mapstructure:"host"`
    Port         int    `mapstructure:"port" default:"5432"`
    Database     string `mapstructure:"database" default:"fleet"`
    Username     string `mapstructure:"username"`
    Password     string `mapstructure:"password"`
    MaxOpenConns int    `mapstructure:"max_open_conns" default:"25"`
    MaxIdleConns int    `mapstructure:"max_idle_conns" default:"5"`
}

type SecretsConfig struct {
    // AES-256-GCM 加密密钥（通过 K8s Secret 注入或环境变量）
    EncryptionKey string `mapstructure:"encryption_key"`
}
```

### 9.3 环境变量

所有配置可通过环境变量覆盖，前缀 `FLEET_`：

```bash
FLEET_SERVER_PORT=9090
FLEET_DB_HOST=localhost
FLEET_DB_PORT=5432
FLEET_DB_DATABASE=fleet
FLEET_DB_USERNAME=fleet
FLEET_DB_PASSWORD=secret
FLEET_REDIS_URL=redis://localhost:6379
FLEET_OIDC_ISSUER=https://sso.company.com
FLEET_OIDC_CLIENT_ID=fleet
FLEET_OIDC_CLIENT_SECRET=xxx
FLEET_ARGOCD_SERVER=https://argocd.internal:443
FLEET_ARGOCD_TOKEN=xxx
FLEET_SECRETS_ENCRYPTION_KEY=base64-encoded-key
```

### 9.4 敏感信息

- **禁止**将密钥/密码/Token 提交到 Git
- 使用 `config.local.yaml`（已 gitignore）或环境变量
- 生产环境通过 K8s Secret 注入
- `.gitignore` 中已排除：`config.local.yaml`、`*.env`、`.env*`

---

## 10. API 设计规范

### 10.1 RESTful 约定

- URL 使用复数名词：`/api/v1/services`、`/api/v1/deployments`
- 资源层级用嵌套：`/api/v1/services/:id/versions`
- 标准 HTTP 方法：GET（列表/详情）、POST（创建）、PUT（全量更新）、PATCH（部分更新）、DELETE（删除）
- HTTP 状态码：200（成功）、201（创建）、204（无内容）、400（请求错误）、401（未认证）、403（无权限）、404（不存在）、409（冲突）、422（校验失败）、500（服务器错误）

### 10.2 统一响应格式

**成功（单资源）：**

```json
{
  "data": {
    "id": "svc-001",
    "name": "user-service",
    "status": "active"
  }
}
```

**成功（列表 + 分页）：**

```json
{
  "data": [
    { "id": "svc-001", "name": "user-service" }
  ],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total": 100
  }
}
```

**错误：**

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "service_name is required",
    "details": [
      { "field": "name", "issue": "required" }
    ]
  }
}
```

### 10.3 错误码定义

| Code | HTTP Status | 说明 |
|------|-------------|------|
| `VALIDATION_ERROR` | 400 | 请求参数校验失败 |
| `UNAUTHORIZED` | 401 | 未登录或 Token 过期 |
| `FORBIDDEN` | 403 | 无操作权限 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `CONFLICT` | 409 | 资源冲突（如部署锁） |
| `LOCK_CONFLICT` | 409 | 部署锁冲突 |
| `SERVICE_FROZEN` | 403 | 服务已冻结 |
| `INTERNAL` | 500 | 服务器内部错误 |

### 10.4 API 版本化

- URL 路径版本：`/api/v1/...`
- 不兼容变更递增主版本号
- 兼容变更在同一版本内追加

---

## 11. 错误处理规范

### 11.1 后端错误处理链

```
Handler 层
  ↓ 参数校验（400 VALIDATION_ERROR）
Domain 层
  ↓ 业务规则校验（返回 sentinel error）
Infra 层
  ↓ 外部系统调用失败（包装为内部错误）
中间件
  ↓ 统一错误响应格式
```

### 11.2 日志与错误

```go
// Domain 层：记录业务异常
if err != nil {
    s.logger.Warn("deployment lock conflict",
        zap.String("service_id", req.ServiceID),
        zap.String("env_id", req.EnvID),
        zap.Error(err),
    )
    return ErrLockConflict
}

// Handler 层：记录未预期错误
if err != nil {
    if !errors.Is(err, domain.ErrNotFound) {
        h.logger.Error("unexpected error",
            zap.String("trace_id", traceID),
            zap.Error(err),
        )
    }
    handleError(w, r, err)
}
```

### 11.3 面向用户的错误信息

错误信息遵循 DESIGN-SPEC.md 第五部分的三段式规范：
- **发生了什么**：一句话说明
- **为什么**：1-3 个可能原因
- **下一步怎么做**：可操作的步骤

后端返回 code + message，前端负责翻译为用户友好的三段式提示。

---

## 12. 日志规范

### 12.1 日志库

使用 zap（结构化日志），通过 `internal/pkg/logger` 统一封装。

### 12.2 日志级别

| 级别 | 使用场景 |
|------|---------|
| `Error` | 影响功能的错误，需要人工介入 |
| `Warn` | 可预期的异常（如部署锁冲突、重试） |
| `Info` | 关键业务事件（部署开始/完成、审批通过） |
| `Debug` | 开发调试信息（生产环境关闭） |

### 12.3 结构化字段约定

```go
// 标准字段
s.logger.Info("deployment started",
    zap.String("trace_id", traceID),       // 链路追踪 ID
    zap.String("deployment_id", dep.ID),
    zap.String("service_id", dep.ServiceID),
    zap.String("environment", env.Name),
    zap.String("version", ver.Version),
    zap.String("initiated_by", user.Name),
)
```

### 12.4 敏感信息脱敏

日志中禁止出现：
- 密码、Token、API Key
- kubeconfig 原文
- 加密密钥
- Webhook Secret

使用 `zap.String("password", "***")` 或自定义 `RedactedString` 类型。

---

## 13. 部署与发布

### 13.1 CI/CD 流水线

```
Push / PR
  → GitHub Actions Trigger
  → Lint (golangci-lint + eslint)
  → Test (go test + npm test)
  → Build (go build + next build)
  → Docker Build & Push (Harbor)
  → (main 分支) Helm Deploy to Dev
```

### 13.2 多环境部署

| 环境 | 触发方式 | Argo CD Sync |
|------|---------|-------------|
| dev | push to main 自动部署 | auto-sync |
| test | 手动触发 | auto-sync |
| pre | 手动触发 + 审批 | manual sync |
| prod | 手动触发 + 审批 | manual sync |

### 13.3 平台自身部署

平台自身通过 Helm Chart 部署到管理命名空间：

```bash
# 构建镜像
make docker-build

# 部署到开发环境
helm upgrade fleet deploy/helm/fleet \
  -n fleet-system \
  -f deploy/helm/fleet/values-dev.yaml
```

### 13.4 版本管理

- 版本号遵循 SemVer：`v<major>.<minor>.<patch>`
- Git Tag 标记发布版本
- Changelog 记录在每个 Release 的 GitHub Release Notes 中

---

## 14. 故障排查

### 14.1 本地开发常见问题

**数据库连接失败：**
```bash
# 检查 PostgreSQL 是否运行
docker compose -f deploy/docker-compose.yaml ps postgres
# 检查端口
lsof -i :5432
# 检查配置
cat config.local.yaml | grep db
```

**Ent 代码生成失败：**
```bash
# 确认 Ent CLI 版本
ent version
# 重新生成
go clean -cache
go generate ./ent
```

**前端依赖冲突：**
```bash
cd web
rm -rf node_modules package-lock.json
npm ci
```

**端口冲突：**
```bash
# 后端默认 8080，前端默认 3000
# 修改后端端口
FLEET_SERVER_PORT=9090 make dev-server
# 修改前端端口
cd web && PORT=3001 npm run dev
```

### 14.2 调试技巧

**后端调试：**
```bash
# 使用 delve 远程调试
dlv debug ./cmd/server --headless --listen=:2345 --api-version=2
# VS Code 连接 delve
# 配置 launch.json: connect mode, port 2345
```

**数据库调试：**
```bash
# 连接开发数据库
docker compose -f deploy/docker-compose.yaml exec postgres psql -U fleet -d fleet

# 查看最近迁移
SELECT * FROM atlas_schema_revisions ORDER BY RAND() DESC LIMIT 5;
```

**API 调试：**
```bash
# 获取 token
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/dev-token | jq -r .data.token)

# 调用 API
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/services
```

### 14.3 获取帮助

- 查看架构文档：[ARCHITECTURE.md](./ARCHITECTURE.md)
- 查看需求文档：[REQUIREMENTS.md](./REQUIREMENTS.md)
- 查看设计规范：[DESIGN-SPEC.md](./DESIGN-SPEC.md)
- 查看 ADR：[adr/](./adr/)
- 查看 PR 流程：[CONTRIBUTING.md](./CONTRIBUTING.md)
- 创建 Issue：使用 `.github/ISSUE_TEMPLATE/` 模板

---

## 附录：IDE 配置推荐

### VS Code

推荐扩展：
- Go (golang.go)
- ESLint (dbaeumer.vscode-eslint)
- Tailwind CSS IntelliSense (bradlc.vscode-tailwindcss)
- PostgreSQL (ckolkman.vscode-postgres)
- Protobuf (for gRPC debugging, if applicable)

### GoLand / IntelliJ

- 启用 Go module 集成
- 配置 Ent 代码生成 File Watcher
- Database 插件连接 PostgreSQL

### settings.json 示例

```json
{
  "go.formatTool": "goimports",
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "workspace",
  "[go]": {
    "editor.formatOnSave": true
  },
  "[typescript]": {
    "editor.formatOnSave": true,
    "editor.defaultFormatter": "esbenp.prettier-vscode"
  }
}
```
