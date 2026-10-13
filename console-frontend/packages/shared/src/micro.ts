/**
 * 微应用接入协议的核心实现（前端接入规范 §生命周期）。
 * 所有业务微应用的 main.ts 只需要一行 defineMicroApp(App)，
 * 集成（wujie 环境导出 mount/unmount）与独立运行（直接挂载）两种形态统一处理。
 */
import { createApp, reactive, type App as VueApp, type Component } from 'vue'
import type { User } from './types'

/** 基座通过 wujie props 注入微应用的信息 */
export interface BaseProps {
  token: string
  userInfo: User | null
  /** 当前用户权限码集合（如 ["order:read"]，permission-design §5.2），
   *  微应用用 hasPermission 判定菜单/按钮显隐；变更经 bus refresh-permission 刷新 */
  permissions?: string[]
  /** 本服务的一级路由前缀（如 '/user'），navigate 事件按它过滤 */
  base?: string
  /** 基座挂载时的入口完整路径（如 '/user/kyc'），刷新/直达时恢复子应用内页面 */
  entry?: string
}

declare global {
  interface Window {
    /** wujie 注入：标识当前运行在微前端环境 */
    __POWERED_BY_WUJIE__?: boolean
    /** wujie 注入：props（基座下发）、bus（跨应用事件） */
    $wujie?: {
      props?: BaseProps
      bus?: { $emit: (e: string, ...a: unknown[]) => void; $on: (e: string, fn: (...a: unknown[]) => void) => void }
    }
    /** wujie 约定的生命周期入口（由 defineMicroApp 挂载） */
    __WUJIE_MOUNT?: () => void
    __WUJIE_UNMOUNT?: () => void
  }
}

/**
 * 会话状态：微应用内所有 http 请求从这里取 token。
 * 来源：wujie props（沙箱集成）或基座 postMessage 下发（iframe 直连），401 时置空。
 * permissions 为基座 permission store 同步的权限码；base/entry 为本服务路由标识
 * （iframe 直连形态从 URL hash query 解析，供 sessionProps 组装 BaseProps）。
 */
export const session = reactive<{
  token: string
  user: User | null
  permissions: string[]
  base: string
  entry: string
}>({ token: '', user: null, permissions: [], base: '', entry: '' })

/**
 * 从 wujie props 或基座 postMessage 初始化会话（defineMicroApp 内部调用）。
 * - wujie 沙箱：props 直传，行为不变
 * - iframe 直连（无沙箱独立运行）：从 URL hash query 解析 service/entry，
 *   注册 message 监听接收基座下发的会话，并主动向基座请求一次
 *   （时序双向兼容：子应用先 ready 请求下发、基座也可能先推，两种顺序都能接——
 *   监听器常驻，晚到的 nemo:session 照样生效）
 */
let sessionChannelReady = false

/** 会话就绪等待器：token 已在则立即 resolve;否则等基座 postMessage 下发。
 *  iframe 直连形态下子应用首屏请求用它做门控,避免"token 未到先请求 → 401
 *  → 误报 unauthorized → 基座登出"的握手竞态。 */
const sessionWaiters: Array<() => void> = []
export function sessionReady(): Promise<void> {
  if (session.token) return Promise.resolve()
  return new Promise((resolve) => sessionWaiters.push(resolve))
}
/** 写入会话 token 并唤醒 sessionReady 等待方。
 * 供 SimpleLogin 等直接注入 token 的入口使用——绕过本函数直接赋值
 * session.token 会让等待方悬挂(sessionReady 门控的组件永不加载)。 */
export function setSessionToken(token: string) {
  session.token = token
  notifySessionWaiters()
}

function notifySessionWaiters() {
  if (!session.token) return
  while (sessionWaiters.length) sessionWaiters.shift()?.()
}


