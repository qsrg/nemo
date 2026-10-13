// 菜单注册表：20+ 服务场景下的导航数据源（规范见 docs/frontend-design.md §菜单注册）
//
// 设计约定：
// - 服务是导航的一等公民：key 即一级路由段（/{key}/...），也是 wujie 应用名
// - 服务按「业务域」分组展示，避免 20+ 菜单平铺；域只是展示分组，不影响路由
// - menus 为服务内二级菜单，第一项是进入该服务的默认页
// - 菜单由基座统一渲染，服务不自带侧边栏 —— 保证交互/样式/权限控制的一致性
//
// 演进方向（demo 简化为静态表）：生产中每个服务在部署清单/manifest 中
// 声明自己的菜单片段，基座启动时聚合（可叠加后端权限过滤），本表退化为聚合结果。

export interface MenuItem {
  /** 菜单名，如「用户列表」 */
  title: string
  /** 完整路由，约定 /{service}/{page} */
  path: string
  /** 菜单图标（Element Plus 图标名，缺省沿用服务图标） */
  icon?: string
  /** 二级子菜单（三级菜单：服务 → 菜单 → 子菜单） */
  children?: MenuItem[]
}

export interface ServiceDef {
  /** 服务标识：一级路由段 + wujie 应用名 */
  key: string
  /** 服务显示名 */
  title: string
  /** 微应用入口 URL（生产由部署配置注入，本地为各服务 dev server） */
  url: string
  /** 业务域分组（侧边栏一级折叠标题） */
  domain: string
  /** 服务图标（Element Plus 图标组件，可选） */
  icon?: unknown
  /** 服务级权限码（已废弃，仅文档保留）：菜单级权限码改造后服务不再有
   *  独立服务级码，服务可见性由 me/menus 按菜单权限码服务端过滤承担
   *  （服务有任一可见菜单行即可见）；不再消费本字段 */
  /** 服务默认入口：库内重建时，若服务仅有根行（无子菜单），服务标题点击
   *  直接进入该入口；静态注册表无此字段 = 取首个菜单项 path */
  defaultPath?: string
  /** 服务内二级菜单 */
  menus: MenuItem[]
}

