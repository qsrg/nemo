/**
 * usePermission：微应用按钮/页面级权限判定的统一 composable（基础能力层）。
 *
 * 收编各微应用里手写的「sessionProps()?.permissions + hasPermission」样板：
 * permissions 响应式跟随基座 session（wujie props / iframe postMessage 重推），
 * 码变更时 computed 自动刷新，页面无需自己管理权限状态。
 *
 * 码形制（permission-design §5.2 两段式）：`{服务}:{动作}`，如
 * kafka-topics:read / kafka-topics:manage。业务页按需判定：
 * - canRead(service)   → 页面只读可见（菜单 read 码）
 * - canManage(service) → 写操作显隐（菜单 manage 码）
 * - can(service, action) / has(code) → 任意精确码
 */
import { computed, type ComputedRef } from 'vue'
import { hasPermission } from './permission'
import { sessionProps } from './micro'

export interface UsePermission {
  /** 当前用户权限码集合（实时跟随基座会话） */
  permissions: ComputedRef<string[]>
  /** 精确码判定：has('kafka-topics:manage') */
  has: (code: string) => boolean
  /** 拼服务:动作判定：can('kafka-topics', 'manage') */
  can: (service: string, action: string) => boolean
  /** 服务级只读码判定：canRead('kafka-topics') ≡ kafka-topics:read */
  canRead: (service: string) => boolean
  /** 服务级管理码判定：canManage('kafka-topics') ≡ kafka-topics:manage */
  canManage: (service: string) => boolean
}

export function usePermission(): UsePermission {
  const permissions = computed(() => sessionProps()?.permissions ?? [])
  return {
    permissions,
    has: (code) => hasPermission(code, permissions.value),
    can: (service, action) => hasPermission(`${service}:${action}`, permissions.value),
    canRead: (service) => hasPermission(`${service}:read`, permissions.value),
    canManage: (service) => hasPermission(`${service}:manage`, permissions.value),
  }
}
