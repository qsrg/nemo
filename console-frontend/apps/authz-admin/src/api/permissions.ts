/**
 * 人员授权 API 包装（Phase C 去角色化）：
 * 消费 authz 授权面 /v1/admin/permissions/*，字段核对
 * services/authz/internal/types/types.go：
 * - GrantPermissionsReq{targetType, targetId, permissions[]} → {granted}
 * - RevokePermissionReq{targetType, targetId, permission} → {revoked}
 * - ListPermissionGrantsReq{targetType, targetId} → {codes[]}
 * - PermissionOverviewReq{userId} → {direct[], viaOrgs[{orgId,orgName,codes}], roles[]}
 *
 * 授权 = 人/组织 × 权限点 一条 can 元组；组织主体（org:{id}#member）成员含
 * parent 链继承均生效；写入即时生效（判定面实时查询）。
 * 注：@nemo/http 的 get/delete 不带 params，query 用字符串内联（同目录约定）。
 */
import { http } from './client'

/** 授予若干权限点（幂等：重复授不重复落库）。targetType：user|org。 */
export function grantPermissions(data: { targetType: string; targetId: string; permissions: string[] }) {
  return http.post<{ granted: number }>('/admin/permissions/grants', data)
}

/** 撤销单个权限点（对称）。 */
export function revokePermission(data: { targetType: string; targetId: string; permission: string }) {
  // @nemo/http 的 delete 无 body，query 携带 permission
  return http.delete<{ revoked: number }>(
    `/admin/permissions/grants?targetType=${encodeURIComponent(data.targetType)}&targetId=${encodeURIComponent(data.targetId)}&permission=${encodeURIComponent(data.permission)}`,
  )
}

/** 某主体当前的直接权限码（授权面读回显）。 */
export function listPermissionGrants(targetType: string, targetId: string) {
  return http.get<{ codes: string[] }>(
    `/admin/permissions/grants?targetType=${encodeURIComponent(targetType)}&targetId=${encodeURIComponent(targetId)}`,
  )
}

/** 某用户的权限总览：直接授予 + 经部门继承 + 过渡期角色。 */
export function permissionOverview(userId: string) {
  return http.get<{
    direct: string[]
    viaOrgs: { orgId: string; orgName: string; codes: string[] }[]
    roles: string[]
  }>(`/admin/permissions/overview?userId=${encodeURIComponent(userId)}`)
}

/** 反向查询：某权限码的直接授权主体（管理面审计用，kind：user|org）。 */
export function listPermissionSubjects(permission: string) {
  return http.get<{ subjects: { kind: 'user' | 'org'; id: string }[] }>(
    `/admin/permissions/subjects?permission=${encodeURIComponent(permission)}`,
  )
}
