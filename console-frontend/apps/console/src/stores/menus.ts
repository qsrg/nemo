/**
 * 基座菜单 store（菜单模型简化改造：id 引用 + path 首段归组）。
 *
 * 数据源：authz GET /api/v1/me/menus —— 服务端按调用者 perms 过滤（与前端
 * hasPermission 同语义），仅下发 enabled 且被授予的行，按 (sort, id) 升序；
 * 基座渲染时再过一道 hasPermission（双保险）。
 *
 * 行语义：每行 = 一条可导航菜单项，行身份 = 自增 id、父子引用 = parentId
 *（顶级为 0）。基座按 path 首段归组到静态 SERVICES 的服务（/kafka/topics →
 * kafka；服务身份 url/title/icon 仍由静态注册表持有——服务宿主映射不落库），
 * 库内行替换静态 menus（标题/路径/排序由库管）；库内无该服务的行 = 服务不可见。
 * 拉取失败回退静态 SERVICES 并标记 fallback（菜单是导航数据而非安全边界，
 * 服务端资源级安全由 Check 把守），menus.ts 保留为回退与 seed 注释来源。
 */
import { defineStore } from 'pinia'
import { createHttp } from '@nemo/http'
import { useAuthStore } from './auth'

import { SERVICES, type MenuItem, type ServiceDef } from '../menus'

/** me/menus 响应字段（services/authz authz.api MeMenuItem 同源）。 */
interface MeMenuItem {
  /** 行 id。 */
  id: number
  /** 父菜单行 id（顶级/服务首页为 0）；按 parentId 组装多级侧边栏。 */
  parentId: number
  title: string
  icon: string
  path: string
  sort: number
  /** 权限标识（可空：空 = 全员可见，非空 = 准入门槛）。 */
  permissionCode: string
}

/** 库内行 → ServiceDef 重建：按 path 首段归组，菜单行按 parentId
 *  组装多级（服务 → 一级菜单 → 二级菜单），行集替换静态 menus。 */
function rebuildServices(rows: MeMenuItem[]): ServiceDef[] {
  const out: ServiceDef[] = []
  for (const s of SERVICES) {
    const owned = rows.filter((r) => r.path.split('/')[1] === s.key)
    if (owned.length === 0) continue // 库内无该服务的行 = 服务不可见
    owned.sort((a, b) => a.sort - b.sort || a.id - b.id)
    // 子菜单（parentId 指向本行）挂到父菜单下；服务根行（path = /{服务key}
    // 且标题与服务名相同，如 RocketMQ 根行）恒为纯结构锚点（父级链头/路由
    // 入口），不进侧边栏——避免子菜单全被过滤时根行被当作叶子菜单重复
    // 渲染（zhangsan 无子菜单权限时「RocketMQ → RocketMQ」的 bug）；仅剩
    // 根行时服务标题本身作为入口（defaultPath，点击进入服务默认页）。
    // 标题不同（如 kafka 根行「集群概览」、redis 根行「实例管理」）=
    // 服务默认页，正常渲染。其余顶级行平铺为一级菜单。
    const toItem = (r: MeMenuItem): MenuItem => ({
      title: r.title,
      path: r.path,
      icon: r.icon || undefined,
      children: owned
        .filter((c) => c.parentId === r.id)
        .sort((a, b) => a.sort - b.sort || a.id - b.id)
        .map(toItem),
    })
    // 根行判定：path=/{服务key} 且标题与服务名相同——恒为结构锚点不进侧边栏，
    // 但其直接子菜单（parentId 指向本根行）必须提升为顶级菜单，否则原 filter
    // 条件 `r.parentId === 0` 会连同子菜单一并排除，导致整个服务下菜单全部
    // 丢失（authz 下 7 个子菜单消失的 bug）。
    const rootRow = owned.find(
      (r) =>
        r.parentId === 0 &&
        r.path === `/${s.key}` &&
        r.title === s.title,
    )
    const menus: MenuItem[] = owned
      .filter((r) => r.parentId === 0 && r !== rootRow)
      .map(toItem)
    if (rootRow) {
      // 根行被过滤：其直接子菜单提升为顶级菜单（按 sort/id 升序），保留
      // toItem 递归挂载更深子菜单的能力（如有三级菜单挂到提升项下）。
      const lifted = owned
        .filter((c) => c.parentId === rootRow.id)
        .sort((a, b) => a.sort - b.sort || a.id - b.id)
        .map(toItem)
      menus.unshift(...lifted)
    }
    // 孤儿子级提升：父级行（顶级行或服务首页行）不在返回集（用户未持父级
    // 权限码但持子级码，如 zhangsan 有 kafka-topics:read 但无 kafka:read）时，
    // 子级行无法挂载（parentId≠0 被顶级过滤排除），服务块会变空壳。此处将
    // 这些孤儿行提升为服务块顶级菜单，保证"有子级权限即有入口"。
    const topParentIds = new Set(owned.filter((r) => r.parentId === 0).map((r) => r.id))
    const orphans = owned
      .filter((r) => r.parentId !== 0 && !topParentIds.has(r.parentId))
      .sort((a, b) => a.sort - b.sort || a.id - b.id)
      .map(toItem)
    menus.push(...orphans)
    // 服务下没有任何可访问菜单页（仅剩锚点根行）→ 服务整体不显示（fail-closed）。
    // 根行本身不挂页面（多为 redirect 锚点，如 NuJob /nujob 在目标微应用无对应
    // 路由），显示"服务标题即入口"是点不进的假入口；无页面可访问 = 服务不可见，
    // 与 me/menus 服务端过滤口径一致（服务端同样移除"仅剩根行"的锚点）。
    if (menus.length === 0) continue
    out.push({
      ...s,
      menus,
      // 兜底服务入口（仅当未来出现「根行挂真实页面但子菜单全被过滤」时生效；
      // 当前服务端已移除仅剩根行，menus 空即不显示）
      defaultPath: rootRow ? rootRow.path : menus[0]?.path,
    })
  }
  return out
}

export const useMenusStore = defineStore('menus', {
  state: () => ({
    /** 菜单渲染源：默认回退静态注册表（fetch 成功后被库内行重建） */
    items: SERVICES as ServiceDef[],
    /** 是否已完成一次拉取（失败也算，降级为静态回退） */
    loaded: false,
    /** 拉取失败标记（true = 当前 items 为静态回退） */
    fallback: false,
  }),
  actions: {
    /**
     * 拉取当前用户可见菜单：登录/会话恢复成功后调用（与 permission store
     * 同节奏：auth store 的三个会话建立点同步拉取）。未登录静默跳过；失败
     * 回退静态并标记 fallback（菜单是导航数据而非安全边界，资源级安全由
     * 服务端 Check 把守），menus.ts 保留为回退与 seed 注释来源。
     */
    async fetch() {
      const auth = useAuthStore()
      if (!auth.token) return
      const http = createHttp({
        baseURL: '/api/v1',
        getToken: () => auth.token,
      })
      try {
        const result = await http.get<{ items: MeMenuItem[] }>('/me/menus')
        this.items = rebuildServices(result.items ?? [])
        this.fallback = false
      } catch {
        this.items = SERVICES
        this.fallback = true
      } finally {
        this.loaded = true
      }
    },
    /** 登出/会话失效时回退静态注册表（auth.logout 调用） */
    reset() {
      this.items = SERVICES
      this.fallback = false
      this.loaded = false
    },
  },
})
