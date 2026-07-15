# Fleet — Argo CD 400 Application 性能压测方案

| 字段 | 内容 |
|------|------|
| 文档版本 | v1.0 |
| 创建日期 | 2026-07-11 |
| 关联里程碑 | M2 — 核心部署链路 |
| 状态 | Draft |

---

## 0. 背景与约束

- **规模**：100+ 微服务 × 4 环境（dev / test / pre / prod）= **400+ Argo CD Application**
- **集群拓扑**：单 K8s 集群，namespace 隔离多环境，Argo CD 部署在管理命名空间
- **平台交互模式**：平台通过 Argo CD API + informer/watch 模式管理 Application CRD，非直接轮询
- **风险来源**：架构文档 §12 明确列出「Argo CD 管理 400+ Application 性能」为中概率/中影响风险

本方案在 M2 阶段执行，验证 Argo CD 在目标规模下能否满足平台性能要求，为 M6 稳定化提供基线数据。

---

## 1. 压测目标

### 1.1 核心验证指标与达标标准

| # | 指标 | 达标线 | 告警线 | 说明 |
|---|------|--------|--------|------|
| G1 | Argo CD API 列表查询延迟 (400 App) | p95 < 1s | p95 < 3s | `argocd app list` 全量查询 |
| G2 | 单个 Application sync 触发延迟 | < 2s | < 5s | 从 API 调用到 sync 开始执行 |
| G3 | 单个 Application sync → Healthy 延迟 | < 90s | < 180s | 简单 Helm Chart（Deployment + Service） |
| G4 | 并发 sync 吞吐 (20 并发) | 全部 < 5min 完成 | 全部 < 10min | 20 个 App 同时触发 sync |
| G5 | 并发 sync 吞吐 (50 并发) | 全部 < 8min 完成 | 全部 < 15min | 极端场景压力测试 |
| G6 | Application 状态变更 watch 延迟 | < 5s | < 15s | 从 K8s 资源变更到 Argo CD 反映状态变化 |
| G7 | 平台 API 触发 → sync 完成端到端延迟 | < 120s | < 300s | dev 环境自动 sync 场景 |
| G8 | Argo CD Server 内存使用 (400 App 稳态) | < 1Gi | < 2Gi | 稳态运行 1h 后 |
| G9 | Argo CD Application Controller 内存 | < 2Gi | < 4Gi | 稳态运行 1h 后 |
| G10 | Argo CD Server CPU 使用率 (400 App) | < 500m | < 1000m | 稳态平均 |
| G11 | Argo CD Application Controller CPU | < 1000m | < 2000m | 稳态平均 |
| G12 | Argo CD Redis 内存 | < 512Mi | < 1Gi | 缓存 400 App 状态 |

### 1.2 平台层间接指标

| # | 指标 | 达标线 | 说明 |
|---|------|--------|------|
| P1 | 平台 API `/services/:id/health` 延迟 | p95 < 200ms | 平台查询 Argo CD 状态（含 informer 缓存命中场景） |
| P2 | 平台 informer 全量 resync 耗时 | < 30s | 400 App CRD 全量同步 |
| P3 | 平台 WebSocket 状态推送延迟 | < 3s | 从 Argo CD 状态变更到平台 WebSocket 推送 |

---

## 2. 测试环境准备

### 2.1 Argo CD 版本与配置

**版本**：Argo CD v2.12.x（M2 开发当前稳定版，执行时取最新 patch）

**部署拓扑**：

