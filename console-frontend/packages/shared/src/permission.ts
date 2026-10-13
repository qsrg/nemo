/**
 * 前端权限码匹配（permission-design §5.2，packages/shared 统一入口）。
 *
 * authz `GET /api/v1/me/permissions` 下发当前用户的「服务:动作」权限码集合
 * （如 `["order:read", "user:manage"]`，由平台角色 × 路由规则表派生），
 * 基座菜单与微应用按钮用 hasPermission 判定显隐——不可见即隐藏（fail-closed）。
 */

/**
 * 判定权限码 code 是否被权限集合 perms 授予（§5.2 口径，自上而下短路）：
 * ① 精确码：perms 含 code 本身 → 授予；
 * ② 通配码：perms 含 `*:<动作>`（跨服务同动作，如规则表 prefix='*' →
 *   `*:read` 授予全部服务菜单）或 `<服务>:*`（跨动作）或 `*`/`*:*`（全部）→ 授予；
 * ③ 纯服务码：perms 含 code 的冒号前段裸 `<服务>` → 授予
 *   （服务级全量授权的粗粒度形态；当前派生口径不产生该形态，留作演进）；
 * 其余一律不授予——不做 `<服务>:read` 兜底其他动作（菜单检查的 code 本身
 * 就是服务级码如 `authz:read`，精确匹配即可），前端显隐 fail-closed，
 * 资源级安全由服务端 Check 把守。
 */
export function hasPermission(code: string, perms: string[]): boolean {
  // ① 精确码
  if (perms.includes(code)) return true
  const sep = code.indexOf(':')
  if (sep <= 0) return false
  const service = code.slice(0, sep)
  const action = code.slice(sep + 1)
  // ② 通配码：`*:<动作>` / `<服务>:*` / `*:*` / `*`（动作/服务为空串视为通配位）
  if (
    perms.includes(`*:${action}`) ||
    perms.includes(`${service}:*`) ||
    perms.includes('*:*') ||
    perms.includes('*')
  ) {
    return true
  }
  // ③ 纯服务码；空服务名（如 ":read"）不参与
  return perms.includes(service)
}

/**
 * 服务级权限码：ServiceDef.permission 显式声明优先，缺省派生 `{key}:read`
 * （code 本身即服务级码，permission-design §5.2）——基座菜单/页签/容器
 * 与微应用统一经此取码再交 hasPermission 判定，避免派生规则散落重复。
 */
export function serviceCode(s: { permission?: string; key: string }): string {
  return s.permission ?? s.key + ':read'
}
