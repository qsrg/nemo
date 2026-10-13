import { createPinia } from 'pinia'
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import { bus } from 'wujie'
import WujieVue from 'wujie-vue3'

import App from './App.vue'
import { router } from './router'
import { usePermissionStore } from './stores/permission'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus)
app.use(WujieVue)

app.mount('#app')

// 微应用权限变更经 wujie bus 广播 refresh-permission（permission-design §5.2，
// frontend-design §6 预留事件）：基座重拉权限码刷新缓存。
// 微应用侧在 props 变更/下次 fetch 后收到新 perms（wujie 沙箱协议，见 packages/shared/micro.ts）
bus.$on('refresh-permission', () => usePermissionStore().fetch())