```yaml
# argocd-server
replicas: 2
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi

# argocd-application-controller
replicas: 1  # 先验证单 controller 能力，不达标再扩容
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 4Gi

# argocd-repo-server
replicas: 2
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi

# argocd-redis
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

**关键参数（初始值，压测中调优）**：

```yaml
# argocd-cmd-params-cm
server.repo.server.timeout.seconds: "60"
controller.status.processors: "20"        # 默认 20
controller.operation.processors: "10"     # 默认 10
controller.app.sync: "5s"                 # sync 状态检查间隔
controller.app.resync: "180s"             # 全量 resync 间隔（压测时缩短以观察行为）
repo.server.git.request.timeout.seconds: "30"
```

**Argo CD ApplicationController CLI 参数**：

```yaml
# 控制并发处理 Application 的 goroutine 数
# 默认通常够用，压测时观察队列深度决定是否调整
commandArgs:
  - --status-processors
  - "20"
  - --operation-processors
  - "10"
```

### 2.2 批量创建 400+ 测试 Application

#### 2.2.1 测试 Helm Chart

准备一个极简 Helm Chart `bench-app`，模拟真实微服务的最小形态：

```
bench-app/
├── Chart.yaml          # version: 0.1.0
├── values.yaml
└── templates/
    ├── deployment.yaml  # 1 replica, nginx:alpine, readinessProbe
    ├── service.yaml     # ClusterIP
    └── configmap.yaml   # 注入 App 编号（用于区分）
```

**Chart 设计原则**：
- 资源轻量：1 replica，requests 10m CPU / 16Mi memory
- 含 readinessProbe：确保 Argo CD 健康检查路径有效
- 无外部依赖：不依赖数据库/中间件，确保 sync 延迟只反映 Argo CD 自身性能
- 参数化：`.Values.appId` 注入编号，`.Values.env` 注入环境标识

#### 2.2.2 OCI 制品准备

将 `bench-app` Helm Chart 打包为 OCI 制品，推送到 Harbor：

```bash
# 打包 Chart
helm package bench-app --version 1.0.0

# 推送到 Harbor
helm registry login harbor.example.com -u $HARBOR_USER -p $HARBOR_PASS
helm push bench-app-1.0.0.tgz oci://harbor.example.com/charts
```

制品地址：`oci://harbor.example.com/charts/bench-app:1.0.0`

Application YAML 通过 `argocd-apps/` 目录批量管理，但 Chart 源不再依赖 Git 仓库目录结构：

```
argocd-apps/
├── dev/
│   ├── bench-001.yaml
│   ├── bench-002.yaml
│   └── ...               # 100 个
├── test/
│   └── ...               # 100 个
├── pre/
│   └── ...               # 100 个
└── prod/
    └── ...               # 100 个
```

#### 2.2.3 批量生成脚本

```bash
#!/usr/bin/env bash
# generate-apps.sh — 批量生成 400 个 Argo CD Application YAML
set -euo pipefail

OCI_URL="oci://harbor.example.com/charts/bench-app"
CHART_VERSION="1.0.0"
NAMESPACES=("dev" "test" "pre" "prod")
APPS_PER_ENV=100

generate_app() {
  local env="$1"
  local idx="$2"
  local app_name="bench-${env}-$(printf '%03d' "$idx")"
  local namespace="bench-${env}"

  cat <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ${app_name}
  namespace: argocd-system
  labels:
    app.kubernetes.io/managed-by: platform
    platform.dev/benchmark: "true"
    platform.dev/env: "${env}"
spec:
  project: default
  source:
    repoURL: ${OCI_URL}
    targetRevision: ${CHART_VERSION}
    chart: bench-app
    helm:
      values: |
        appId: "${app_name}"
        env: "${env}"
  destination:
    server: https://kubernetes.default.svc
    namespace: ${namespace}
  syncPolicy:
    automated:
      prune: true
      selfHeal: false         # 压测时手动触发 sync，dev/test 场景再开启
    syncOptions:
      - CreateNamespace=true
EOF
}

# 生成所有 Application YAML
for env in "${NAMESPACES[@]}"; do
  dir="argocd-apps/${env}"
  mkdir -p "$dir"
  for i in $(seq 1 "$APPS_PER_ENV"); do
    generate_app "$env" "$i" > "${dir}/bench-$(printf '%03d' "$i").yaml"
  done
done

echo "Generated $(( ${#NAMESPACES[@]} * APPS_PER_ENV )) Application manifests"
```

