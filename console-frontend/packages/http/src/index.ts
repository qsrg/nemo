/**
 * @nemo/http 统一请求封装（架构 §8.6）：
 * - baseURL 走 /api/v1/*，由网关（生产）或 vite dev proxy（开发）按 path 路由
 * - 错误体解析对齐 kit errors 约定：{"error":{"code","message","request_id"}}
 * - 自动注入 Authorization: Bearer <token> 与 X-Request-ID（与后端日志 trace_id 贯通）
 */
export class ApiError extends Error {
  code: string
  requestId: string
  status: number

  constructor(code: string, message: string, requestId: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.requestId = requestId
    this.status = status
  }
}

export interface HttpOptions {
  baseURL?: string
  /** 返回当前 token；为空时不注入 Authorization 头 */
  getToken?: () => string | undefined | null
  /** 每个请求都携带的默认头（如 X-User-Level 等自定义头）；被请求级同名头覆盖 */
  extraHeaders?: Record<string, string>
  /** 请求发出前的异步门控（await 后再取 token 并发起请求）：
   *  微应用 iframe 直连形态下基座会话（token）经 postMessage 异步下发，动态
   *  挂载的组件（弹窗等）可能先于会话就绪发起请求 → 无 token 401。调用方传
   *  入会话就绪等待器（如 sessionReady），保证任何组件（含 onMounted 即加载的
   *  常驻弹窗）都不会发出缺 token 的请求；不传则立即放行（行为不变）。 */
  gate?: () => Promise<void>
  /** 收到 401 时回调（基座：登出并跳登录页；微应用：经 wujie bus 通知基座）。
   *  ctx.hadToken 表示本次请求发出时是否携带了 token——微应用 iframe 直连形态下
   *  用它区分「握手竞态（发出时无 token，会话尚未下发）」与「真过期（带 token 仍 401）」，
   *  避免握手竞态被误判为会话失效而把整站登出。 */
  onUnauthorized?: (ctx?: { hadToken: boolean }) => void
}

interface ErrorBody {
  error?: { code?: string; message?: string; request_id?: string }
}

function newRequestId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID().replace(/-/g, '')
  }
  return Math.random().toString(36).slice(2) + Date.now().toString(36)
}

export function createHttp(options: HttpOptions = {}) {
  const baseURL = options.baseURL ?? '/api/v1'

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    // 前置门控：先等会话就绪再取 token（动态组件不再发缺 token 的请求）
    await options.gate?.()
    const headers: Record<string, string> = { Accept: 'application/json', ...options.extraHeaders }
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    const token = options.getToken?.()
    if (token) headers['Authorization'] = `Bearer ${token}`
    headers['X-Request-ID'] = newRequestId()

    const resp = await fetch(baseURL + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })

    const text = await resp.text()
    let data: unknown = undefined
    if (text) {
      try {
        data = JSON.parse(text)
      } catch {
        /* 非 JSON 响应按原文处理 */
      }
    }

    if (!resp.ok) {
      if (resp.status === 401) {
        // 携带本次请求发出时的 token 状态：竞态判定依据（见 HttpOptions.onUnauthorized）
        options.onUnauthorized?.({ hadToken: !!token })
      }
      const err = (data as ErrorBody | undefined)?.error
      throw new ApiError(
        err?.code ?? 'HTTP_ERROR',
        err?.message ?? `请求失败（HTTP ${resp.status}）`,
        err?.request_id ?? '',
        resp.status,
      )
    }
    return data as T
  }

  return {
    get<T>(path: string) {
      return request<T>('GET', path)
    },
    post<T>(path: string, body?: unknown) {
      return request<T>('POST', path, body)
    },
    put<T>(path: string, body?: unknown) {
      return request<T>('PUT', path, body)
    },
    delete<T>(path: string) {
      return request<T>('DELETE', path)
    },
  }
}

export type Http = ReturnType<typeof createHttp>
