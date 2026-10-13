/**
 * 共享管理 API 包装（Task18/B10）：
 * 消费 authz 判定面 /v1/authz/*（普通 JWT 组，非 admin 组），字段核对
 * services/authz/internal/types/types.go：
 * - WriteRelationsReq{tuples:[{user,relation,object}]} → WriteRelationsResp{written}
 * - CheckReq{object,relation} → CheckResp{allowed}（无权也是 200 {allowed:false}，非错误）
 * - ListObjectsReq{type,relation} → ListObjectsResp{objects}
 *
 * 注意元组 user 字段须为 `user:{uid}` 格式（引擎约定，与 addmember/assignrole
 * logic 同源），视图层拼装。
 *
 * 写白名单（writerelationslogic.go）：仅 owner/editor/viewer/sharer 四个共享
 * 语义关系，且对象类型须已注册（保留类型 user/org/platform_role 一律拒），
 * 越权 403 AUTHZ_RELATION_FORBIDDEN——client.ts showApiError 映射「不允许写该关系」。
 *
 * 后端缺口：/v1/authz/relations 只有幂等写，没有删除元组端点——撤销能力
 * 后端缺失，展示层只提供「授权」，不做假撤销（与 Roles 分配回显同类缺口，
 * 如需要可后续加删除端点回填）。
 */
import { http } from './client'

/** 关系元组（user 须为 `user:{uid}` 格式）。 */
export interface RelationTuple {
  user: string
  relation: string
  object: string
}

/** 批量幂等写关系元组（共享授权：重复提交不报错不重复落引擎）。 */
export function writeRelations(tuples: RelationTuple[]) {
  return http.post<{ written: number }>('/authz/relations', { tuples })
}

/** 资源级权限判定：以当前登录用户（JWT claims.sub）为主语。 */
export function checkPermission(data: { object: string; relation: string }) {
  return http.post<{ allowed: boolean }>('/authz/check', data)
}

/** 列出当前用户在某类型某关系下的对象（自助盘点可见资源）。 */
export function listObjects(data: { type: string; relation: string }) {
  return http.post<{ objects: string[] }>('/authz/list-objects', data)
}
