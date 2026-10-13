/**
 * Redis 实例管理 API 包装：
 * 消费 redis 服务 API /v1/redis/clusters* 与 /v1/redis/instances*，字段核对
 * services/redis/internal/types/types.go：
 * - ClusterResp{id,code,remark,createdBy,updatedAt,canManageGrants,owner}（集群全量列表，不分页）
 * - InstanceResp{id,name,addr,description,createdBy,canEdit,canDelete,canManageGrants,owner}
 * - CreateInstanceReq{clusterId,name,addr,description}
 * - ListInstancesReq{clusterId,page,pageSize}（先选集群再出实例；分页：纯引擎
 *   可见性 ID 集合 IN 下推 + 集群谓词 AND 组合 + 批量复核）
 *
 * 集群可见性为双通道（进引擎）：引擎授权（redis_cluster can_view——空集群
 * 也可授权）∪ 实例派生（可见实例归属的集群）；写操作挂 redis:manage 码，
 * 行内「集群权限」入口 = owner 或 manage 码（行级徽标驱动）。
 * 实例可见性设计（与 kafka 的 org_id 列谓词方案不同）：无归属列，纯引擎口径
 * （owner/viewer/editor 关系）——创建后由 owner 经通用 ResourceGrantDialog
 * （/v1/authz/relations）按需授予他人。
 */
import { createMicroHttp } from '@nemo/shared'

/** 微应用统一 http 工厂（基础能力层）：token 实时取 session、401 竞态区分。 */
export const http = createMicroHttp()

/** 集群行（redis ClusterResp；code 唯一且创建后不可改——对象身份不可变；
 *  canManageGrants = owner 关系 + redis:manage 码回退，与后端守卫同源）。 */
export interface RedisCluster {
  id: string
  code: string
  remark: string
  createdBy: string
  updatedAt: string
  canManageGrants: boolean
  owner: boolean
}

/** 集群全量列表（id 升序；集群量级个位/十级，选择器场景不分页）。 */
export function listClusters() {
  return http.get<{ items: RedisCluster[] }>('/redis/clusters')
}

/** 新建集群（需 redis:manage 码；重码 409）。 */
export function createCluster(data: { code: string; remark: string }) {
  return http.post<RedisCluster>('/redis/clusters', data)
}

/** 删除集群（需 redis:manage 码；集群下仍有实例时 409 拒绝）。 */
export function deleteCluster(id: string) {
  return http.delete<void>(`/redis/clusters/${encodeURIComponent(id)}`)
}

/** 实例行（redis InstanceResp，含逐条判定徽标；canManageGrants = owner 关系
 *  + redis:manage 码回退，与后端守卫同源，前端直接消费、不再用 owner 自行
 *  推导；owner 为纯 owner 关系，授权对话框初始态用）。 */
export interface RedisInstance {
  id: string
  name: string
  addr: string
  description: string
  createdBy: string
  canEdit: boolean
  canDelete: boolean
  canManageGrants: boolean
  owner: boolean
}

/** 实例列表（先选集群再出实例：clusterId 必传；分页：纯引擎可见 ID 集合
 *  IN 下推 + 集群谓词 AND 组合 + 批量复核，total 为近似值）。 */
export function listInstances(clusterId: string, page = 1, pageSize = 20) {
  return http.get<{ items: RedisInstance[]; total: number }>(
    `/redis/instances?clusterId=${encodeURIComponent(clusterId)}&page=${page}&pageSize=${pageSize}`,
  )
}

/** 新建实例（创建人即 owner；归属当前选中集群；需 redis:create 动作码或
 *  redis:manage 管理码）。 */
export function createInstance(data: { clusterId: string; name: string; addr: string; description: string }) {
  return http.post<RedisInstance>('/redis/instances', data)
}

/** 更新实例（需该实例 can_edit 或 redis:manage 码；addr/description 全量覆盖）。 */
export function updateInstance(id: string, data: { addr: string; description: string }) {
  return http.put<RedisInstance>(`/redis/instances/${encodeURIComponent(id)}`, data)
}

/** 删除实例（需该实例 can_delete 或 redis:manage 码；删库 + 清全部元组）。 */
export function deleteInstance(id: string) {
  return http.delete<void>(`/redis/instances/${encodeURIComponent(id)}`)
}