/** 校验并应用基座下发的会话消息：字段存在且类型正确才覆盖（避免脏数据污染会话） */
function applySessionMessage(data: unknown) {
  if (!data || typeof data !== 'object') return
  const msg = data as { type?: unknown; token?: unknown; user?: unknown; permissions?: unknown }
  if (msg.type !== 'nemo:session') return
  if (typeof msg.token === 'string') session.token = msg.token
  notifySessionWaiters()
  if (msg.user && typeof msg.user === 'object') session.user = msg.user as User
  if (Array.isArray(msg.permissions)) {
    session.permissions = msg.permissions.filter((p): p is string => typeof p === 'string')
  }
}

export function initSessionFromProps() {
  const w = window.$wujie
  if (w?.props) {
    session.token = w.props.token ?? ''
    session.user = w.props.userInfo ?? null
    return
  }
  // —— iframe 直连形态 ——
  if (sessionChannelReady) return
  sessionChannelReady = true
  // frameSrc 形如 `${url}#/?service=user&entry=%2Fuser%2Fkyc`：基座经 hash query
  // 下发服务标识，供 sessionProps() 组装 BaseProps（导航桥接/服务概览消费）
  const params = new URLSearchParams(window.location.hash.split('?')[1] ?? '')
  session.base = params.get('service') ?? ''
  session.entry = params.get('entry') ?? ''
  // 接收基座会话（含权限变更重推）：listener 常驻，postMessage 跨源场景
  // 无需手动清理（独立运行页面生命周期即应用生命周期）
  window.addEventListener('message', (e) => applySessionMessage(e.data))
  // 主动请求基座下发会话。targetOrigin 用 '*' 的前提：当前仅 dev/内网联调
  // （基座与子应用同域部署时无跨源风险）；生产若跨域，应改为基座 URL 的
  // 白名单 origin，避免会话 token 被恶意父页截获
  window.parent.postMessage({ type: 'nemo:session-ready' }, '*')
}

/** 非 wujie 形态回退：session 已持有会话时组装出与 BaseProps 同构的对象（无则 null） */
export function sessionProps(): BaseProps | null {
  if (window.$wujie?.props) return window.$wujie.props
  if (!session.token) return null
  return {
    token: session.token,
    userInfo: session.user,
    permissions: session.permissions,
    base: session.base,
    entry: session.entry,
  }
}

/**
 * 通知基座跨应用事件（401 登出等，架构 §8.4）。
 * wujie 环境走 bus；iframe 直连降级为 postMessage（事件名统一 nemo: 前缀，
 * 基座 MicroAppFrame 监听，如 unauthorized → 基座统一登出）。
 */
export function emitToBase(event: string, ...args: unknown[]) {
  if (window.$wujie?.bus) {
    window.$wujie.bus.$emit(event, ...args)
    return
  }
  window.parent.postMessage({ type: 'nemo:' + event, args }, '*')
}

/**
 * 基座↔子应用导航桥接（前端接入规范 §6 navigate 事件）。
 * memory 路由的子应用 URL 由基座管理：基座侧边栏点击 → 基座 router.push →
 * bus 广播 navigate → 本服务路由跳转；刷新/直达时由 props.entry 恢复子应用内页面。
 * 各微应用 main.ts 在 defineMicroApp 前调用一次：setupBaseNavigation(router)
 */