#### 2.2.4 批量部署脚本

```bash
#!/usr/bin/env bash
# deploy-apps.sh — 通过 kubectl apply 批量创建 Application
set -euo pipefail

APPS_DIR="argocd-apps"

# 分批 apply，每批 50 个，间隔 2s，避免瞬时压力
find "$APPS_DIR" -name '*.yaml' | sort | awk 'NR%50==1{print "";}{printf "%s ", $0}' | \
while IFS= read -r batch; do
  for f in $batch; do
    kubectl apply -f "$f" 2>/dev/null && echo "applied: $f" || echo "FAILED: $f"
  done
  echo "--- batch done, sleeping 2s ---"
  sleep 2
done

echo "All applications deployed. Use 'argocd app list' to verify."
```

#### 2.2.5 批量 sync 触发脚本

```bash
#!/usr/bin/env bash
# trigger-sync.sh — 并发触发指定数量的 Application sync
# Usage: ./trigger-sync.sh <concurrency> <total_apps>
set -euo pipefail

CONCURRENCY="${1:-10}"
TOTAL="${2:-20}"
ENV="${3:-dev}"

# 获取目标 App 列表
APPS=$(argocd app list -l platform.dev/env="${ENV}" -l platform.dev/benchmark=true \
       -o name | head -n "$TOTAL")

echo "Triggering sync for ${TOTAL} apps with concurrency ${CONCURRENCY}..."

# 使用 xargs 控制并发度
echo "$APPS" | xargs -P "$CONCURRENCY" -I {} argocd app sync {} --timeout 300

echo "Sync trigger completed."
```

### 2.3 压测工具

| 工具 | 用途 |
|------|------|
| `argocd` CLI | 批量触发 sync、查询 App 状态 |
| `kubectl` | 创建 Application CRD、查询 K8s 资源状态 |
| `hey` / `vegeta` | Argo CD API Server HTTP 压测（列表查询等） |
| `k6` | 复杂 API 调用编排（可选，如需更复杂场景） |
| Prometheus + Grafana | 指标采集与可视化 |
| `jq` + shell | 时间戳采集与延迟计算 |

### 2.4 环境准备清单

- [ ] 独立测试 K8s 集群（或隔离 namespace），不影响业务环境
- [ ] Argo CD 安装并配置上述资源/参数
- [ ] Harbor OCI 制品仓库就绪，bench-app Chart 已推送
- [ ] Prometheus scrape Argo CD metrics 已配置
- [ ] Grafana 导入 Argo CD Dashboard（官方 ID: 14592 / 15760）
- [ ] 压测脚本部署到执行节点，argocd CLI 已登录
- [ ] 基线快照：部署前采集 Argo CD 空 idle 资源使用

---

## 3. 测试场景

### 3.0 通用约定

- 每个场景执行 **3 轮**，取中位数
- 每轮之间间隔 60s 冷却，确保 Argo CD 回到稳态
- 记录环境信息（节点数、K8s 版本、Argo CD 版本、网络延迟）
- 所有时间戳使用 `date -u +%s%3N`（毫秒级 UTC）

---

### 场景 1：400 Application 列表查询性能

**目的**：验证平台在 400+ Application 规模下，通过 Argo CD API 查询列表的响应速度。平台 informer/watch 缓存未命中时需回退到 API 查询。

**步骤**：

```bash
# 1a. 全量列表查询（CLI）
time argocd app list -o json | jq '. | length'

# 1b. 全量列表查询（API 直调，用 vegeta 压测）
# 测试不同并发下的查询性能
echo 'GET http://argocd-server:2746/api/v1/applications' | \
  vegeta attack -duration=60s -rate=10 | tee results/list-r10.bin | vegeta report
echo 'GET http://argocd-server:2746/api/v1/applications' | \
  vegeta attack -duration=60s -rate=50 | tee results/list-r50.bin | vegeta report

# 1c. 带过滤条件的查询（模拟平台按环境过滤）
time argocd app list -l platform.dev/env=dev -o json | jq '. | length'

# 1d. 单个 Application 详情查询
time argocd app get bench-dev-001 -o json > /dev/null
```

