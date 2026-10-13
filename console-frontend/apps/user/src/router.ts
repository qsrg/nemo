import { createRouter, createMemoryHistory, createWebHashHistory } from 'vue-router'
import { ServiceOverview } from '@nemo/shared'

// 路由约定（前端接入规范 §路由）：
// - 集成于基座：memory 模式，URL 由基座管理（navigate 事件桥接）；
//   所有服务共用 '/' 路由，服务身份由 props.base 区分（见 ServiceOverview）
// - 独立运行：hash 模式，可 #/?service=redis 指定服务单独调试
const createHistory = () =>
  window.__POWERED_BY_WUJIE__ ? createMemoryHistory() : createWebHashHistory()

export const router = createRouter({
  history: createHistory(),
  routes: [{ path: '/', component: ServiceOverview }],
})
