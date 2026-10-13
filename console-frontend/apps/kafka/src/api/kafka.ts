/**
 * Kafka Topic 管理 API 包装（Phase C 改造③④ → 菜单级授权改造 → 通用
 * relations 收敛）：
 * 消费 authz 演示域 API /v1/kafka/topics*，字段核对
 * services/authz/internal/types/types.go：
 * - KafkaTopicResp{id,name,env,description,orgId,createdBy,canEdit,canDelete,canManageGrants,owner}
 * - CreateKafkaTopicReq{name,env,description}
 * - UpdateKafkaTopicReq{id 路径参数 + env/description 全量覆盖}
 * - ListTopicsReq{page,pageSize}（分页：可见性谓词下推 SQL + 引擎逐行复核）
 *
 * ABAC 方案 A 改造：requiredLevel 已删除（旧 ABAC 等级门槛遗留字段，不参与
 * 判定）；鉴权属性统一由 authz 属性存储 + ABAC 策略管理，业务表单不再承载。
 *
 * 菜单级授权：页面可见性由 kafka-topics:read（菜单码）控制，写操作
 *（增删改）由 kafka-topics:manage 控制——后端 create/update/delete 先
 * ListObjects 实时判定 manage 码（不满足 403 AUTHZ_DENIED），前端按钮按
 * usePermission().canManage('kafka-topics') 显隐。
 *
 * Topic 名单管理（viewer/editor）已收拢到通用资源授权 API /v1/authz/relations
 * （GET 列表 / POST 写 / DELETE 撤，见 @nemo/shared ResourceGrantDialog）；
 * 原 kafka 专用 /topics/:id/grants 端点保留兼容、不再被前端调用。
 * 原 X-User-Level（ABAC 模拟等级）头已移除。
 *
 * 注：消息域演示页——真实环境由独立 Kafka 管理服务承载，同模式接入 kit/authz。
 */
import { createMicroHttp } from '@nemo/shared'

/** 微应用统一 http 工厂（基础能力层）：token 实时取 session、401 竞态区分、
 *  自定义头支持——替代本地自写 fetch 封装（原 apps/order/src/api/http.ts）。 */
export const http = createMicroHttp()

/** Topic 行（kafka KafkaTopicResp，含逐条判定徽标；orgId 为归属部门
 *（列投影，供展示——权限真源是引擎 org 元组）。canManageGrants = owner
 * 关系 + kafka-topics:manage 码回退，与后端名单管理守卫同源，前端直接
 * 消费、不再用 owner 自行推导；owner 为纯 owner 关系（对话框初始态用）。 */
export interface KafkaTopic {
  id: string
  name: string
  env: string
  description: string
  orgId: string
  createdBy: string
  canEdit: boolean
  canDelete: boolean
  canManageGrants: boolean
  canView: boolean
  owner: boolean
}

/** 菜单按钮声明（authz 判定面 /v1/authz/menu-buttons）：Topic 管理页操作列
 * 按此声明驱动渲染——按钮文案/授权码（permissionCode）/模型出口（relation）/
 * 归属类型来自菜单配置，改菜单声明即改页面按钮（显隐再按行 canX 字段匹配判定）。 */
export interface MenuButtonDecl {
  /** 按钮授权码（页面级按钮的功能码，资源级按钮为空串）。 */
  permissionCode: string
  title: string
  resourceType: string
  relation: string
}

/** 拉取 Topic 管理页菜单下的按钮声明（父菜单按路由定位 path=/kafka/topics；
 *  登录即可读，按钮定义非敏感，授权判定在消费方按关系进行）。 */
export function getTopicMenuButtons() {
  return http.get<{ items: MenuButtonDecl[] }>(
    '/authz/menu-buttons?path=/kafka/topics',
  )
}

/** Topic 列表（分页：服务端按可见性谓词查页 + 引擎复核，total 为满足
 * 谓词的近似总数——ABAC 门槛启用时页内可能少于 pageSize）。 */
export function listKafkaTopics(page = 1, pageSize = 20) {
  return http.get<{ items: KafkaTopic[]; total: number }>(
    `/kafka/topics?page=${page}&pageSize=${pageSize}`,
  )
}

/** 新建 Topic（创建人即 owner；需 kafka-topics:manage）。
 *  orgId 归属组织（可选）：指定则该组织（含子树成员）可见，缺省回退创建人所在部门。 */
export function createKafkaTopic(data: {
  name: string
  env: string
  description: string
  orgId?: string
}) {
  return http.post<KafkaTopic>('/kafka/topics', data)
}

/** 更新 Topic（后端先判定 kafka-topics:manage，不满足 403 AUTHZ_DENIED）。 */
export function updateKafkaTopic(id: string, data: {
  env: string
  description: string
}) {
  return http.put<KafkaTopic>(`/kafka/topics/${encodeURIComponent(id)}`, data)
}

/** 删除 Topic（后端先判定 kafka-topics:manage；删库 + 清全部元组）。 */
export function deleteKafkaTopic(id: string) {
  return http.delete<void>(`/kafka/topics/${encodeURIComponent(id)}`)
}

/** 导出 Topic 清单（CSV 文本；需 kafka-topics:export 动作码，后端守卫）。
 *  导出范围 = 当前用户可见行（与列表同口径，不因导出扩大可见范围）。 */
export function exportKafkaTopics() {
  return http.post<string>('/kafka/topics/export')
}

/** 重置某 Topic 的消费者位点（demo 语义；需 kafka-topics:reset-offset
 *  动作码 + 该 topic 可编辑，两层判定后端完成）。 */
export function resetKafkaTopicOffset(id: string) {
  return http.post<{ id: string; reset: boolean }>(
    `/kafka/topics/${encodeURIComponent(id)}/reset-offset`,
  )
}

/** 消费者组信息（生产环境来自 Kafka AdminClient；demo 无集群时为空）。 */
export interface TopicConsumerGroup {
  groupId: string
  state: string
  members: number
  lag: number
}

/** 查看某 Topic 的消费者组（行级只读动作，鉴权复用 can_view；见后端 topicCanView）。 */
export function getTopicConsumers(id: string) {
  return http.get<{ topicId: string; items: TopicConsumerGroup[] }>(
    `/kafka/topics/${encodeURIComponent(id)}/consumers`,
  )
}