**采集指标**：

| 指标 | 采集方式 |
|------|----------|
| API 响应延迟 (p50/p95/p99) | vegeta report |
| argocd-server CPU/内存 | Prometheus |
| argocd-server GRPC 请求延迟 | `grpc_server_handling_seconds_bucket` |

**达标判定**：全量列表查询 p95 < 1s（G1）；50 QPS 下无 5xx 错误。

---

### 场景 2：并发触发 10/20/50 个 sync

**目的**：验证 Argo CD 在多服务同时部署时的 sync 队列处理能力。模拟平台多用户同时触发部署或批量发布场景。

**步骤**：

```bash
# 2a. 10 并发 sync
./trigger-sync.sh 10 10 dev
# 记录: 第一个 sync 开始时间 → 最后一个 sync 完成时间

# 2b. 20 并发 sync（核心达标线 G4）
./trigger-sync.sh 20 20 dev

# 2c. 50 并发 sync（压力测试 G5）
./trigger-sync.sh 50 50 dev
```

**采集指标**：

| 指标 | 采集方式 |
|------|----------|
| sync 操作排队等待时间 | `argocd_app_sync_status` 时间戳差值 |
| 全部 sync 完成总耗时 | 脚本记录首尾时间戳 |
| sync 成功率 | `argocd_app_sync_total` / 触发总数 |
| operation processor 队列深度 | Argo CD metrics（如暴露） |
| application-controller CPU 峰值 | Prometheus |
| argocd-repo-server 并发数和延迟 | `argocd_git_request_total` / `repo_server_request_total` |

**达标判定**：
- 10 并发：全部 < 3min 完成
- 20 并发：全部 < 5min 完成（G4 达标线）
- 50 并发：全部 < 8min 完成（G5 达标线）
- 无 sync 失败（非应用本身的错误）

---

### 场景 3：单个 Application 的 sync → Healthy 延迟

**目的**：测量单个应用从触发 sync 到状态变为 Healthy 的完整延迟，建立基线。

**步骤**：

```bash
# 对 10 个不同 App 逐一测试，取中位数
for i in $(seq 1 10); do
  APP="bench-dev-$(printf '%03d' "$i")"

  # 记录触发时间
  START_MS=$(date -u +%s%3N)
  argocd app sync "$APP" --timeout 180 >/dev/null 2>&1

  # 轮询直到 Healthy
  while true; do
    STATUS=$(argocd app get "$APP" -o json | jq -r '.status.health.status')
    if [ "$STATUS" = "Healthy" ]; then
      END_MS=$(date -u +%s%3N)
      echo "App $i: sync→healthy = $(( END_MS - START_MS ))ms"
      break
    fi
    sleep 2
  done
done
```

**采集指标**：

| 指标 | 采集方式 |
|------|----------|
| sync → Healthy 总延迟 | 时间戳差值 |
| OCI 制品拉取/render 延迟 | Argo CD sync_result 时间线 |
| K8s resource create → ready 延迟 | K8s events 时间戳 |
| Argo CD health check 延迟 | App status operationState 时间线 |

**达标判定**：中位数 < 90s（G3 达标线）。

---

### 场景 4：Application 状态变更的 watch/informer 延迟

**目的**：验证平台通过 informer/watch 模式感知 Application 状态变更的延迟。架构文档 §5.4 要求实时 watch + 5 分钟全量对账。

**步骤**：

