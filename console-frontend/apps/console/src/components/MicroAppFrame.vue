<template>
  <iframe
    ref="frameRef"
    class="micro-frame"
    :src="frameSrc"
    frameborder="0"
    :title="`${appName} 微应用`"
    @load="onFrameLoad"
  />
</template>

<script setup lang="ts">
/**
 * 微应用承载容器（原生 iframe 直连，兼容模式）。
 *
 * 背景：新版 Chrome 将 wujie 的 blob 空页 iframe 隔离为 opaque origin
 * （contentDocument = null、跨源访问被拒），沙箱脚本无法注入，
 * 微应用在沙箱模式下永久 loading。故降级为无沙箱的直连集成：
 * 子应用以独立形态运行（hash 路由），服务标识经 URL query 下发
 * （ServiceOverview 据此渲染）。
 *
 * 会话通道（postMessage 握手，决策 2）：
 * - 子应用 init 时 postMessage `nemo:session-ready` 请求会话
 * - 本组件回 `nemo:session`（token/user/permissions，targetOrigin 限定子应用 origin）；
 *   iframe load 时也主动推一次（双保险，防 ready 通知早于本组件监听注册）
 * - 权限变更重推（实时下发）；子应用 401 时发 `nemo:unauthorized` → 基座统一登出
 */
import { onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { usePermissionStore } from '../stores/permission'

const props = defineProps<{ appName: string; url: string }>()
const emit = defineEmits<{ appMounted: [key: string] }>()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const perm = usePermissionStore()
const frameRef = ref<HTMLIFrameElement>()

// 入口路径在挂载时刻固定：iframe src 不随基座路由变化而重载
// （页签切换仅 v-show 显隐，子应用内容与服务一一对应，无需跟随基座导航）
const entry = encodeURIComponent(route.path)
const frameSrc = `${props.url}#/?service=${props.appName}&entry=${entry}`

/** 子应用 origin：postMessage targetOrigin 限定，防会话 token 泄露到其他父页 */
function frameOrigin(): string {
  return new URL(props.url, window.location.href).origin
}

/** 向子应用下发当前会话（token/user/permissions） */
function pushSession() {
  const win = frameRef.value?.contentWindow
  if (!win) return
  // JSON 往返转纯对象:pinia 的 reactive 代理无法被 postMessage 结构化克隆
  // (DataCloneError),必须先拍平成 plain object
  const payload = JSON.parse(
    JSON.stringify({
      type: 'nemo:session',
      token: auth.token,
      user: auth.user,
      permissions: perm.perms,
    }),
  )
  win.postMessage(payload, frameOrigin())
}

function onFrameLoad() {
  emit('appMounted', props.appName)
  // 双保险：即使子应用 ready 通知早于本组件监听注册/已被错过，load 后再推一次
  pushSession()
}

function onMessage(e: MessageEvent) {
  const data = e.data as { type?: unknown } | null
  if (!data || typeof data !== 'object') return
  if (data.type === 'nemo:session-ready') {
    // 子应用就绪请求会话：立即下发当前会话
    pushSession()
  } else if (data.type === 'nemo:unauthorized') {
    // 微应用 401：基座统一登出（与 wujie bus unauthorized 语义一致）
    auth.logout()
    router.push('/login')
  }
}

window.addEventListener('message', onMessage)
onUnmounted(() => {
  window.removeEventListener('message', onMessage)
})

// 权限变更实时下发：权限码刷新后重推给所有在挂子应用
watch(() => perm.perms, pushSession)

// 页签切换导航桥接：基座路由变化 → nemo:navigate 下发（子应用
// setupBaseNavigation 的 iframe 分支响应）。外层服务判断由 Layout 的
// v-show/canSee 保证，此处只管把本服务相关的路径转发进去
watch(() => route.path, (p) => {
  const win = frameRef.value?.contentWindow
  if (!win) return
  win.postMessage({ type: 'nemo:navigate', path: p }, frameOrigin())
})
</script>

<style scoped>
.micro-frame {
  width: 100%;
  height: 100%;
  display: block;
  background: var(--el-bg-color);
}
</style>
