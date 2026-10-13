/**
 * 基座权限 store（permission-design §5.2）：
 * 登录/会话恢复后拉取 authz `GET /api/v1/me/permissions` 并缓存，
 * 微应用经 props 消费 perms；权限变更经 wujie bus `refresh-permission`
 * 事件广播刷新（基座在 main.ts 监听）。
 */
import { defineStore } from 'pinia'
import { createHttp } from '@nemo/http'
import { useAuthStore } from './auth'

interface MePermissionsResp {
  perms: string[]
}

export const usePermissionStore = defineStore('permission', {
  state: () => ({
    /** 权限码集合（如 ["order:read","user:manage"]），登出/未登录/拉取失败为空 */
    perms: [] as string[],
    /** 是否已完成一次拉取（失败也算——降级为无权限） */
    loaded: false,
  }),
  actions: {
    /** 拉取当前用户权限码；未登录静默跳过，失败静默降级 perms=[]（fail-closed：不可见即隐藏） */
    async fetch() {
      const auth = useAuthStore()
      if (!auth.token) return
      const http = createHttp({
        baseURL: '/api/v1',
        getToken: () => auth.token,
        // 401 不经此链路登出/跳转（基座统一由 auth store 的 /me 恢复流程处理）；
        // 权限拉取失败一律静默降级为无权限
      })
      try {
        const result = await http.get<MePermissionsResp>('/me/permissions')
        this.perms = result.perms ?? []
      } catch {
        this.perms = []
      } finally {
        this.loaded = true
      }
    },
    /** 登出/会话失效时清空（auth.logout 调用） */
    reset() {
      this.perms = []
      this.loaded = false
    },
  },
})
