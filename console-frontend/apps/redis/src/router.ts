import { createRouter, createMemoryHistory, createWebHashHistory } from 'vue-router'
import InstanceManage from './views/InstanceManage.vue'

// 路由约定（与 apps/kafka 同款）：
// - 集成于基座：memory 模式，URL 由基座管理（navigate 事件桥接）；
//   服务身份由 props.base 区分（基座 navigate('/redis/instances') 剥掉
//   /redis 前缀后路由 '/instances'）
// - 独立运行：hash 模式，可 #/?service=redis 指定服务单独调试
const createHistory = () =>
  window.__POWERED_BY_WUJIE__ ? createMemoryHistory() : createWebHashHistory()

export const router = createRouter({
  history: createHistory(),
  routes: [
    // 根行即实例管理页（menus.yaml 中 redis 根行 path=/redis，基座剥前缀
    // 后落 '/'；'/instances' 与 '/redis/instances' 为别名直达）
    { path: '/', component: InstanceManage },
    { path: '/instances', component: InstanceManage },
    { path: '/redis/instances', component: InstanceManage },
  ],
})
