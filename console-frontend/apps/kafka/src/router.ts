import { createRouter, createMemoryHistory, createWebHashHistory } from 'vue-router'
import { ServiceOverview } from '@nemo/shared'
import TopicManage from './views/TopicManage.vue'
import ClusterManage from './views/Clusters.vue'

// 路由约定（Phase C 改造③④）：kafka 菜单宿主为本应用（独立微应用 apps/kafka）（base '/kafka'）。
// 基座集成（memory 路由）：navigate('/kafka/topics') 按 props.base 剥掉前缀
// 后路由 '/topics'（ServiceOverview 同款约定）；独立调试可用 '/kafka/topics'
// 别名直达。

// 路由约定（前端接入规范 §路由）：
// - 集成于基座：memory 模式，URL 由基座管理（navigate 事件桥接）；
//   所有服务共用 '/' 路由，服务身份由 props.base 区分（见 ServiceOverview）
// - 独立运行：hash 模式，可 #/?service=redis 指定服务单独调试
const createHistory = () =>
  window.__POWERED_BY_WUJIE__ ? createMemoryHistory() : createWebHashHistory()

export const router = createRouter({
  history: createHistory(),
  routes: [
    { path: '/', component: ServiceOverview },
    // kafka 菜单第一项「集群概览」：服务首页（概览/子菜单导航）。
    // 菜单 path 为 /kafka/overview（服务根行 /kafka 为纯结构锚点）。
    { path: '/overview', component: ServiceOverview },
    { path: '/kafka/overview', component: ServiceOverview },
    // kafka 菜单第二项「Topic 管理」：ABAC 实测页（消息域演示页，
    // 真实环境由独立 Kafka 管理服务承载）
    { path: '/topics', component: TopicManage },
    { path: '/kafka/topics', component: TopicManage },
    // kafka 菜单第三项「集群管理」：集群基础信息维护页（机房/编码/地址/备注）
    { path: '/clusters', component: ClusterManage },
    { path: '/kafka/clusters', component: ClusterManage },
  ],
})
