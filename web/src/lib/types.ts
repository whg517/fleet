// =====================
// 通用类型
// =====================

export interface PaginationParams {
  page?: number;
  pageSize?: number;
  keyword?: string;
}

export interface PaginatedResult<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

// =====================
// 服务目录
// =====================

export interface Service {
  id: string;
  name: string;
  description: string;
  language: "go" | "java" | "python" | "nodejs" | "rust";
  repository: string;
  owner: string;
  team: string;
  status: "active" | "archived" | "deprecated";
  tags: string[];
  createdAt: string;
  updatedAt: string;
}

// =====================
// 部署
// =====================

export type DeploymentStatus =
  | "pending"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled";

export interface Deployment {
  id: string;
  serviceId: string;
  serviceName: string;
  environment: "dev" | "staging" | "production";
  version: string;
  status: DeploymentStatus;
  initiator: string;
  startedAt: string;
  finishedAt?: string;
  logs?: string;
}

// =====================
// 发布审批
// =====================

export type ApprovalStatus = "pending" | "approved" | "rejected";

export interface Approval {
  id: string;
  deploymentId: string;
  serviceName: string;
  version: string;
  environment: "dev" | "staging" | "production";
  status: ApprovalStatus;
  requester: string;
  approver?: string;
  comment?: string;
  createdAt: string;
  resolvedAt?: string;
}

// =====================
// 集群
// =====================

export interface Cluster {
  id: string;
  name: string;
  provider: "kubernetes" | "nomad" | "docker-swarm";
  endpoint: string;
  status: "healthy" | "degraded" | "offline";
  nodeCount: number;
  version: string;
  region: string;
  createdAt: string;
}

// =====================
// 审计日志
// =====================

export interface AuditLog {
  id: string;
  actor: string;
  action: string;
  resource: string;
  resourceId: string;
  detail?: string;
  ip: string;
  userAgent: string;
  timestamp: string;
}

// =====================
// 用户
// =====================

export interface User {
  id: string;
  username: string;
  email: string;
  avatar?: string;
  role: "admin" | "developer" | "operator" | "viewer";
  team: string;
}

// =====================
// 系统设置
// =====================

export interface SystemSetting {
  key: string;
  value: string;
  description: string;
  category: string;
  updatedAt: string;
}
