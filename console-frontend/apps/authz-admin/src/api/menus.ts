/**
 * 菜单管理 API 包装（菜单模型简化改造：id 引用 + permission_code 列）：
 * 消费 authz 管理 API /v1/admin/menus*，字段核对
 * services/authz/internal/types/types.go：
 * - MenuResp{id,parentId,type,title,icon,path,sort,permissionCode,resourceType,relation,enabled,createdAt,updatedAt}
 * - CreateMenuReq{type,parentId,title,icon,path,sort,permissionCode,resourceType,relation}
 * - UpdateMenuReq{id 路径参数定位 + type/title/icon/path/sort/permissionCode/resourceType/relation/enabled}
 *   （parentId/type 为行身份锚点，更新不可改）
 *
 * 行身份 = 自增 id，父子引用 = parentId（顶级为 0）。
 * permissionCode 权限标识：菜单行可选（空 = 全员可见）；页面级按钮
 *（type=button 且 relation 空）必填；资源级按钮（relation 非空）为空串；
 * 非空码全表唯一（重复 409 AUTHZ_MENU_EXISTS）。
 * 错误码：400 AUTHZ_MENU_INVALID（必填/命名非法）、409 AUTHZ_MENU_EXISTS
 *（重码）、404 AUTHZ_MENU_NOT_FOUND（更新/删除未命中）。
 */
import { http } from './client'

/** 菜单注册行（authz MenuResp）。 */
export interface MenuRow {
  /** 行 id（PUT/DELETE 以 id 定位行）。 */
  id: number
  /** 父行 id（顶级/服务首页为 0）。 */
  parentId: number
  /** 行类型：menu（导航菜单，默认）| button（按钮级授权实体，挂父菜单下）。 */
  type: string
  title: string
  icon: string
  path: string
  sort: number
  /** 权限标识：菜单行可空（空 = 全员可见）；页面级按钮必填；资源级按钮为空串。 */
  permissionCode: string
  resourceType: string
  /** 资源级按钮的资源关系（如 can_freeze）：仅按钮行且资源级时非空。 */
  relation: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

/** me/menus 响应行（authz MeMenuItem，服务端已按调用者权限过滤，
 *  仅含 enabled 且可见的菜单行；按钮行不下发）。 */
export interface MeMenuItem {
  id: number
  parentId: number
  title: string
  icon: string
  path: string
  sort: number
  /** 权限标识（可空：空 = 全员可见）。 */
  permissionCode: string
}

/** 菜单注册表全量列表（含 disabled，按 (sort, id) 升序）。 */
export function listMenus() {
  return http.get<{ items: MenuRow[]; total: number }>('/admin/menus')
}

/** 资源级按钮 relation 引擎出口白名单（有序，如 can_edit / can_delete）：
 * 后端代码常量随版本发布（与 modelgen canExits 同源），本接口只读下发——
 * 菜单管理页「资源关系」下拉选项的唯一数据源，前端不硬编码副本。 */
export function listMenuRelations() {
  return http.get<{ items: string[] }>('/admin/menus/relations')
}

/** 资源类型列表（新增菜单/按钮时资源类型输入联想的数据源）。 */
export function listResourceTypes() {
  return http.get<{ items: { name: string; actions: string[]; manageCode: string }[]; total: number }>('/admin/resource-types')
}

/** 新建菜单/按钮行（parentId=0 = 顶级行；按钮行 parentId 指向所属菜单行）。 */
export function createMenu(data: {
  type?: string
  parentId?: number
  title: string
  icon: string
  path: string
  sort: number
  permissionCode: string
  resourceType: string
  relation?: string
}) {
  return http.post<MenuRow>('/admin/menus', data)
}

/** 更新菜单/按钮行（id 定位；type/parentId 身份锚点不可改，字段全量覆盖）。 */
export function updateMenu(id: number, data: {
  type: string
  title: string
  icon: string
  path: string
  sort: number
  permissionCode: string
  resourceType: string
  relation?: string
  enabled: boolean
}) {
  return http.put<MenuRow>(`/admin/menus/${id}`, data)
}

/** 删除菜单行（按 id；调用方先做二次确认）。 */
export function deleteMenu(id: number) {
  return http.delete<void>(`/admin/menus/${id}`)
}
