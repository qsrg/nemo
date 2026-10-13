/**
 * 用户目录 API 包装（B 阶段两改造 2.6）：消费 auth 服务用户目录
 * GET /v1/auth/users?ids=（裸用户名批量反查，未命中静默跳过），供裸 userId
 * 渲染姓名（查不到回退裸 ID）。统一经 client.ts 的 http 实例（token 实时取
 * 基座 props / shared session 回退）。
 *
 * 内存缓存：Map 随会话生命周期，命中即不再请求；查不到的裸 ID 不缓存负结果
 *（CAS 首登自动建档语义保证：新用户登录后同会话内反查即可见）。
 */
import { http } from './client'

/** 目录用户（auth DirectoryUserResp）。 */
export interface DirectoryUser {
  userId: string
  name: string
  email: string
  /** 所属组织（CAS 目录组织架构信息；无归属为空）。 */
  orgId?: string
  /** 组织展示名（与 orgId 同源）。 */
  orgName?: string
}

/** 会话级内存缓存：userId → 展示名。 */
const cache = new Map<string, string>()

/**
 * 带缓存的姓名反查：ids → {userId: name}（查不到的条目回退裸 ID）。
 * 未缓存的 id 一次批量请求拉取；反查失败静默降级——姓名仅展示增强，
 * 不阻断页面渲染。
 */
export async function fetchUserNames(ids: string[]): Promise<Record<string, string>> {
  const result: Record<string, string> = {}
  const pending = [...new Set(ids.filter((s) => !!s))].filter((id) => !cache.has(id))
  if (pending.length > 0) {
    try {
      // @nemo/http 的 get 只收 path：查询串拼进 path（ids 逗号分隔，encodeURIComponent 防特殊字符）。
      const resp = await http.get<{ items: DirectoryUser[]; total: number }>(
        `/auth/users?ids=${encodeURIComponent(pending.join(','))}`,
      )
      for (const u of resp.items) {
        cache.set(u.userId, u.name)
        result[u.userId] = u.name
      }
      // 未命中的 id 用裸 ID 兜底（下循环统一处理，此处不缓存负结果）。
    } catch {
      // 静默降级，交由下循环回退裸 ID。
    }
  }
  for (const id of ids) {
    if (id && !result[id]) {
      result[id] = cache.get(id) ?? id
    }
  }
  return result
}

/**
 * 组织目录聚合：分页拉取全量用户目录，把每条用户携带的 orgId→orgName 归并
 * 成组织名映射（组织名称的权威展示源是 CAS 目录组织架构信息，与 authz
 * 组织注册表/引擎回退 id 无关）。供部门树/成员抽屉展示可读组织名。
 * 失败向上抛由调用方静默降级（组织名仅展示增强，不阻断页面渲染）。
 */
export async function fetchOrgNames(): Promise<Record<string, string>> {
  const orgNames: Record<string, string> = {}
  const pageSize = 100
  for (let page = 1; ; page++) {
    const resp = await http.get<{ items: DirectoryUser[]; total: number }>(
      `/auth/users?page=${page}&pageSize=${pageSize}`,
    )
    for (const u of resp.items) {
      if (u.orgId && u.orgName && !orgNames[u.orgId]) {
        orgNames[u.orgId] = u.orgName
      }
    }
    if (resp.items.length === 0 || page * pageSize >= resp.total) break
  }
  return orgNames
}
