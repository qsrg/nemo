/**
 * 资源类型列表 API 包装（只读）。资源类型不再独立注册——类型由
 * 菜单/按钮声明驱动派生（resourceType=菜单隐含 can_view；资源级按钮
 * relation 贡献出口；actions 取并集按引擎白名单顺序渲染，见
 * logic/resourcetypesync.go）。本接口仅授权名单/列表展示聚合用：
 * 消费 authz 管理 API GET /v1/admin/resource-types（响应
 * ResourceTypeListResp{items,total}，字段核对
 * services/authz/internal/types/types.go）。
 */
import { http } from './client'

/** 已注册资源类型（actions 为声明的 can_* 出口清单，按模板固定顺序）。 */
export interface ResourceType {
  name: string
  actions: string[]
  /** 名单管理码（存量能力：派生保留历史值，新类型不设）。 */
  manageCode: string
}

/** 已注册类型列表（按名升序）。 */
export function listResourceTypes() {
  return http.get<{ items: ResourceType[]; total: number }>('/admin/resource-types')
}
