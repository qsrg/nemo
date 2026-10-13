/**
 * 审计查询 API 包装（Task19/B11）：
 * 消费 authz 管理 API GET /v1/admin/audit，字段核对
 * services/authz/internal/types/types.go（AuditResp{id,operator,action,target,
 * snapshot,status,createdAt}、AuditListResp{items,total}）。
 * 统一经 client.ts 的 http 实例（baseURL /api/v1，token 实时取基座 props）。
 *
 * 契约核对（listauditlogic.go + ListAuditReq）：后端仅支持三个 query 参数——
 *   page / pageSize（repo 层规范化：Page<1→1、PageSize<1→20，按 CreatedAt,ID 倒序）
 *   userId（按操作人过滤；简报写的 operator= 实际不存在）
 * 操作类型/时间范围后端不过滤——页面下拉与日期范围为当前页内客户端过滤（见 Audit.vue）。
 */
import { http } from './client'

/** 审计条目（authz AuditResp）。createdAt 为 RFC3339 文本；snapshot 为 JSON 文本列。 */
export interface AuditEntry {
  id: string
  operator: string
  action: string
  target: string
  snapshot: string
  /** pending / committed / failed */
  status: string
  createdAt: string
}

/** 后端实际会产生过的 action 集（核对 internal/logic/*writeWithAudit 调用点）。 */
export const AUDIT_ACTIONS = [
  'create_org',
  'update_org',
  'delete_org',
  'add_member',
  'remove_member',
  'assign_role',
  'revoke_role',
  'grant_permission',
  'revoke_permission',
  'sync_orgs',
  'upsert_rule',
  'delete_rule',
  'register_resource_type',
  'write_relations',
  'create_topic',
  'grant_topic',
  'revoke_topic',
  'create_menu',
  'delete_menu',
  'seed_menus',
  'migrate_rules',
] as const

/** 动作 → 可读名（表格与过滤下拉共用；未收录动作原样展示）。 */
export const AUDIT_ACTION_LABELS: Record<string, string> = {
  create_org: '创建组织',
  update_org: '修改组织',
  delete_org: '删除组织',
  add_member: '添加成员',
  remove_member: '移除成员',
  assign_role: '授予角色',
  revoke_role: '回收角色',
  grant_permission: '授予权限',
  revoke_permission: '回收权限',
  sync_orgs: 'CAS 组织同步',
  upsert_rule: '保存规则',
  delete_rule: '删除规则',
  register_resource_type: '注册资源类型',
  write_relations: '写共享关系',
  create_topic: '创建 Topic',
  grant_topic: '授予 Topic',
  revoke_topic: '回收 Topic',
  create_menu: '创建菜单',
  delete_menu: '删除菜单',
  seed_menus: '初始化默认菜单',
  migrate_rules: '规则迁移',
}

export interface ListAuditParams {
  page?: number
  pageSize?: number
  /** 操作人过滤（后端参数名为 userId） */
  userId?: number | string
  /** 排除系统操作人（system）条目：定时任务审计默认隐藏 */
  excludeSystem?: boolean
}

/** 分页查询审计条目（管理面）。 */
export function listAudit(params: ListAuditParams = {}) {
  const qs = new URLSearchParams()
  if (params.page && params.page > 1) qs.set('page', String(params.page))
  if (params.pageSize && params.pageSize !== 20) qs.set('pageSize', String(params.pageSize))
  if (params.userId) qs.set('userId', String(params.userId))
  if (params.excludeSystem) qs.set('excludeSystem', 'true')
  const query = qs.toString()
  return http.get<{ items: AuditEntry[]; total: number }>(`/admin/audit${query ? `?${query}` : ''}`)
}