```bash
# 4a. 状态变更感知延迟
# 修改某个 App 的 deployment replica（直接 kubectl），观察 Argo CD 多久反映状态变化
APP="bench-dev-050"
DEPLOYMENT="bench-dev-050"

# 触发变更
START_MS=$(date -u +%s%3N)
kubectl scale deployment "$DEPLOYMENT" -n bench-dev --replicas=2

# 轮询 Argo CD App 状态直到显示 OutOfSync
while true; do
  SYNC_STATUS=$(argocd app get "$APP" -o json | jq -r '.status.sync.status')
  if [ "$SYNC_STATUS" = "OutOfSync" ]; then
    END_MS=$(date -u +%s%3N)
    echo "Status change detected in $(( END_MS - START_MS ))ms"
    break
  fi
  sleep 1
done

# 4b. 恢复后再测 self-heal 延迟（开启 auto-sync 的场景）
# 4c. 400 App 同时发生状态变更（模拟滚动重启）
# 对 50 个 App 同时 scale，测量 Argo CD controller 的处理吞吐
for i in $(seq 1 50); do
  APP="bench-dev-$(printf '%03d' "$i")"
  kubectl scale deployment "$APP" -n bench-dev --replicas=2 &
done
wait
# 记录所有 App 反映 OutOfSync 的时间分布
```

**采集指标**：

| 指标 | 采集方式 |
|------|----------|
| 资源变更 → Argo CD 状态更新延迟 | 时间戳差值 |
| application-controller informer resync 周期 | Prometheus `workqueue_depth` |
| 50 App 并发状态变更吞吐 | 变更开始 → 最后一个 App 反映状态的时间 |

**达标判定**：单 App 状态变更感知 < 5s（G6）；50 App 并发变更全部感知 < 30s。

---

### 场景 5：平台 API 触发到 sync 完成的端到端延迟

**目的**：测量平台托管模式全链路延迟——从平台 API（或 Argo CD API）更新 Application values 到 sync 完成。对应 dev/test 环境通过平台触发部署的场景。

**步骤**：

```bash
# 5a. 单个 App 的 API 触发 → sync 全链路
APP="bench-dev-001"
START_MS=$(date -u +%s%3N)

# 通过 Argo CD API 更新 Application 的 Helm values
argocd app set "$APP" \
  --helm-set appId="bench-dev-001-modified"

# 触发 sync
argocd app sync "$APP" --timeout 300 >/dev/null 2>&1

# 轮询直到 sync 完成
while true; do
  HEALTH_STATUS=$(argocd app get "$APP" -o json | jq -r '.status.health.status')
  SYNC_STATUS=$(argocd app get "$APP" -o json | jq -r '.status.sync.status')

  if [ "$HEALTH_STATUS" = "Healthy" ] && [ "$SYNC_STATUS" = "Synced" ]; then
    END_MS=$(date -u +%s%3N)
    echo "API trigger → sync complete = $(( END_MS - START_MS ))ms"
    break
  fi
  sleep 2
done

# 5b. 通过平台 API 触发（如果平台封装了部署接口）
# curl -X PUT https://platform-api/services/bench-dev-001/deploy \
#   -H 'Content-Type: application/json' \
#   -d '{"values":{"appId":"bench-dev-001-modified"}}'
```

**采集指标**：

| 指标 | 采集方式 |
|------|----------|
| API 调用 → Argo CD 开始处理延迟 | `operationState.startedAt` - API 调用时间戳 |
| Argo CD 检测 → sync 开始延迟 | `operationState.startedAt` - 参数更新时间 |
| sync 开始 → Healthy 延迟 | `operationState.finishedAt` - `startedAt` |
| 端到端总延迟 | API 调用时间 → Healthy 时间 |

**达标判定**：端到端 < 120s（G7 达标线）。

---

## 4. 监控指标

### 4.1 Argo CD Prometheus Metrics