export function setupBaseNavigation(router: {
  push: (p: string) => void
  replace: (p: string) => void
}) {
  // 导航响应:按基座路径切换子应用内部路由(只认本服务前缀)。
  // 必须等 router.isReady():setupBaseNavigation 在 main.ts 里先于 app.use(router)
  // 执行,未就绪时 replace/push 会被初始化导航(如 '/' 重定向到默认页)覆盖——
  // 表现为"entry 恢复/页签导航失效,永远落在默认页"。
  const follow = (base: string, raw: unknown, replace: boolean) => {
    if (typeof raw !== 'string' || !raw.startsWith(base)) return
    const sub = raw.slice(base.length)
    const target = sub === '' ? '/' : sub
    // isReady:安装完成前注册会静默悬挂,故 follow 仅应在导航确认后触发
    // (afterEach 语义)或运行时(已就绪)调用
    router.isReady().then(() => (replace ? router.replace(target) : router.push(target)))
  }

  // 初始 entry 恢复：本函数在 main.ts 先于 app.use(router) 执行。isReady/
  // setTimeout 的时序都会被"初始化导航(含默认页重定向)异步完成后才落定"覆盖
  // ——恢复必须挂在 afterEach(导航确认后必触发)上执行一次;独立运行兜底 setTimeout。
  let restored = false
  const restoreEntry = (base: string, entry: string) => {
    if (restored || !entry.startsWith(base)) return
    restored = true
    follow(base, entry, true)
  }

  if (window.__POWERED_BY_WUJIE__) {
    const w = window.$wujie
    const base = w?.props?.base ?? ''
    // 直达/刷新：按基座 URL 恢复子应用内页面
    const wEntry = w?.props?.entry ?? ''
    router.afterEach(() => restoreEntry(base, wEntry))
    setTimeout(() => restoreEntry(base, wEntry), 0)
    // 运行中导航：只响应本服务前缀的 navigate 广播
    w?.bus?.$on('navigate', (path: unknown) => follow(base, path, false))
    return
  }

  // iframe 直连形态：base/entry 从 URL hash query 解析（frameSrc 携带），
  // 页签切换导航由基座 postMessage nemo:navigate 下发（MicroAppFrame 桥接）
  const q = new URLSearchParams(location.hash.split('?')[1] ?? '')
  const base = q.get('service') ? '/' + q.get('service') : ''
  const iEntry = decodeURIComponent(q.get('entry') ?? '')
  router.afterEach(() => restoreEntry(base, iEntry))
  setTimeout(() => restoreEntry(base, iEntry), 0)
  window.addEventListener('message', (e: MessageEvent) => {
    const data = e.data as { type?: unknown; path?: unknown } | null
    if (!data || typeof data !== 'object' || data.type !== 'nemo:navigate') return
    follow(base, data.path, false)
  })
}

/**
 * 微应用生命周期标准接线：
 * - wujie 环境：向 window 挂 __WUJIE_MOUNT / __WUJIE_UNMOUNT（wujie 约定入口），挂载到 #app
 * - 独立/iframe 直连运行：直接挂载，可脱离基座单独调试
 * install 用于注册路由等插件（每个微应用的 main.ts 保持同样的三行结构）。
 * 沙箱/iframe 内的执行错误对用户不可见，这里兜底渲染到页面上便于排查。
 */
function showFatal(message: unknown) {
  try {
    const pre = document.createElement('pre')
    pre.style.cssText = 'margin:0;padding:16px;color:var(--el-color-danger,#f56c6c);font-size:12px;white-space:pre-wrap'
    pre.textContent = `[微应用执行错误] ${message instanceof Error ? (message.stack ?? message.message) : String(message)}`
    document.body.appendChild(pre)
  } catch { /* 渲染失败时静默，避免二次异常 */ }
}

export function defineMicroApp(root: Component, install?: (app: VueApp) => void) {
  let app: VueApp | null = null

  function render() {
    try {
      initSessionFromProps()
      app = createApp(root)
      install?.(app)
      app.config.errorHandler = (err) => showFatal(err)
      app.mount('#app')
    } catch (err) {
      showFatal(err)
    }
  }

  if (window.__POWERED_BY_WUJIE__) {
    window.addEventListener('error', (e) => showFatal(`脚本错误：${e.message}`))
    window.addEventListener('unhandledrejection', (e) => showFatal(`未处理的 Promise 拒绝：${e.reason}`))
    window.__WUJIE_MOUNT = render
    window.__WUJIE_UNMOUNT = () => {
      app?.unmount()
      app = null
    }
  } else {
    render()
  }
}
