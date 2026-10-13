/**
 * RocketMQ API 包装（集群级授权模型，2026-10-12 定稿）：
 * 消费 rocketmq 独立服务 /v1/rocketmq/clusters* 与 /topics*，字段与
 * services/rocketmq/internal/types/types.go 一致：
 * - RocketmqClusterResp{id,code,remark,createdBy,updatedAt,canManageGrants}
 * - RocketmqTopicResp{id,name,env,description,clusterId,orgId,createdBy}
 * - CreateRocketmqTopicReq{name,env,description,clusterId,orgId}
 * - UpdateRocketmqTopicReq{id 路径参数 + env/description}
 *
 * 授权模型（与 redis 集群同构）：
 * - 集群可见（rmq_cluster can_view）即该集群下全部 topic 可见可操作，
 *   topic 无独立行级判定字段，操作按钮恒显示（后端集群级守卫兜底）；
 * - 列表读取无守卫（页面入口菜单码过滤），新建/删除集群仅管理码
 *   （rocketmq-topics:manage）；
 * - 新建 topic 挂 create 动作码或管理码；编辑/删除按 topic 归属集群做
 *   集群 can_view 判定（路由守卫）。
 */
import { createMicroHttp } from '@nemo/shared'

/** 微应用统一 http 工厂（基础能力层）：token 实时取 session、401 竞态区分。 */
export const http = createMicroHttp()

/** 集群行（rmq_cluster ClusterResp；code 唯一且创建后不可改——对象身份
 *  不可变；canManageGrants = owner 关系 + 管理码回退，与后端守卫同源）。 */
export interface RocketmqCluster {
  id: string
  code: string
  remark: string
  createdBy: string
  updatedAt: string
  canManageGrants: boolean
}

/** 集群全量列表（id 升序；集群量级个位/十级，选择器场景不分页）。 */
export function listClusters() {
  return http.get<{ items: RocketmqCluster[] }>('/rocketmq/clusters')
}

/** 新建集群（需 rocketmq-topics:manage 码；重码 409）。 */
export function createCluster(data: { code: string; remark: string }) {
  return http.post<RocketmqCluster>('/rocketmq/clusters', data)
}

/** 删除集群（需 rocketmq-topics:manage 码；集群下仍有 topic 时 409 拒绝）。 */
export function deleteCluster(id: string) {
  return http.delete<void>(`/rocketmq/clusters/${encodeURIComponent(id)}`)
}

/** Topic 行（RocketmqTopicResp；集群级模型下无行级判定字段）。 */
export interface RocketmqTopic {
  id: string
  name: string
  env: string
  description: string
  clusterId: string
  orgId: string
  createdBy: string
}

/** Topic 列表：按集群加载（clusterId 必传；集群不可见返回空列表）。 */
export function listRocketmqTopics(clusterId: string) {
  return http.get<{ items: RocketmqTopic[]; total: number }>(
    `/rocketmq/topics?clusterId=${encodeURIComponent(clusterId)}`,
  )
}

/** 新建 Topic（后端守卫：rocketmq-topics:create 动作码或管理码；logic 再
 *  校验目标集群可见——防建到不可见集群）。clusterId 必传。 */
export function createRocketmqTopic(data: {
  name: string
  env: string
  description: string
  clusterId: string
  orgId?: string
}) {
  return http.post<RocketmqTopic>('/rocketmq/topics', {
    ...data,
    // 后端契约 clusterId 为 int64（JSON 数字），前端选择器值是 string，需转换。
    clusterId: Number(data.clusterId),
  })
}

/** 更新 Topic（后端守卫：topic 归属集群 rmq_cluster:can_view 或管理码）。 */
export function updateRocketmqTopic(id: string, data: {
  env: string
  description: string
}) {
  return http.put<RocketmqTopic>(`/rocketmq/topics/${encodeURIComponent(id)}`, data)
}

/** 删除 Topic（后端守卫：topic 归属集群 rmq_cluster:can_view 或管理码）。 */
export function deleteRocketmqTopic(id: string) {
  return http.delete<void>(`/rocketmq/topics/${encodeURIComponent(id)}`)
}
