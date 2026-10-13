/**
 * Kafka 集群管理 API 包装：消费 kafka 服务 /v1/kafka/clusters*（集群管理页）。
 * 字段核对 services/kafka/internal/types/types.go：
 * - ClusterResp{id,code,dc,address,remark,createdBy,updatedAt}
 * - CreateClusterReq{code,dc,address,remark}
 * - UpdateClusterReq{id 路径参数 + dc/address/remark（code 对象身份不可改）}
 * - ListClustersReq{page,pageSize}
 *
 * 权限模型：页面入口由 kafka-clusters:read（菜单码）控制，写操作（新建/编辑/
 * 删除）由 kafka-clusters:manage 控制——集群是基础配置数据，无资源关系模型，
 * 管理档天然全权（无按钮动作码，与人员授权三档语义一致）。前端按钮按
 * usePermission().canManage('kafka-clusters') 显隐。
 */
import { createMicroHttp } from '@nemo/shared'

/** 微应用统一 http 工厂（基础能力层）：token 实时取 session、401 竞态区分。 */
export const http = createMicroHttp()

/** 集群行（kafka ClusterResp）。 */
export interface KafkaCluster {
  id: number
  code: string
  dc: string
  address: string
  remark: string
  createdBy: string
  updatedAt: string
}

/** 分页列表。 */
export function listClusters(page = 1, pageSize = 20) {
  return http.get<{ items: KafkaCluster[]; total: number }>('/kafka/clusters', {
    page,
    pageSize,
  })
}

/** 新建集群（code 必填、业务唯一）。 */
export function createCluster(data: {
  code: string
  dc: string
  address: string
  remark: string
}) {
  return http.post<KafkaCluster>('/kafka/clusters', data)
}

/** 编辑集群（code 不可改：对象身份不可变，只传属性）。 */
export function updateCluster(
  id: number,
  data: { dc: string; address: string; remark: string },
) {
  return http.put<null>(`/kafka/clusters/${id}`, data)
}

/** 删除集群。 */
export function deleteCluster(id: number) {
  return http.delete<null>(`/kafka/clusters/${id}`)
}
