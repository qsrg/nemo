/** 基座统一登录：token 只存在于基座（架构 §8.2），微应用经 props 接收。 */
import { defineStore } from 'pinia'
import { createHttp } from '@nemo/http'
import type { LoginResult } from '@nemo/shared'
import { usePermissionStore } from './permission'
import { useMenusStore } from './menus'

const TOKEN_KEY = 'nemo_token'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) ?? '',
    user: null as LoginResult['user'] | null,
  }),
  actions: {
    /** 口令登录（SimpleLogin 独立调试用）。注意：口令端点 POST /auth/login 已下线，
     *  基座登录统一走 CAS 流程（acceptToken）；本 action 保留仅作调试通道。 */
    async login(username: string, password: string) {
      // onUnauthorized 在登录接口上不应触发登出循环，这里独立创建实例
      const http = createHttp({ baseURL: '/api/v1' })
      const result = await http.post<LoginResult>('/auth/login', { username, password })
      this.token = result.token
      this.user = result.user
      localStorage.setItem(TOKEN_KEY, result.token)
      // 登录成功即拉取权限码与动态菜单缓存（permission-design §5.2 + Phase C 改造①）
      await usePermissionStore().fetch()
      await useMenusStore().fetch()
    },
    /**
     * 接收 CAS 回跳的一次性票据并建立完整会话（基座登录主通道）：
     * POST /auth/cas/callback 以 ticket 换 token → 设 token/localStorage →
     * 拉 /me 恢复用户 → 拉权限码缓存。任一步失败（票据无效/服务不可用）
     * 即登出并抛错，由调用方提示回登录页。
     */
    async acceptToken(ticket: string) {
      try {
        // 票据兑换：独立实例（换票时无 token，401 不应触发登出循环）
        const exchange = createHttp({ baseURL: '/api/v1' })
        const resp = await exchange.post<{ token: string; expiresAt?: string }>(
          '/auth/cas/callback',
          // service 须与 Login.vue 发起跳转时一致（后端校验 service 匹配）
          { ticket, service: window.location.origin + '/cas-ticket' },
        )
        this.token = resp.token
        localStorage.setItem(TOKEN_KEY, resp.token)
        // 会话恢复：/me 上 401 不应触发登出循环，独立创建实例
        const http = createHttp({
          baseURL: '/api/v1',
          getToken: () => this.token,
          onUnauthorized: () => this.logout(),
        })
        this.user = await http.get<LoginResult['user']>('/me')
        // 会话建立成功后拉取权限码与动态菜单缓存（permission-design §5.2 + Phase C 改造①）
        await usePermissionStore().fetch()
        await useMenusStore().fetch()
      } catch (e) {
        this.logout()
        throw e
      }
    },
    // 页面刷新后 token 在 localStorage、user 为空：拉一次 /me 恢复
    async restoreUser() {
      if (!this.token || this.user) return
      const http = createHttp({
        baseURL: '/api/v1',
        getToken: () => this.token,
        onUnauthorized: () => this.logout(),
      })
      this.user = await http.get<LoginResult['user']>('/me')
      // 会话恢复成功（刷新场景）后同步拉取权限码与动态菜单
      await usePermissionStore().fetch()
      await useMenusStore().fetch()
    },
    logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem(TOKEN_KEY)
      // 权限码/动态菜单随会话失效清空（菜单回退静态注册表）
      usePermissionStore().reset()
      useMenusStore().reset()
    },
  },
})
