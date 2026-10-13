import { createRouter, createMemoryHistory, createWebHashHistory } from 'vue-router'
import TopicManage from './views/TopicManage.vue'
import Clusters from './views/Clusters.vue'

// 路由约定：rocketmq 菜单宿主为本应用（独立微应用 apps/rocketmq）（base '/rocketmq'）。
// 基座集成（memory 路由）：navigate('/rocketmq/topics') 按 props.base 剥掉前缀
// 后路由 '/topics'（ServiceOverview 同款约定）；独立调试可用 '/rocketmq/topics'
// 别名直达。
//
// 根路径 '/' 不再渲染共享 ServiceOverview（静态演示数据，与生产数据不符），
// 直接重定向到「Topic 管理」——用户进入本服务即看到按集群授权的真实 Topic 列表。
// （集群概览由「集群管理」菜单进入真实 list-clusters 数据。）

// 路由约定（前端接入规范 §路由）：
// - 集成于基座：memory 模式，URL 由基座管理（navigate 事件桥接）；
//   所有服务共用 '/' 路由，服务身份由 props.base 区分（见 ServiceOverview）
// - 独立运行：hash 模式，可 #/?service=rocketmq 指定服务单独调试
const createHistory = () =>
  window.__POWERED_BY_WUJIE__ ? createMemoryHistory() : createWebHashHistory()

export const router = createRouter({
  history: createHistory(),
  routes: [
    { path: '/', redirect: '/topics' },
    { path: '/rocketmq', redirect: '/topics' },
    // rocketmq 菜单「集群管理」：集群基础配置 + 授权锚点（rmq_cluster）。
    { path: '/clusters', component: Clusters },
    { path: '/rocketmq/clusters', component: Clusters },
    // rocketmq 菜单「Topic 管理」：顶部选集群 + 该集群 Topic 列表。
    { path: '/topics', component: TopicManage },
    { path: '/rocketmq/topics', component: TopicManage },
  ],
})
