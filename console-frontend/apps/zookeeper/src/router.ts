import { createRouter, createMemoryHistory, createWebHashHistory } from 'vue-router'
import NodeBrowser from './views/NodeBrowser.vue'

// 路由约定（与 apps/redis 同款）：
// - 集成于基座：memory 模式，URL 由基座管理（navigate 事件桥接）；
//   服务身份由 props.base 区分（基座 navigate('/zookeeper') 剥掉 /zookeeper
//   前缀后路由 '/'）
// - 独立运行：hash 模式，可 #/?service=zookeeper 指定服务单独调试
const createHistory = () =>
  window.__POWERED_BY_WUJIE__ ? createMemoryHistory() : createWebHashHistory()

export const router = createRouter({
  history: createHistory(),
  routes: [
    // 根行即节点浏览页（menus.yaml 中 zookeeper 根行 path=/zookeeper，基座
    // 剥前缀后落 '/'；'/zookeeper' 为别名直达）
    { path: '/', component: NodeBrowser },
    { path: '/zookeeper', component: NodeBrowser },
    // 菜单「节点浏览」path=/zookeeper/nodes（008_menu_seed.sql id=380）：
    // 基座 iframe 直连经 entry 恢复时剥 /zookeeper 前缀后路由 /nodes，
    // 缺此路由会白屏（Vue Router No match）。'/zookeeper/nodes' 为直达兜底。
    { path: '/nodes', component: NodeBrowser },
    { path: '/zookeeper/nodes', component: NodeBrowser },
  ],
})
