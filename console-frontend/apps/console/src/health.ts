// 平台态势数据（静态占位）：按生产要求的多机房多集群部署模型三级组织——
// 域 → 服务 → 机房 → 集群。每个服务声明各机房的集群数（集群规模随服务
// 与机房差异明显，如 Redis 在机房 10 有 10 个集群、机房 15 有 20 个），
// 集群实例数确定性生成，告警经 alerts.ts 按 serviceKey + 机房 + 集群关联。
// 首页态势卡（域级聚合）与域态势页（/domain/:domain，服务机房集群明细）
// 共用本模块；阶段二整体替换为监控服务/CMDB 接口，消费方只换 import 来源。
import { SERVICES } from './menus'
import { ALERT_ITEMS, firingAlerts, type AlertItem } from './alerts'

/** 机房命名：内部数据中心编号（与告警数据 region 字段对应） */
export const REGION_NAMES = ['机房10', '机房11', '机房12', '机房13', '机房14', '机房15'] as const

/** 服务 → 各机房集群数：数组下标即机房序（REGION_NAMES），长度 = 部署机房数 */
const CLUSTER_COUNTS: Record<string, number[]> = {
  // 存储域（6 机房，Redis 这类大规模服务集群数多）
  redis: [10, 12, 8, 6, 20, 5],
  storage: [4, 4, 3, 3, 4, 0],
  // 注册中心域（3 机房）
  zookeeper: [3, 3, 2],
  // 消息域（6 机房）
  rocketmq: [4, 4, 4, 3, 5, 2],
  kafka: [5, 4, 4, 6, 4, 3],
  eventbus: [2, 2, 2],
  // 分库分表域（3 机房）
  cds: [4, 3, 2],
  // 调度域（6 机房）
  schedule: [3, 3, 3, 2, 2, 2],
  nujob: [2, 2, 2],
  // 监控运维域（3 机房）
  monitor: [3, 3, 2],
  oncall: [2, 2, 1],
  install: [1, 1, 1],
  configcheck: [2, 2, 2],
  // 安全合规域（3 机房）
  audit: [2, 2, 2],
  review: [1, 1, 1],
  permission: [2, 2, 1],
}

/** 集群实例数：静态阶段确定性生成（2~6），真实环境由 CMDB/监控提供 */
function clusterInstances(serviceKey: string, region: string, idx: number): number {
  const seed = (serviceKey + region).split('').reduce((n, ch) => n + ch.charCodeAt(0), 0)
  return 2 + ((seed + idx * 7) % 5)
}

/** 集群名：服务-机房号-序号，如 rocketmq-14-05 */
function clusterName(serviceKey: string, region: string, idx: number): string {
  const regionNo = region.replace(/\D/g, '')
  return `${serviceKey}-${regionNo}-${String(idx + 1).padStart(2, '0')}`
}

/** 某服务的某个机房上未恢复的告警（告警中心同源数据） */
function firingAlertsOf(serviceKey: string, region: string): AlertItem[] {
  return firingAlerts.filter((a) => a.serviceKey === serviceKey && a.region === region)
}

export interface ClusterRow {
  /** 集群名（服务-机房-序号） */
  name: string
  /** 集群实例数 */
  instances: number
  /** 挂在该集群上的未恢复告警 */
  alerts: AlertItem[]
}

export interface RegionRow {
  /** 机房名（如 机房14） */
  name: string
  /** 该机房集群数 */
  clusterCount: number
  /** 该机房集群明细 */
  clusters: ClusterRow[]
  /** 该机房实例数（集群求和） */
  instances: number
  /** 该机房未恢复告警（集群级告警的机房视图） */
  alerts: AlertItem[]
}

export interface ServiceHealth {
  /** 服务 key（菜单注册表标识） */
  key: string
  /** 服务显示名 */
  title: string
  /** 服务图标（serviceIcons 键名） */
  icon: string
  /** 所属业务域 */
  domain: string
  /** 各机房明细（数组顺序即机房，长度 = 部署机房数） */
  regions: RegionRow[]
  /** 聚合实例数 */
  instances: number
  /** 聚合集群数 */
  clusterCount: number
  /** 未恢复告警数 */
  alarmCount: number
  /** 服务默认页（服务头行「去处理」用） */
  path: string
}

