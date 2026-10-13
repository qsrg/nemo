/**
 * useSession：微应用会话访问的统一 composable（基础能力层）。
 *
 * 收编各页面「sessionReady() 门控 + sessionProps() 取会话」样板：
 * - ready()：握手竞态门控——iframe 直连形态下 token 未到前阻塞，避免
 *   首屏请求 401 被误判为会话失效（见 micro.ts sessionReady）
 * - token / user / permissions / base / entry：响应式会话字段
 * - props：与基座 BaseProps 同构的只读视图（wujie props 或组装值）
 */
import { computed, type ComputedRef } from 'vue'
import { session, sessionProps, sessionReady, type BaseProps } from './micro'

export interface UseSession {
  /** 会话就绪门控：await 后再发首屏请求（微应用 onMounted 约定） */
  ready: () => Promise<void>
  /** 基座会话只读视图（wujie props 优先；独立运行组装；无会话为 null） */
  props: ComputedRef<BaseProps | null>
  token: ComputedRef<string>
  user: ComputedRef<typeof session.user>
  permissions: ComputedRef<string[]>
  base: ComputedRef<string>
  entry: ComputedRef<string>
}

export function useSession(): UseSession {
  return {
    ready: sessionReady,
    props: computed(() => sessionProps()),
    token: computed(() => session.token),
    user: computed(() => session.user),
    permissions: computed(() => session.permissions),
    base: computed(() => session.base),
    entry: computed(() => session.entry),
  }
}
