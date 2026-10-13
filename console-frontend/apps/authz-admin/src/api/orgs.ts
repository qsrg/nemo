/**
 * 组织/成员查询 API（只读）：
 * 消费 authz 管理 API /v1/admin/orgs*，字段核对 services/authz/internal/types/types.go
 * （OrgResp{id,name,parentId}、MemberResp{orgId,userId}、MemberListResp{members}）。
 * 组织事实源在 CAS（orgsync 全量对账落库），前端已下线手工维护入口——
 * 创建/改名/删除/成员增删等后端接口保留仅作内部与过渡期兜底，本文件不再封装。
 * 统一经 client.ts 的 http 实例（baseURL /api/v1，token 实时取基座 props）；
 * 错误统一由调用方走 showApiError。
 */
import { http } from './client'

/** 组织信息（authz OrgResp）。 */
export interface Org {
  id: string
  name: string
  parentId: string
}

/** 组织直接成员（authz MemberResp）。 */
export interface OrgMember {
  orgId: string
  userId: string
}

/** CAS 组织同步结果摘要（authz SyncOrgsResp，一轮对账的写入动作统计）。 */
export interface SyncOrgsResp {
  /** 注册表 upsert 条数（新建与更新合计，幂等轮也计入）。 */
  orgsUpserted: number
  parentsWritten: number
  parentsDeleted: number
  membersWritten: number
  membersDeleted: number
  /** 删除的组织数（CAS 已撤销且无授权引用）。 */
  orgsDeleted: number
  /** 因仍有授权/子组织引用而跳过删除的组织数。 */
  orgsSkipped: number
  /** orgId 不在组织架构中而被跳过的用户数。 */
  danglingUsers: number
}

/** 手动触发 CAS 组织架构同步（全量对账；与后端定时任务同入口，幂等）。 */
export function syncOrgs() {
  return http.post<SyncOrgsResp>('/admin/orgs/sync')
}

/** 组织列表（全量，量级小；后端按 id 升序返回）。 */
export function listOrgs() {
  return http.get<{ items: Org[]; total: number }>('/admin/orgs')
}

/** 列出组织直接成员（不含沿层级的继承成员——继承由判定面承担）。 */
export function listOrgMembers(id: string) {
  return http.get<{ members: OrgMember[] }>(`/admin/orgs/${id}/members`)
}