/** 服务级健康：全部服务的机房/集群部署明细（域态势页数据源）。
 *
 * visible：服务可见键集合（基座菜单 store 的 items key 集——me/menus 服务端
 * 按权限过滤后的口径）；传入后不可见服务不产出健康数据，域聚合/域态势页
 * 随之只见可见服务（首页态势卡与菜单同一套可见性，无权限服务不再展示）。
 * 阶段二替换为监控服务/CMDB 接口时，权限过滤同样在此层叠加。
 */
export function serviceHealthList(visible?: ReadonlySet<string>): ServiceHealth[] {
  return SERVICES.flatMap((s) => {
    if (visible && !visible.has(s.key)) return []
    const counts = CLUSTER_COUNTS[s.key] ?? []
    if (!counts.length) return []
    const regions: RegionRow[] = counts.map((clusterCount, i) => {
      const name = REGION_NAMES[i] ?? `机房${i + 1}`
      const clusters: ClusterRow[] = Array.from({ length: clusterCount }, (_, idx) => ({
        name: clusterName(s.key, name, idx),
        instances: clusterInstances(s.key, name, idx),
        alerts: firingAlertsOf(s.key, name).filter((a) => a.cluster === clusterName(s.key, name, idx)),
      }))
      return {
        name,
        clusterCount,
        clusters,
        instances: clusters.reduce((n, c) => n + c.instances, 0),
        alerts: firingAlertsOf(s.key, name),
      }
    })
    return [{
      key: s.key,
      title: s.title,
      icon: s.icon ?? 'Menu',
      domain: s.domain,
      regions,
      instances: regions.reduce((n, r) => n + r.instances, 0),
      clusterCount: regions.reduce((n, r) => n + r.clusterCount, 0),
      alarmCount: regions.reduce((n, r) => n + r.alerts.length, 0),
      path: s.menus[0].path,
    }]
  })
}

export interface DomainHealth {
  /** 业务域名 */
  domain: string
  /** 域内服务健康（保持注册表顺序） */
  services: ServiceHealth[]
  /** 聚合实例数（域内全部服务求和） */
  instances: number
  /** 聚合集群数 */
  clusterCount: number
  /** 聚合未恢复告警数 */
  alarmCount: number
  /** 机房分布：跨服务按机房聚合（首页态势卡分布条用） */
  regionStrip: { name: string; instances: number; alarms: number }[]
}

/** 域级健康：由服务级数据聚合（首页态势卡数据源）。visible 语义同
 * serviceHealthList；某域可见服务为空时该域不产出（首页不显示该域卡）。 */
export function domainHealthList(visible?: ReadonlySet<string>): DomainHealth[] {
  const all = serviceHealthList(visible)
  return [...new Set(all.map((s) => s.domain))].map((domain) => {
    const services = all.filter((s) => s.domain === domain)
    const maxRegions = Math.max(...services.map((s) => s.regions.length))
    const regionStrip = Array.from({ length: maxRegions }, (_, i) => ({
      name: REGION_NAMES[i] ?? `机房${i + 1}`,
      instances: services.reduce((n, s) => n + (s.regions[i]?.instances ?? 0), 0),
      alarms: services.reduce((n, s) => n + (s.regions[i]?.alerts.length ?? 0), 0),
    }))
    return {
      domain,
      services,
      instances: services.reduce((n, s) => n + s.instances, 0),
      clusterCount: services.reduce((n, s) => n + s.clusterCount, 0),
      alarmCount: services.reduce((n, s) => n + s.alarmCount, 0),
      regionStrip,
    }
  })
}

/** 按域名取单个域的健康数据（域态势页用）；visible 语义同 serviceHealthList。 */
export function domainHealthOf(domain: string, visible?: ReadonlySet<string>): DomainHealth | undefined {
  return domainHealthList(visible).find((d) => d.domain === domain)
}
