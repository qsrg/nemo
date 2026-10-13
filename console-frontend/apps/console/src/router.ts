import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { useMenusStore } from './stores/menus'

// 基座路由由菜单注册表驱动：新增服务 = SERVICES 加一条记录，零代码扩展。
// 服务路由只承担 URL 语义（直达/刷新/守卫），微应用页面由 Layout 的
// 常驻容器渲染（页签多开模式，同时打开的微应用全部保持运行）。
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('./views/Login.vue') },
    // CAS 接票页：CAS 认证成功后 GET 回跳到这里（?ticket=ST-xxx 一次性票据），
    // 与 /login 同属公开路由（无 token 也放行），换 token 成功后进基座
    { path: '/cas-ticket', component: () => import('./views/CasTicket.vue') },
    {
      path: '/',
      component: () => import('./views/Layout.vue'),
      redirect: '/workbench',
      children: [
        { path: 'workbench', component: () => import('./views/Workbench.vue') },
        // 系统设置 / 告警中心 / 域态势：基座本地页面（不经过微应用）
        { path: 'settings', component: () => import('./views/Settings.vue') },
        { path: 'alerts', component: () => import('./views/AlertsCenter.vue') },
        { path: 'domain/:domain', component: () => import('./views/DomainStatus.vue') },
        { path: ':serviceKey/:pathMatch(.*)*', component: { render: () => null } },
      ],
    },
  ],
})

// 基座本地页面的一级路由段：不归属任何微服务，不受服务可见性守卫约束。
const LOCAL_BASE_PATHS = new Set(['workbench', 'settings', 'alerts', 'domain'])

// 登录态守卫：无 token 一律回登录页（/login 与 /cas-ticket 接票页例外）。
// 服务可见性守卫：菜单按 me/menus 服务端过滤，隐藏的服务直达 URL 也不放行
//（前端 fail-closed 显隐——与左侧菜单/首页态势同一套可见性；资源级安全仍
// 由服务端 Check 把守，此处只是导航防线）。刷新直达时菜单尚未拉取则先等
// 拉取完成再判定；拉取失败回退静态全量（与菜单降级口径一致，放行）。
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.path !== '/login' && to.path !== '/cas-ticket' && !auth.token) return '/login'
  if (to.path === '/login' && auth.token) return '/'

  // 公开路由（登录/接票页）不判定服务可见性，避免与登录守卫互踢。
  if (to.path === '/login' || to.path === '/cas-ticket') return

  const menus = useMenusStore()
  if (!menus.loaded) await menus.fetch()
  const seg = to.path.slice(1).split('/')[0]
  if (!seg || LOCAL_BASE_PATHS.has(seg)) return
  if (!menus.items.some((s) => s.key === seg)) return '/workbench'
})