| Metric | 说明 | 对应目标 |
|--------|------|----------|
| `argocd_app_info` | Application 基本信息（含 sync_status, health_status labels） | G1, G3 |
| `argocd_app_sync_total` | sync 操作计数 | G2, G4, G5 |
| `argocd_app_sync_status_timestamp` | 最近一次 sync 状态更新时间戳 | G4, G5 |
| `argocd_app_k8s_request_total` | K8s API 请求计数 | 后台监控 |
| `argocd_cluster_api_resource_objects` | 监控的 K8s 资源对象数 | 容量规划 |
| `argocd_git_request_total` | Git 请求计数（fetch/ls-remote） | 场景 3 |
| `argocd_git_request_duration_seconds_bucket` | Git 请求延迟分布 | 场景 3 |
| `argocd_repo_server_request_total` | repo-server GRPC 请求计数 | G3 |
| `argocd_repo_server_request_duration_seconds_bucket` | repo-server 请求延迟 | G3 |
| `grpc_server_handling_seconds_bucket` | GRPC 请求延迟（按 method 分） | G1 |
| `workqueue_depth` | controller 工作队列深度 | G4, G5, G6 |
| `rest_client_requests_total` | K8s API 调用计数 | 后台监控 |

### 4.2 K8s & 基础设施指标

| Metric | 说明 | 采集来源 |
|--------|------|----------|
| `container_cpu_usage_seconds_total` | 容器 CPU 使用率 | cAdvisor |
| `container_memory_working_set_bytes` | 容器内存使用 | cAdvisor |
| `kube_pod_status_phase` | Pod 状态分布 | kube-state-metrics |
| `kube_deployment_status_replicas_available` | Deployment 可用副本数 | kube-state-metrics |
| `etcd_request_duration_seconds` | etcd 请求延迟（间接反映 K8s API 压力） | etcd metrics |

### 4.3 自定义采集指标

压测脚本中记录的时间戳数据，可通过 pushgateway 或直接写入文件后分析：

| 自定义指标 | 说明 |
|-----------|------|
| `bench_sync_trigger_to_complete_ms` | sync 触发到完成的毫秒数 |
| `bench_api_trigger_to_healthy_ms` | API 触发到 Healthy 的毫秒数 |
| `bench_status_change_detect_ms` | 状态变更感知延迟毫秒数 |
| `bench_concurrent_sync_total_duration_ms` | 并发 sync 全部完成的毫秒数 |

### 4.4 Grafana Dashboard

- **官方 Dashboard**：导入 Argo CD 官方 Dashboard（Grafana ID: 14592）
- **压测专用 Dashboard**：创建包含以下 panel：
  - App Sync Status 分布（pie chart: Synced / OutOfSync / Unknown）
  - App Health Status 分布（pie chart: Healthy / Progressing / Degraded）
  - sync 操作吞吐（rate over time）
  - Argo CD 组件 CPU/内存趋势
  - GRPC 请求延迟热力图
  - 压测自定义指标时间线

---

## 5. 结果分析标准

### 5.1 达标矩阵

| 场景 | 核心指标 | 达标线 | 告警线 | 严重不达标 |
|------|----------|--------|--------|-----------|
| 1-列表查询 | p95 响应延迟 | < 1s | < 3s | > 5s |
| 1-列表查询 | 50 QPS 错误率 | 0% | < 1% | > 5% |
| 2-10并发 sync | 全部完成时间 | < 3min | < 5min | > 10min |
| 2-20并发 sync | 全部完成时间 | < 5min | < 10min | > 15min |
| 2-50并发 sync | 全部完成时间 | < 8min | < 15min | > 20min |
| 2-并发 sync | sync 失败率 | 0% | < 5% | > 10% |
| 3-单 App sync | sync→Healthy 中位数 | < 90s | < 180s | > 300s |
| 4-状态感知 | 单 App 变更检测延迟 | < 5s | < 15s | > 30s |
| 4-状态感知 | 50 App 并发变更全部感知 | < 30s | < 60s | > 120s |
| 5-API 触发 | 端到端延迟 | < 120s | < 300s | > 600s |
| 稳态资源 | server 内存 | < 1Gi | < 2Gi | > 3Gi |
| 稳态资源 | controller 内存 | < 2Gi | < 4Gi | > 6Gi |
| 稳态资源 | server CPU 均值 | < 500m | < 1000m | > 2000m |
| 稳态资源 | controller CPU 均值 | < 1000m | < 2000m | > 3000m |
| 稳态资源 | redis 内存 | < 512Mi | < 1Gi | > 2Gi |

