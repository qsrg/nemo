// 告警数据（静态占位）：首页「待关注」与告警中心（/alerts）共用。
// 阶段二由监控服务接口提供，替换本模块即可（消费方只需换 import 来源）。
// 注意：ALERT_ITEMS 的机房告警数与 Workbench.vue 的 ALARM_SNAPSHOT 对应
// （消息域杭州机房 2 条 firing），两处同源替换时一起迁移。
export interface AlertDetail {
  /** 详情字段名 */
  label: string
  /** 详情字段值（静态演示为文案，阶段二为真实指标值） */
  value: string
}

export interface AlertItem {
  id: string
  severity: 'critical' | 'major'
  /** 告警状态：firing 告警中 / resolved 已恢复 */
  status: 'firing' | 'resolved'
  /** 所属服务显示名 */
  service: string
  /** 服务 key（菜单注册表的服务标识，态势页按它关联服务与机房） */
  serviceKey: string
  /** 服务图标（serviceIcons 键名） */
  icon: string
  domain: string
  /** 告警机房名 */
  region: string
  /** 告警所在集群名（可缺省：机房级告警不指定集群） */
  cluster?: string
  /** 告警标题：一句话说清发生了什么 */
  title: string
  /** 开始时间（静态演示为文案） */
  startedAt: string
  /** 已持续时长 */
  duration: string
  /** 详情：触发条件、影响范围等 */
  details: AlertDetail[]
  /** 处理建议 */
  advice: string
  /** 单击「去处理」直达的服务页 */
  path: string
}

export const ALERT_ITEMS: AlertItem[] = [
  {
    id: 'rocketmq-disk-hz',
    severity: 'critical',
    status: 'firing',
    service: 'RocketMQ',
    serviceKey: 'rocketmq',
    icon: 'Connection',
    domain: '消息域',
    region: '机房14',
    cluster: 'rocketmq-14-05',
    title: '磁盘水位超过 85%，写入可能被限流',
    startedAt: '09-06 08:12',
    duration: '2 小时',
    details: [
      { label: '触发条件', value: '磁盘水位 87.2%（阈值 85%）' },
      { label: '影响范围', value: '杭州机房 · 3 实例 · 写入路径' },
    ],
    advice: '清理过期数据或对杭州机房节点扩容磁盘，处理前关注写入限流情况。',
    path: '/rocketmq',
  },
  {
    id: 'kafka-lag-sh',
    severity: 'major',
    status: 'firing',
    service: 'Kafka',
    serviceKey: 'kafka',
    icon: 'DataLine',
    domain: '消息域',
    region: '机房14',
    cluster: 'kafka-14-03',
    title: '消费组堆积超过 10 万条',
    startedAt: '09-06 05:40',
    duration: '5 小时',
    details: [
      { label: '触发条件', value: '订单同学生产组堆积 12.4 万条（阈值 10 万）' },
      { label: '影响范围', value: '杭州机房 · 消费延迟约 6 分钟' },
    ],
    advice: '确认下游消费者实例数与处理能力，必要时提升消费并发。',
    path: '/kafka',
  },
  {
    id: 'redis-slowlog-bj',
    severity: 'major',
    status: 'resolved',
    service: 'Redis',
    serviceKey: 'redis',
    icon: 'Coin',
    domain: '存储域',
    region: '机房10',
    title: '慢查询数突增至基线 8 倍',
    startedAt: '09-05 14:20',
    duration: '40 分钟',
    details: [
      { label: '触发条件', value: '慢查询 62 次/分钟（基线 8 次/分钟）' },
      { label: '影响范围', value: '北京机房 · 缓存读路径' },
      { label: '恢复原因', value: '业务侧下线大 Key 全量扫描任务' },
    ],
    advice: '已恢复；建议业务侧评审大 Key 使用方式。',
    path: '/redis',
  },
  {
    id: 'schedule-retry-bj',
    severity: 'major',
    status: 'resolved',
    service: 'Schedule',
    serviceKey: 'schedule',
    icon: 'Odometer',
    domain: '调度域',
    region: '机房10',
    title: '调度任务连续失败重试',
    startedAt: '09-05 02:00',
    duration: '25 分钟',
    details: [
      { label: '触发条件', value: '对账任务连续失败 5 次触发重试' },
      { label: '影响范围', value: '北京机房 · 对账链路延迟' },
      { label: '恢复原因', value: '依赖的上游存储抖动恢复' },
    ],
    advice: '已恢复；当日对账已补跑完成。',
    path: '/schedule',
  },
]

/** 未恢复（告警中）的告警：首页「待关注」与顶栏铃铛计数用 */
export const firingAlerts = ALERT_ITEMS.filter((a) => a.status === 'firing')
