/**
 * ABAC 策略 API 包装（方案 A）：
 * 消费 authz 管理 API /v1/admin/policies*，字段核对
 * services/authz/internal/types/types.go：
 * - PolicyItem{id,name,resourceType,effect,subjectAttr,op,resourceAttr,value,enabled}
 * - CreatePolicyReq / UpdatePolicyReq（op 白名单 lt/lte/gt/gte/eq/neq；
 *   effect 白名单 deny/allow，缺省 deny）
 *
 * 定位：策略叠加于 ReBAC 授权之上——deny（拒绝）命中即拒绝，优先于一切
 * 授权；allow（正向门槛）对适用资源须至少命中一条才放行（语义见后端
 * logic/abac.go）。enabled=false 一律不评估（回滚开关）。
 */
import { http } from './client'

/** 一条 ABAC 策略（effect：deny 拒绝 | allow 正向门槛）。 */
export interface Policy {
  id: string
  name: string
  resourceType: string
  effect: string
  subjectAttr: string
  op: string
  resourceAttr: string
  value: string
  enabled: boolean
}

/** 策略写入载荷（创建/更新共用，id 路径参数区分）。 */
export interface PolicyInput {
  name: string
  resourceType: string
  effect: string
  subjectAttr: string
  op: string
  resourceAttr: string
  value: string
  enabled: boolean
}

/** 新建策略。 */
export function createPolicy(data: PolicyInput) {
  return http.post<{ policy: Policy }>('/admin/policies', data)
}

/** 策略列表（按 ID 升序）。 */
export function listPolicies() {
  return http.get<{ items: Policy[]; total: number }>('/admin/policies')
}

/** 更新策略（id 路径参数，其余字段全量覆盖）。 */
export function updatePolicy(id: string, data: PolicyInput) {
  return http.put<{ policy: Policy }>(`/admin/policies/${encodeURIComponent(id)}`, data)
}

/** 删除策略。 */
export function deletePolicy(id: string) {
  return http.delete<void>(`/admin/policies/${encodeURIComponent(id)}`)
}
