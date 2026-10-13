/**
 * Zookeeper 节点浏览 API 包装：
 * 消费 zookeeper 服务 API /v1/zookeeper/clusters*，字段核对
 * services/zookeeper/internal/types/types.go：
 * - ClusterResp{id,code,hosts,remark,createdBy,updatedAt,canManageGrants,owner}（全量，不分页）
 * - ZnodeTreeResp{clusterId,code,root}（节点树，只读浏览）
 *
 * 可见性口径："看到集群才能浏览节点"——集群列表按引擎 can_view 过滤
 * （manage 码全量），节点树接口按集群 can_view 守卫（同一判定面）；集群
 * 进引擎（zk_cluster），创建写 owner 元组、创建人即名单管理员。
 * 阶段一 znode 数据为 mock 演示树（接口即生产形态），阶段二按 hosts 连真实 ZK。
 */
import { createMicroHttp } from '@nemo/shared'

/** 微应用统一 http 工厂（基础能力层）：token 实时取 session、401 竞态区分。 */
export const http = createMicroHttp()

/** 集群行（zookeeper ClusterResp；code 唯一且创建后不可改——对象身份不可变；
 *  canManageGrants = owner 关系 + zookeeper:manage 码回退，与后端守卫同源）。 */
export interface ZkCluster {
  id: string
  code: string
  hosts: string
  remark: string
  createdBy: string
  updatedAt: string
  canManageGrants: boolean
  owner: boolean
}

/** 集群全量列表（id 升序；集群量级个位/十级，选择器场景不分页）。 */
export function listClusters() {
  return http.get<{ items: ZkCluster[] }>('/zookeeper/clusters')
}

/** 新建集群（需 zookeeper:manage 码；重码 409）。 */
export function createCluster(data: { code: string; hosts: string; remark: string }) {
  return http.post<ZkCluster>('/zookeeper/clusters', data)
}

/** 删除集群（需 zookeeper:manage 码）。 */
export function deleteCluster(id: string) {
  return http.delete<void>(`/zookeeper/clusters/${encodeURIComponent(id)}`)
}

/** znode 节点（阶段一 mock 演示树）。 */
export interface Znode {
  path: string
  name: string
  data: string
  children: Znode[]
}

/** 节点树（集群可见性守卫：不可见集群 403——看到集群才能浏览节点）。 */
export function listNodes(clusterId: string) {
  return http.get<{ clusterId: string; code: string; root: Znode }>(
    `/zookeeper/clusters/${encodeURIComponent(clusterId)}/nodes`,
  )
}