### 5.2 判定规则

- **全部达标**：✅ Argo CD 可支撑 400+ Application 规模，平台可按计划推进
- **部分告警**：⚠️ 记录瓶颈点，在 M6 稳定化阶段针对性优化
- **严重不达标**：🔴 启动优化预案（见 §6），必要时调整架构方案

### 5.3 压测报告模板

```
## Argo CD 压测报告

### 环境信息
- K8s 版本 / 节点数 / 节点规格
- Argo CD 版本 / 组件资源配置
- 测试时间 / 执行人

### 结果汇总表
| 场景 | 指标 | 达标线 | 实际值 | 判定 |
|------|------|--------|--------|------|

### 详细数据
（每场景 3 轮原始数据 + 中位数）

### 瓶颈分析
（如有不达标项，分析瓶颈位置）

### 结论与建议
（通过 / 需优化 / 需架构调整）
```

---

## 6. 优化预案

按瓶颈位置分层，从低成本到高成本排列。压测不达标时按序尝试。

### 6.1 Argo CD 参数调优（零成本）

| 参数 | 调优方向 | 场景 | 预期收益 |
|------|----------|------|----------|
| `--status-processors` | 20 → 30~50 | 场景 4 状态感知慢 | 增加 Application 状态处理并发 |
| `--operation-processors` | 10 → 20~30 | 场景 2 并发 sync 排队 | 增加 sync 操作处理并发 |
| `controller.app.resync` | 180s → 60s | 场景 4 全量对账慢 | 缩短全量 resync 间隔（注意会增加 CPU 开销） |
| `ARGOCD_RECONCILIATION_TIMEOUT` | 调整超时 | sync 超时失败 | 适当增大超时容忍 |
| `server.repo.server.timeout.seconds` | 60 → 120 | OCI/Git 操作超时 | 大 Chart 拉取更稳定 |
| Redis `maxmemory-policy` | allkeys-lru → volatile-ttl | Redis 内存增长快 | 更合理的缓存淘汰 |
| Application `syncOptions` | 启用 `PruneLast=true` | 大量资源 prune 慢 | 分批处理 prune |

### 6.2 资源调整（低成本）

| 组件 | 调整方向 | 触发条件 |
|------|----------|----------|
| argocd-application-controller | 增加 CPU limit / memory limit | CPU 长期 > 80% 或 OOM |
| argocd-repo-server | 扩展到 3~4 replicas | OCI 制品拉取排队 |
| argocd-redis | 增加内存 limit | 缓存命中率低 / eviction 频繁 |
| argocd-server | 扩展到 3 replicas | API 查询 p95 不达标 |

### 6.3 Application Controller 水平扩展（中等成本）

Argo CD 支持 ApplicationController 分片（sharding），将 400 个 App 分配到多个 controller 实例：

```yaml
# 设置 controller replicas + 分片策略
# v2.12+ 支持基于 cluster 数或 instance 数的分片
spec:
  replicas: 2
  # 每个 controller 负责 ~200 个 Application
```

**适用场景**：单个 controller CPU/内存持续高位，状态处理延迟不达标。

### 6.4 Repo Server 缓存优化（中等成本）

```yaml
# 增加 repo-server 本地缓存
env:
  - name: ARGOCD_REPO_CACHE_DIR
    value: /tmp/repo-cache
  - name: ARGOCD_REPO_CACHE_SIZE
    value: "10"

# 增加 repo-server 并发处理
# OCI 制品默认有 layer 缓存，此处调优本地渲染缓存
```

### 6.5 架构级优化（高成本，需评估）

