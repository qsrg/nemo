/**
 * createMicroHttp：微应用统一 http 工厂（基础能力层）。
 *
 * 收编各微应用自写的 fetch 封装 / createHttp 样板（order 的 http.ts、
 * authz-admin 的 client.ts http 实例）：
 * - token：每次请求实时从 wujie props 或 shared session 读取（续期无需重建）
 * - 401：区分「握手竞态（发出时无 token，静默）」与「真过期（上报基座统一
 *   登出）」，语义见 packages/http HttpOptions.onUnauthorized
 * - extraHeaders：全局自定义头（order 历史需要 X-User-Level 透传的场景）
 * - 请求前置门控（gate）：await sessionReady 后再取 token 发请求——动态挂载
 *   组件（弹窗/抽屉等）可能先于基座会话下发发起请求，若无门控会发出缺 token
 *   的 401 请求；超时保险避免无基座独立调试时永久悬挂（超时放行，无 token
 *   请求走 401 静默路径，不误报登出）
 *
 * 返回实例契约与 @nemo/http 一致（get/post/put/delete + ApiError 错误体）。
 */
import { createHttp } from '@nemo/http'
import { emitToBase, session, sessionReady } from './micro'

export interface MicroHttpOptions {
  baseURL?: string
  /** 每个请求都携带的默认头（如自定义业务头），被 @nemo/http 注入的 Authorization 等覆盖规则见其实现 */
  extraHeaders?: Record<string, string>
}

/** 当前 token：基座 props 优先（每次读取），独立运行回退 shared session */
export function currentToken(): string {
  return window.$wujie?.props?.token || session.token || ''
}

/** 会话就绪门控（带超时保险）：token 已到立即放行；未到等基座下发，
 *  超时（8s）也放行——独立调试无基座时请求发出后走 401 静默路径。 */
const GATE_TIMEOUT_MS = 8000

function sessionGate(): Promise<void> {
  if (session.token) return Promise.resolve()
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, GATE_TIMEOUT_MS)
    void sessionReady().then(() => {
      clearTimeout(timer)
      resolve()
    })
  })
}

export function createMicroHttp(options: MicroHttpOptions = {}) {
  return createHttp({
    baseURL: options.baseURL ?? '/api/v1',
    getToken: () => currentToken() || undefined,
    extraHeaders: options.extraHeaders,
    // 前置门控：请求发出前等会话就绪，杜绝动态组件（弹窗等）的无 token 401
    gate: () => sessionGate(),
    onUnauthorized: (ctx) => {
      // 区分两种 401（判定依据是「请求发出时是否携带 token」而非处理时刻的
      // session.token，避免握手竞态被误判为会话失效而登出整站）：
      // - 带 token 仍 401 → 真过期 → 上报基座统一登出（wujie bus / iframe
      //   postMessage nemo:unauthorized）
      // - 无 token → iframe 握手竞态 → 静默，会话到达后门控请求自动重发
      if (window.__POWERED_BY_WUJIE__ || ctx?.hadToken) {
        emitToBase('unauthorized')
      }
    },
  })
}
