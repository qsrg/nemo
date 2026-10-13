import { createRouter, createMemoryHistory, createWebHashHistory } from 'vue-router'
import Layout from './views/Layout.vue'
import ServiceHome from './views/ServiceHome.vue'
import Grants from './views/Grants.vue'
import Orgs from './views/Orgs.vue'
import Shares from './views/Shares.vue'
import Menus from './views/Menus.vue'
import Audit from './views/Audit.vue'
import Attributes from './views/Attributes.vue'
import Policies from './views/Policies.vue'

// 路由约定（前端接入规范 §路由）：
// - 集成于基座：memory 模式，URL 由基座管理（navigate 事件桥接）；
//   基座一级路由段 /authz 由 setupBaseNavigation 按 props.base 剥掉，
//   子应用内部路由是去前缀后的路径（/authz/grants → /grants），与 apps/user 同约定
// - 独立运行：hash 模式，可 #/grants 直达调试
// 去角色化：默认首页 = 人员授权（Grants）；平台角色/规则表两页已删除
// （后端 roles/rules API 保留、.api 标 deprecated，界面不再暴露）。
// 资源类型页下线：类型不再独立注册，由菜单/按钮声明派生（只读列表经
// Shares/授权名单聚合展示），详见 docs/permission-design.md。
// 服务根行（权限管理）入口 = /authz：渲染 ServiceHome（该服务下子菜单导航），
// 不再 redirect 到 /grants —— 与子菜单「人员授权」不再是同一页，避免双入口。
const createHistory = () =>
  window.__POWERED_BY_WUJIE__ ? createMemoryHistory() : createWebHashHistory()

export const router = createRouter({
  history: createHistory(),
  routes: [
    { path: '/authz', redirect: '/' },
    {
      path: '/',
      component: Layout,
      children: [
        { path: '', component: ServiceHome, meta: { title: '权限管理' } },
        { path: 'grants', component: Grants, meta: { title: '人员授权' } },
        { path: 'orgs', component: Orgs, meta: { title: '部门与成员' } },
        { path: 'shares', component: Shares, meta: { title: '资源共享' } },
        { path: 'attributes', component: Attributes, meta: { title: '资源属性' } },
        { path: 'policies', component: Policies, meta: { title: 'ABAC 策略' } },
        { path: 'menus', component: Menus, meta: { title: '菜单管理' } },
        { path: 'audit', component: Audit, meta: { title: '审计' } },
      ],
    },
  ],
})
