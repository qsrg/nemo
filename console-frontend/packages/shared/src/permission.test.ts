/**
 * hasPermission 匹配规则单测（permission-design §5.2 口径，2026-09-07 语义回写后）：
 * ① 精确码；② 纯服务码（冒号前段）；其余 false——不做 <服务>:read 兜底其他动作。
 */
import { describe, expect, it } from 'vitest'
import { hasPermission } from './permission'

describe('hasPermission 精确码（规则①）', () => {
  it('perms 含整码 → 授予', () => {
    expect(hasPermission('order:read', ['order:read', 'user:read'])).toBe(true)
  })

  it('服务级码精确匹配（authz:read）', () => {
    expect(hasPermission('authz:read', ['authz:read'])).toBe(true)
  })

  it('perms 含同名纯服务码且 code 无冒号 → 精确授予', () => {
    expect(hasPermission('order', ['order'])).toBe(true)
  })
})

describe('hasPermission 纯服务码（规则②）', () => {
  it('perms 含裸 <服务> → 该服务任意动作码授予', () => {
    expect(hasPermission('order:manage', ['order'])).toBe(true)
    expect(hasPermission('order:delete', ['audit', 'order'])).toBe(true)
  })
})

describe('hasPermission 不授予（fail-closed）', () => {
  // 回归钉住（计划语义回写）：read 不兜底其他动作
  it('order:manage vs perms 含 order:read → 不授予', () => {
    expect(hasPermission('order:manage', ['order:read'])).toBe(false)
  })

  it('空 perms → 一律不授予', () => {
    expect(hasPermission('order:read', [])).toBe(false)
    expect(hasPermission('order', [])).toBe(false)
  })

  it('无关服务码 → 不授予', () => {
    expect(hasPermission('order:write', ['user:read', 'audit:manage'])).toBe(false)
  })

  it('服务级动作不反向兜底：code 为服务串、perms 只有 <服务>:read → 不授予', () => {
    expect(hasPermission('order', ['order:read'])).toBe(false)
  })
})

describe('hasPermission 边界形态', () => {
  it('空服务名（":read"）不参与服务码比对，仅精确匹配', () => {
    expect(hasPermission(':read', [':read'])).toBe(true)
    expect(hasPermission(':read', ['read'])).toBe(false)
  })

  it('多段冒号按首段取服务', () => {
    expect(hasPermission('order:sub:manage', ['order'])).toBe(true)
  })
})

describe('hasPermission 通配码（规则表 prefix="*" 派生）', () => {
  it('*:read 授予任意服务的服务级码（菜单全可见）', () => {
    expect(hasPermission('redis:read', ['*:read'])).toBe(true)
    expect(hasPermission('authz:read', ['*:read'])).toBe(true)
  })
  it('*:read 不授予其他动作（菜单可见 ≠ 管理按钮可用）', () => {
    expect(hasPermission('authz:manage', ['*:read'])).toBe(false)
  })
  it('*:manage 授予任意服务管理动作', () => {
    expect(hasPermission('order:manage', ['*:read', '*:manage'])).toBe(true)
  })
  it('服务级通配 <服务>:* 授予该服务全部动作', () => {
    expect(hasPermission('order:manage', ['order:*'])).toBe(true)
    expect(hasPermission('order:read', ['order:*'])).toBe(true)
    expect(hasPermission('user:read', ['order:*'])).toBe(false)
  })
  it('* 与 *:* 授予一切', () => {
    expect(hasPermission('anything:at:all', ['*'])).toBe(true)
    expect(hasPermission('anything:at:all', ['*:*'])).toBe(true)
  })
})