// 演示数据说明：demo 只有 user/order 两个真实微应用；其余服务复用
// 这两个应用的页面（url 轮流指向 5274/5275，各自独立 wujie 沙箱），
// 用于验证多服务规模下的导航与多开体验。生产中每个服务有独立应用。
export const SERVICES: ServiceDef[] = [
  // —— 存储域 ——
  // redis：实例管理页宿主（真实微应用 apps/redis :5277 + 独立后端 :8086，
  // 纯引擎可见性：owner/viewer/editor 关系，无归属列）
  { key: 'redis', title: 'Redis', url: 'http://localhost:5277/', domain: '存储域', icon: 'Coin', menus: [{ title: '实例管理', path: '/redis' }] },
  { key: 'storage', title: '分布式存储', url: 'http://localhost:5275/', domain: '存储域', icon: 'Box', menus: [{ title: '存储池', path: '/storage' }] },
  // —— 注册中心域 ——
  { key: 'zookeeper', title: 'Zookeeper', url: 'http://localhost:5278/', domain: '注册中心域', icon: 'Connection', menus: [{ title: '节点浏览', path: '/zookeeper' }] },
  // —— 消息域 ——
  // rocketmq：集群级授权试点微应用（apps/rocketmq :5279 + 独立后端 :8088；
  // 集群可见即该集群下 topic 全可见可操作，授权入口在页面内）
  { key: 'rocketmq', title: 'RocketMQ', url: 'http://localhost:5279/', domain: '消息域', icon: 'Connection', menus: [{ title: '集群管理', path: '/rocketmq/clusters' }, { title: 'Topic 管理', path: '/rocketmq/topics' }] },
  // kafka：Topic 管理页宿主（Phase C 改造③④）——集群概览（概览页）+
  // Topic 管理（apps/order 的 /kafka/topics 路由承载，ABAC 实测页）
  { key: 'kafka', title: 'Kafka', url: 'http://localhost:5275/', domain: '消息域', icon: 'DataLine', menus: [{ title: '集群概览', path: '/kafka' }, { title: 'Topic 管理', path: '/kafka/topics' }] },
  { key: 'eventbus', title: '事件总线', url: 'http://localhost:5274/', domain: '消息域', icon: 'Share', menus: [{ title: '事件查询', path: '/eventbus' }] },
  // —— 调度域 ——
  { key: 'cds', title: 'CDS', url: 'http://localhost:5275/', domain: '分库分表域', icon: 'DataAnalysis', menus: [{ title: '订阅管理', path: '/cds' }] },
  { key: 'schedule', title: 'Schedule', url: 'http://localhost:5274/', domain: '调度域', icon: 'Odometer', menus: [{ title: '调度任务', path: '/schedule' }] },
  { key: 'nujob', title: 'NuJob', url: 'http://localhost:5275/', domain: '调度域', icon: 'List', menus: [{ title: '作业列表', path: '/nujob' }] },
  // —— 监控运维域 ——
  { key: 'monitor', title: '集群监控', url: 'http://localhost:5274/', domain: '监控运维域', icon: 'Monitor', menus: [{ title: '集群总览', path: '/monitor' }] },
  { key: 'oncall', title: '组件值班', url: 'http://localhost:5275/', domain: '监控运维域', icon: 'Service', menus: [{ title: '值班表', path: '/oncall' }] },
  { key: 'install', title: '组件安装', url: 'http://localhost:5274/', domain: '监控运维域', icon: 'Upload', menus: [{ title: '安装任务', path: '/install' }] },
  { key: 'configcheck', title: '配置核查', url: 'http://localhost:5275/', domain: '监控运维域', icon: 'Stamp', menus: [{ title: '核查报告', path: '/configcheck' }] },
  // —— 安全合规域 ——
  // 安全审计：复用 authz-admin 微应用（:5276）的审计页（Audit.vue）。
  // 基座 entry 需为 /audit/xxx：子应用按 service=audit → base=/audit 剥前缀，
  // /audit/audit 剥后 = /audit 命中 authz-admin 内部路由 audit → Audit.vue。
  // 曾误指向 kafka（:5275）导致"审计日志"打不开（kafka 无 audit 路由）。
  { key: 'audit', title: '安全审计', url: 'http://localhost:5276/', domain: '安全合规域', icon: 'Files', menus: [{ title: '审计日志', path: '/audit/audit' }] },
  { key: 'review', title: '双人复核', url: 'http://localhost:5274/', domain: '安全合规域', icon: 'User', menus: [{ title: '待复核', path: '/review' }] },
  // nemo-bot 企业智能问答（外部项目接入示例）：/Users/wcf/go-project/nemo-bot/frontend，
  // Vue3+Vite 独立应用（:5173），经 micro.ts 三消息协议接入（会话接收存储于
  // nemo_platform_* 独立命名空间，本应用登录态不混用）
  { key: 'nemobot', title: '智能问答', url: 'http://localhost:5173/', domain: 'AI 助手域', icon: 'ChatDotRound', menus: [{ title: '智能问答', path: '/nemobot' }] },
  // authz 权限管理（Phase B）：真实微应用 apps/authz-admin（:5276）；
  // 菜单级权限码（authz-grants:read 等）由 me/menus 服务端按调用者过滤，
  // 服务可见性 = 有任一可见菜单行；token/permissions 经 wujie props 运行期
  // 下发（WujieHost subProps）
  {
    key: 'authz', title: '权限管理', url: 'http://localhost:5276/', domain: '安全合规域', icon: 'Lock',
    menus: [
      { title: '人员授权', path: '/authz/grants' },
      { title: '部门与成员', path: '/authz/orgs' },
      { title: '菜单管理', path: '/authz/menus' },
      { title: '审计', path: '/authz/audit' },
      { title: '资源共享', path: '/authz/shares' },
      { title: '资源类型', path: '/authz/types' },
    ],
  },
]

/** 业务域 → 服务的分组视图（侧边栏渲染用），保持注册顺序 */
export function groupByDomain(services: ServiceDef[]): Map<string, ServiceDef[]> {
  const map = new Map<string, ServiceDef[]>()
  for (const s of services) {
    if (!map.has(s.domain)) map.set(s.domain, [])
    map.get(s.domain)!.push(s)
  }
  return map
}

/** 按路由查服务（面包屑 / WujieHost 映射用） */
export function serviceByPath(path: string): ServiceDef | undefined {
  const key = path.split('/')[1]
  return SERVICES.find((s) => s.key === key)
}

/** 按路径取叶子菜单标题（最长前缀优先），页签命名用 */
export function leafTitle(service: ServiceDef, path: string): string {
  const hit = service.menus
    .filter((m) => path.startsWith(m.path))
    .sort((a, b) => b.path.length - a.path.length)[0]
  return hit?.title ?? service.title
}
