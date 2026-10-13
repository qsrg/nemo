/**
 * 鉴权属性 API 包装（ABAC 方案 A）：
 * 消费 authz 管理 API /v1/admin/attributes/{subject,resource}*，字段核对
 * services/authz/internal/types/types.go：
 * - SetSubjectAttributesReq{subject,attrs} / GetSubjectAttributesResp{subject,attrs}
 * - SetResourceAttributesReq{resource,attrs} / GetResourceAttributesResp{resource,attrs}
 * - DELETE 按 owner+key 幂等删除
 *
 * 定位：主体/资源属性是"是什么"的描述，由 ABAC deny 策略在鉴权链路叠加
 * 使用；业务表不再承载鉴权门槛字段（旧 required_level 已删）。
 */
import { http } from './client'

/** 属性条目（key → value，value 为字符串）。 */
export type AttrMap = Record<string, string>

/** 查询主体属性。 */
export function getSubjectAttributes(subject: string) {
  return http.get<{ subject: string; attrs: AttrMap }>('/admin/attributes/subject', {
    params: { subject },
  })
}

/** 覆盖式设置主体属性（对每个 key upsert，未传的 key 保留）。 */
export function setSubjectAttributes(subject: string, attrs: AttrMap) {
  return http.post<{ subject: string }>('/admin/attributes/subject', { subject, attrs })
}

/** 删除主体单条属性（幂等）。 */
export function deleteSubjectAttribute(subject: string, key: string) {
  return http.delete<void>('/admin/attributes/subject', { params: { subject, key } })
}

/** 查询资源属性。 */
export function getResourceAttributes(resource: string) {
  return http.get<{ resource: string; attrs: AttrMap }>('/admin/attributes/resource', {
    params: { resource },
  })
}

/** 覆盖式设置资源属性。 */
export function setResourceAttributes(resource: string, attrs: AttrMap) {
  return http.post<{ resource: string }>('/admin/attributes/resource', { resource, attrs })
}

/** 删除资源单条属性（幂等）。 */
export function deleteResourceAttribute(resource: string, key: string) {
  return http.delete<void>('/admin/attributes/resource', { params: { resource, key } })
}