| 方案 | 说明 | 适用场景 |
|------|------|----------|
| **多 Argo CD 实例** | 按环境拆分：dev/test 一个实例，pre/prod 一个实例。平台层做路由 | 单实例优化后仍不达标 |
| **App-of-Apps 模式** | 用一个 root App 管理一批子 App，减少 Application CRD 数量 | 简化管理，但增加 sync 复杂度 |
| **ApplicationSet** | 用 ApplicationSet 替代独立 Application CRD，减少 controller watch 压力 | 适合批量同质 App |
| **独立集群部署 Argo CD** | Argo CD 独占一个管理集群，避免与业务集群资源竞争 | 避免噪音邻居问题 |

### 6.6 平台层优化

| 方案 | 说明 |
|------|------|
| informer 缓存优化 | 本地缓存 Application 状态，减少回查 Argo CD API。架构已设计 informer 模式（§5.4），确保实现层到位 |
| 批量 API 调用 | 平台需要查询多个 App 状态时，使用 Argo CD 批量 API 或本地 informer 缓存，避免 N+1 查询 |
| WebSocket 增量推送 | 状态变更通过 watch 事件驱动推送，非全量轮询 |
| 查询分页 | App 列表 API 强制分页（默认 page_size=50），避免单次返回 400 条 |

---

## 7. 执行计划

| 阶段 | 内容 | 预估时间 |
|------|------|----------|
| 准备 | 环境搭建、脚本编写、Chart 制作 | 0.5 天 |
| 预热 | 部署 400 App，等待稳态，采集基线 | 0.5 天 |
| 场景 1-3 | 列表查询 + 并发 sync + 单 App 延迟 | 0.5 天 |
| 场景 4-5 | 状态感知 + API 触发全链路 | 0.5 天 |
| 分析 | 数据整理、报告编写、瓶颈分析 | 0.5 天 |
| **总计** | | **2.5 天** |

### 7.1 前置依赖

- [ ] 测试 K8s 集群可用（至少 3 worker 节点，16GB+ memory）
- [ ] Harbor OCI 制品仓库访问权限
- [ ] Argo CD 安装配置权限
- [ ] Prometheus + Grafana 可用

### 7.2 交付物

- [ ] 压测报告（按 §5.3 模板）
- [ ] Grafana Dashboard 截图（稳态 + 压测期间）
- [ ] 原始数据文件（JSON / CSV）
- [ ] 优化建议（如有不达标项）

---

## 附录 A：Argo CD 性能参考基线

基于 Argo CD 官方和社区数据的经验值（非官方 SLA）：

| 规模 | 典型表现 | 来源 |
|------|----------|------|
| 100 App | 列表查询 < 500ms，单 App sync < 60s | 社区反馈 |
| 200 App | 列表查询 < 1s，需关注 controller 内存 | 社区反馈 |
| 500 App | 列表查询 1~3s，建议分片或调优 | 官方 Slack 讨论 |
| 1000+ App | 建议多实例 / 分片 | 官方建议 |

400 App 处于「单实例可处理但需关注」的区间，本方案旨在用实测数据验证。

## 附录 B：快速排障指南

| 现象 | 可能原因 | 排查方法 |
|------|----------|----------|
| sync 一直 Pending | operation processor 队列堆积 | 检查 `workqueue_depth`，增加 `--operation-processors` |
| App 状态长期 Unknown | controller 未及时处理 | 检查 controller 日志、CPU/内存是否到 limit |
| OCI 制品拉取超时 | repo-server 资源不足 | 检查 repo-server 副本数和 CPU |
| API 查询超时 | server 资源不足或 Redis 缓存失效 | 检查 Redis 状态、server CPU |
| 状态更新延迟大 | informer resync 周期过长 | 缩短 `controller.app.resync`，检查 K8s API 延迟 |
| OOM Kill | App 数量多导致内存不够 | 增加 memory limit，考虑分片 |
