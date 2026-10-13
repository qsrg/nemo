// 微应用入口：与 user 微应用完全同构（接入规范要求的三行结构）
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import { router } from './router'
import { defineMicroApp, setupBaseNavigation } from '@nemo/shared'

// 基座↔子应用导航桥接：刷新/直达按 props.entry 恢复页面，运行中响应 navigate 广播
setupBaseNavigation(router)

defineMicroApp(App, (app) => {
  app.use(router)
  app.use(ElementPlus)
})
