// 服务概览数据（微应用侧静态演示）：与基座态势体系同构的「服务 → 机房 → 集群」
// 三级模型。微应用按基座下发的 props.base（如 /redis）渲染对应服务的集群列表，
// 阶段二替换为各服务自己的接口数据（经 @nemo/http 调用，失败回退本演示数据）。

/** 服务 key → 显示名（与基座菜单注册表 title 一致） */
export const SERVICE_TITLES: Record<string, string> = {
  redis: 'Redis',
  storage: '分布式存储',
  zookeeper: 'Zookeeper',
  rocketmq: 'RocketMQ',
  kafka: 'Kafka',
  eventbus: '事件总线',
  cds: 'CDS',
  schedule: 'Schedule',
  nujob: 'NuJob',
  monitor: '集群监控',
  oncall: '组件值班',
  install: '组件安装',
  configcheck: '配置核查',
  audit: '安全审计',
  review: '双人复核',
  permission: '权限管理',
}

/** 机房编号：与基座态势数据的机房命名对应 */
export const REGION_NAMES = ['机房10', '机房11', '机房12', '机房13', '机房14', '机房15'] as const

/** 服务 → 各机房集群数（演示值：核心服务 6 机房、其余 3 机房；规模随服务/机房差异） */
const CLUSTER_COUNTS: Record<string, number[]> = {
  redis: [10, 12, 8, 6, 20, 5],
  storage: [4, 4, 3, 3, 4, 0],
  zookeeper: [3, 3, 2],
  rocketmq: [4, 4, 4, 3, 5, 2],
  kafka: [5, 4, 4, 6, 4, 3],
  eventbus: [2, 2, 2],
  cds: [4, 3, 2],
  schedule: [3, 3, 3, 2, 2, 2],
  nujob: [2, 2, 2],
  monitor: [3, 3, 2],
  oncall: [2, 2, 1],
  install: [1, 1, 1],
  configcheck: [2, 2, 2],
  audit: [2, 2, 2],
  review: [1, 1, 1],
  permission: [2, 2, 1],
}

export interface ClusterRow {
  /** 集群名（服务-机房号-序号，与基座态势页命名一致） */
  name: string
  /** 所属机房（如 机房14） */
  region: string
  /** 集群实例数 */
  instances: number
  /** 集群状态（演示恒为正常；真实状态阶段二由接口提供） */
  status: '正常' | '告警'
}

/** 集群实例数：静态阶段确定性生成（2~6），真实环境由服务自己的接口提供 */
function clusterInstances(serviceKey: string, region: string, idx: number): number {
  const seed = (serviceKey + region).split('').reduce((n, ch) => n + ch.charCodeAt(0), 0)
  return 2 + ((seed + idx * 7) % 5)
}

/**
 * 生成某服务的集群列表（按机房分组排序）。
 * @param serviceKey 服务标识（props.base 去掉斜杠，如 redis）
 * @returns 集群行数组；未登记的服务返回空数组（页面显示空态）
 */
export function clusterRowsOf(serviceKey: string): ClusterRow[] {
  const counts = CLUSTER_COUNTS[serviceKey] ?? []
  const rows: ClusterRow[] = []
  counts.forEach((count, i) => {
    const region = REGION_NAMES[i] ?? `机房${i + 1}`
    for (let idx = 0; idx < count; idx++) {
      rows.push({
        name: `${serviceKey}-${region.replace(/\D/g, '')}-${String(idx + 1).padStart(2, '0')}`,
        region,
        instances: clusterInstances(serviceKey, region, idx),
        status: '正常',
      })
    }
  })
  return rows
}

/** 服务显示名（未登记的 key 回退显示 key 本身） */
export function serviceTitle(serviceKey: string): string {
  return SERVICE_TITLES[serviceKey] ?? serviceKey
}
