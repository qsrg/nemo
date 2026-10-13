/**
 * 人员授权（去角色化后的任务导向主页）：
 * 「选人/选部门 → 看 TA 现在有什么 → 按菜单拨级别 → 保存即生效」。
 *
 * 授权模型：权限点即引擎对象，授权 = 人/部门 × 权限点 一条元组（can），
 * 写入判定面即时生效（/me/permissions 实时查询），给部门授权对该部门子树
 * （直属 + 全部子孙部门）的成员即时生效（模型 v5：member from child）。
 *
 * 授权粒度（菜单级，permission-design §5.2 两段式）：每个菜单独立
 * read/manage 码（如 authz-grants:read / authz-grants:manage），矩阵每行
 * 一个菜单，可单独授权——不再有"服务级一起给"的粗粒度。
 *
 * 交互设计（2026-09 易用性重构）：
 * - 级别三档单选（无权限/可查看/可管理）：替代双 checkbox + 隐藏联动——控件即语义，
 *   不存在"取消可查看连可管理一起没了"的隐性规则；「可管理」仍映射 read+manage
 *   两码，保存差集逻辑不变。
 * - 继承锁定：经部门继承的权限在矩阵中以「禁用 + 停在继承档位」呈现，
 *   悬停提示来源部门与调整路径——矩阵与总览不再打架。
 * - 矩阵搜索 + 吸底操作条：菜单目录可按名称/码/路径过滤；待保存变更数
 *   实时可见，保存/重置随手可及；切换授权对象前有未保存更改会确认。
 * - 「全部菜单」通配置顶：批量授权是高频操作，不再沉底。
 */
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { OrgTreePicker, useApiError, usePermission, useSession } from '@nemo/shared'
import { grantPermissions, revokePermission, permissionOverview, listPermissionGrants } from '../api/permissions'
import { listMenus } from '../api/menus'
import PermissionPreviewDialog from './components/PermissionPreviewDialog.vue'
import GrantSubjectsDialog from './components/GrantSubjectsDialog.vue'

/** 按钮级权限：写操作仅当前菜单（人员授权）的 manage 码可见（fail-closed）。
 *  注意 usePermission.canManage 是 (service)=>boolean 函数，解构后需包一层
 *  computed 才能作布尔/ref 使用（历史误用：裸函数当 ref 导致 save 恒被拦截）。 */
const { canManage: hasManageCode } = usePermission()
const canManage = computed(() => hasManageCode('authz-grants'))
const handleApiError = useApiError()

// ---------------------------------------------------------------------------
// 授权目标
// ---------------------------------------------------------------------------
const targetType = ref<'user' | 'org'>('user')
const userId = ref('')
const orgId = ref('')
const loadingTarget = ref(false)
/** 组织架构单选树（基础能力层组件：左栏定位组织→选人，部门/用户合一；
 *  内部懒加载 /admin/orgs 与 /admin/orgs/:id/members，暴露 orgs 与
 *  显示名查询供本页反查）。 */
const orgTreePickerRef = ref<InstanceType<typeof OrgTreePicker>>()

/** 部门扁平列表（供 GrantSubjectsDialog 反查部门名；数据由 OrgTreePicker
 *  懒加载后暴露，未加载时为空数组而非 undefined，避免组件 prop 类型校验失败）。 */
const orgs = computed(() => orgTreePickerRef.value?.orgs ?? [])

/** 按钮权限行展开状态：base → true 表示已展开；默认折叠，避免一屏被 checkbox 淹没。 */
const expandedBtnRows = ref<Set<string>>(new Set())
function toggleBtnRow(base: string) {
  const s = expandedBtnRows.value
  if (s.has(base)) { s.delete(base); expandedBtnRows.value = new Set(s) }
  else { s.add(base); expandedBtnRows.value = new Set(s) }
}
function isBtnRowExpanded(base: string): boolean { return expandedBtnRows.value.has(base) }

/** 总览区权限码折叠状态：管理员直接/继承授予的权限码可能铺满一屏，
 *  默认仅显示前 N 个 + 「还有 X 项」展开标记，避免摘要卡被淹没。
 *  每个 ov-block 独立折叠（用块 key 标识：direct / viaOrgs:{orgId} 等）。 */
const OV_PREVIEW_LIMIT = 6
const expandedOverviews = ref<Set<string>>(new Set())
function toggleOverviewExpanded(blockKey: string) {
  const s = expandedOverviews.value
  if (s.has(blockKey)) { s.delete(blockKey); expandedOverviews.value = new Set(s) }
  else { s.add(blockKey); expandedOverviews.value = new Set(s) }
}
function isOverviewExpanded(blockKey: string): boolean { return expandedOverviews.value.has(blockKey) }

/** 当前选中节点 key（org:{id} / user:{uid}），驱动左树高亮回显。 */
const currentKey = computed(() =>
  targetType.value === 'user' ? (userId.value ? `user:${userId.value}` : '') : orgId.value ? `org:${orgId.value}` : '',
)

/** 是否已选中授权对象（右栏空态引导用）。 */
const targetSelected = computed(() => (targetType.value === 'user' ? !!userId.value : !!orgId.value))

/** 右栏对象卡展示名：用户 = 显示名（userId），部门 = 部门名。 */
const objectName = computed(() => {
  if (targetType.value === 'user') {
    const n = orgTreePickerRef.value?.getMemberName(userId.value) ?? ''
    return n ? `${n}（${userId.value}）` : userId.value || '未选择'
  }
  const n = orgTreePickerRef.value?.getOrgName(orgId.value) ?? ''
  return n || orgId.value || '未选择'
})

/** 直接授权计数（对象卡摘要）。 */
const directCount = computed(() =>
  targetType.value === 'user' ? overview.value.direct.length : orgOverview.value.direct.length,
)

/** 经组织继承的授权计数（对象卡摘要）。 */
const inheritedCount = computed(() => {
  if (targetType.value === 'user') {
    return overview.value.viaOrgs.reduce((n, o) => n + o.codes.length, 0)
  }
  return orgOverview.value.ancestors.reduce((n, o) => n + o.codes.length, 0)
})

/** 模拟用户视角对话框显隐（模板引用，必须声明）。 */
const previewVisible = ref(false)

// ---------------------------------------------------------------------------
// 总览（当前有效权限 + 来源）
// ---------------------------------------------------------------------------
const overview = ref<{ direct: string[]; viaOrgs: { orgId: string; orgName: string; codes: string[] }[]; roles: string[] }>({
  direct: [],
  viaOrgs: [],
  roles: [],
})
/** 部门视角的成员有效权限来源：本部门直接授予 + 沿父链各上级部门的直接授予
 * （模型 v5：部门成员 ⊆ 祖先部门成员集合，祖先的授权对本部门成员生效）。 */
const orgOverview = ref<{ direct: string[]; ancestors: { orgId: string; orgName: string; codes: string[] }[] }>({
  direct: [],
  ancestors: [],
})
const orgCodes = ref<string[]>([]) // 部门目标的直接授权（无继承总览）

/** 判定是否通配码（* / *:* / *:动作 / 服务:*）：总览里需视觉强调并附说明，
 *  非技术管理员无法从码串本身理解其含义。 */
function isWildcardCode(code: string): boolean {
  return code === '*' || code === '*:*' || code.startsWith('*:') || code.endsWith(':*')
}

/** 当前目标直接授予中的通配码（存在即总览区展示说明块）。 */
const wildcardCodes = computed(() => overview.value.direct.filter(isWildcardCode))

/** 通配码 → 一句含义说明（用于总览说明块，解释它为什么让普通码"多余"）。 */
function wildcardHint(code: string): string {
  if (code === '*' || code === '*:*') return '匹配任意「服务:动作」——等价全部菜单·管理与全部按钮，系统引导授予（超管），非本矩阵配置'
  if (code === '*:read') return '匹配任意服务的只读档'
  if (code.endsWith(':*')) return `匹配「${code.slice(0, -2)}」服务下的全部动作`
  return `匹配任意服务的「${code.slice(2)}」动作`
}

/** 通配权限码的人话展示：*:read 等码对非技术管理员不可读。 */
function codeDisplayName(code: string): string {
  if (code === '*' || code === '*:*') return '全部权限（超管）'
  if (code === '*:read') return '全部菜单·可查看'
  if (code === '*:manage') return '全部菜单·可管理'
  // 按钮码（如 kafka-topics:create）反查按钮名，避免裸露码串。
  const t = pageButtonTitle(code)
  if (t) return t
  // 菜单码（read/manage）映射为「菜单名·档位」。
  const m = menuTitleOf(code)
  if (m) return m
  return code
}

/** 按钮码 → 按钮名（用于总览人话展示；未匹配返回空串）。 */
function pageButtonTitle(code: string): string {
  for (const r of serviceRows.value) {
    for (const b of r.pageButtons ?? []) {
      if (b.permissionCode === code) return b.title
    }
  }
  return ''
}

/** 菜单档位码 → 「菜单名·可查看/可管理」（用于总览人话展示；未匹配返回空串）。 */
function menuTitleOf(code: string): string {
  for (const r of serviceRows.value) {
    if (r.readCode === code) return `${r.title}·可查看`
    if (r.manageCode === code) return `${r.title}·可管理`
  }
  return ''
}

// ---------------------------------------------------------------------------
// 菜单目录（勾选矩阵数据源，来自菜单注册表）
// 授权粒度 = 菜单：每个菜单行独立 read/manage 码（两段式，如
// authz-grants:read / authz-grants:manage），可单独授权、互不影响。
// ---------------------------------------------------------------------------
/** 菜单下声明的按钮（按钮实体化：type=button 行挂父菜单下）：
 *  页面级（relation 空）= 功能码，矩阵勾选；资源级（relation 非空）=
 *  资源关系，授权跟随「数据权限」三档、矩阵只读展示。 */
interface MenuButton {
  /** 授权码（页面级）= permissionCode（如 kafka-topics:create）。 */
  permissionCode: string
  /** 按钮名（如 新建 Topic）。 */
  title: string
  /** 资源级按钮的资源关系（如 can_freeze）；页面级为空串。 */
  relation: string
  /** 资源级按钮关联的资源类型（如 kafka_topic）。 */
  resourceType: string
}
interface MenuGrantRow {
  /** 菜单行 id。 */
  id: number
  /** 父菜单行 id（二级菜单非 0；顶级行为 0）。 */
  parentId: number
  /** 权限码 base 段（= readCode 去掉 :read；引擎对象不允许冒号，菜单级码以连字符
   *  连接）。空码菜单行 base 为空串——树骨架节点（无档位可授；可见性：
   *  顶级空码行全员可见，非顶级空码子行继承父级，见 memenuslogic）。 */
  base: string
  /** 所属服务段（路由首段，如 /kafka/topics → kafka） */
  service: string
  title: string
  path: string
  sort: number
  readCode: string
  manageCode: string
  /** 关联业务资源类型（空 = 纯菜单；非空 = 资源驱动菜单，只读档通常无需授） */
  resourceType: string
  /** 页面级按钮（功能码勾选数据源）。 */
  pageButtons: MenuButton[]
  /** 资源级按钮（随数据权限三档，矩阵只读展示）。 */
  resourceButtons: MenuButton[]
}
const serviceRows = ref<MenuGrantRow[]>([])

/** 授权矩阵树节点：与菜单注册表同构（parentId 递归挂载，无服务分组层——
 *  顶级菜单本来就是一级菜单，人为的服务分组节点会造成「权限管理有父菜单
 *  authz」的错觉，kafka/rocketmq 等也会被错误包裹）。节点 key：`menu:{id}`。
 *  空码行 base 为空串：展示「全员可见/随父级可见」标签，无档位控件。 */
interface MenuTreeNode {
  key: string
  /** 菜单行 id。 */
  id?: number
  base: string
  title: string
  path: string
  sort: number
  readCode: string
  manageCode: string
  resourceType?: string
  pageButtons?: MenuButton[]
  resourceButtons?: MenuButton[]
  count?: number
  children?: MenuTreeNode[]
}

/** 矩阵搜索：按菜单名/码/路径过滤（父分支保留有匹配叶子的子树）。 */
const filterText = ref('')
const filteredMenuTree = computed<MenuTreeNode[]>(() => {
  const kw = filterText.value.trim().toLowerCase()
  // 无搜索词：直接返回整树（count 已按子树递归统计，勿重算为一级数）。
  if (!kw) return menuTreeAll.value
  const match = (n: MenuTreeNode) =>
    n.title.toLowerCase().includes(kw) ||
    n.base.toLowerCase().includes(kw) ||
    n.path.toLowerCase().includes(kw)
  // 递归剪枝：命中行或子树有命中则保留；count = 子树命中的菜单行数。
  const prune = (nodes: MenuTreeNode[]): MenuTreeNode[] =>
    nodes
      .filter((n) => match(n) || (n.children ?? []).some((c) => prune([c]).length > 0))
      .map((n) => ({
        ...n,
        count: (match(n) ? 1 : 0) + (n.children ?? []).reduce((s, c) => s + (prune([c])[0]?.count ?? 0), 0),
        children: n.children ? prune(n.children) : undefined,
      }))
  return prune(menuTreeAll.value)
})

const menuTreeAll = computed<MenuTreeNode[]>(() => {
  const rows = serviceRows.value
  const byId = new Map<number, MenuTreeNode>()
  for (const r of rows) byId.set(r.id, { ...r, key: `menu:${r.id}` })
  // 递归挂载二级菜单：children 仅承载子菜单行（按钮在 pageButtons/
  // resourceButtons 内渲染，不入树）。
  const attach = (node: MenuTreeNode): MenuTreeNode => {
    node.children = rows
      .filter((r) => r.parentId === node.id)
      .map((r) => byId.get(r.id))
      .filter((n): n is MenuTreeNode => !!n)
      .sort((a, b) => a.sort - b.sort || (a.id ?? 0) - (b.id ?? 0))
      .map(attach)
    return node
  }
  const countMenu = (node: MenuTreeNode): number =>
    1 + (node.children ?? []).reduce((s, c) => s + countMenu(c), 0)
  // 根 = 顶级菜单（parentId=0），无独立服务分组层：顶级菜单（权限管理/
  // Kafka/RocketMQ/Redis…）本来就是一级菜单、没有父级，直接平铺为树根，
  // 子菜单递归挂载（与侧边栏结构一致）。服务分组节点会造成「权限管理有
  // 父菜单 authz」的错觉，且 kafka/rocketmq 等顶级菜单同样被错误包裹。
  return rows
    .filter((r) => r.parentId === 0)
    .map((r) => byId.get(r.id)!)
    .map(attach)
    .map((n) => ({ ...n, count: countMenu(n) }))
    .sort((a, b) => a.sort - b.sort || (a.id ?? 0) - (b.id ?? 0))
})

// ---------------------------------------------------------------------------
// 级别状态（三档单选：0 无权限 / 1 可查看 / 2 可管理）
// levels 仅承载「直接授予」的编辑态；经部门继承的码以 inheritedLevel 覆盖
// 展示并禁用（锁定态），保存差集永远基于直接授予，语义不混。
// ---------------------------------------------------------------------------
type Level = 0 | 1 | 2
const levels = ref<Record<string, Level>>({})
/** 动作码勾选态：base → code → checked（与三档并列的行内附加授权）。 */
const actionChecks = ref<Record<string, Record<string, boolean>>>({})
const wildLevel = ref<Level>(0)
/** 通配批量面板折叠态（低频操作默认收起）。 */
const wildcardOpen = ref(false)
/** 保存基线：目标当前直接授予码集合（载入时快照，供 dirty 计算）。 */
const baselineCodes = ref<Set<string>>(new Set())

/** 继承档位：该菜单经部门继承到的最高码（0 = 无继承）。 */
function inheritedLevel(base: string): Level {
  const r = serviceRows.value.find((x) => x.base === base)
  if (!r) return 0
  let lv: Level = 0
  for (const o of overview.value.viaOrgs) {
    if (o.codes.includes(r.manageCode)) return 2
    if (o.codes.includes(r.readCode)) lv = 1
  }
  return lv
}

/** 矩阵展示档位：直接授予与继承取高者（继承存在即锁定）。 */
function effectiveLevel(base: string): Level {
  const direct = levels.value[base] ?? 0
  return Math.max(direct, inheritedLevel(base)) as Level
}

/** 继承锁定提示：说明继承来源与调整路径（本行因继承存在而整体锁定，
 *  无法在继承之上叠加直接授予——如需变更请调整对应部门授权）。 */
function inheritedTip(base: string): string {
  const r = serviceRows.value.find((x) => x.base === base)
  const org = (() => {
    if (!r) return ''
    for (const o of overview.value.viaOrgs) {
      if (o.codes.includes(r.manageCode) || o.codes.includes(r.readCode)) return o.orgName
    }
    return ''
  })()
  return `该菜单已由部门「${org}」授予（继承锁定，成员始终有效）；本行不可在继承之上叠加直接授予，如需调整请修改「${org}」部门授权或先撤销部门授予`
}

/** 档位 → 码：可管理档展开 read+manage 两码，可查看档仅 read。 */
function codesOfLevel(r: MenuGrantRow, lv: Level): string[] {
  if (lv >= 2) return [r.readCode, r.manageCode]
  if (lv === 1) return [r.readCode]
  return []
}

/** 当前编辑态的目标码集合（含通配；保存差集与 dirty 计数的单一事实源）。
 *  骨架行（base 空）无授予语义，跳过。 */
function wantedCodes(): Set<string> {
  const s = new Set<string>()
  for (const r of serviceRows.value) {
    if (!r.base) continue
    for (const c of codesOfLevel(r, levels.value[r.base] ?? 0)) s.add(c)
    for (const c of pageActionCodes(r)) {
      if (actionChecks.value[r.base]?.[c]) s.add(c)
    }
  }
  if (wildLevel.value >= 1) s.add('*:read')
  if (wildLevel.value >= 2) s.add('*:manage')
  return s
}

/** 矩阵可表达的码全集（各行 read/manage + 页面级功能码 + 通配码）：dirty 与
 *  保存差集只在此集合内计算。矩阵外的存量码（如超管自举通配 *:*——菜单按
 *  base='*' 跳过，矩阵没有对应勾选）编辑不影响、保存原样保留：否则管理员
 *  在自身矩阵上做任意修改并保存，差集都会把自举通配码撤销掉。 */
const expressibleCodes = computed(() => {
  const s = new Set<string>(['*:read', '*:manage'])
  for (const r of serviceRows.value) {
    if (!r.base) continue
    s.add(r.readCode)
    s.add(r.manageCode)
    for (const c of pageActionCodes(r)) s.add(c)
  }
  return s
})

/** 高等级包含低等级：码集合里的 X:manage 视作同时持有 X:read（与三档勾选
 *  的展开语义 codesOfLevel 对齐）。否则"只授了 manage 元组、未授 read 元组"
 *  的存量数据（如服务账号 svc-redis 仅持 redis:manage）在基线与目标间恒差
 *  一个 read——切到该对象即误报未保存，保存还会补授一个无意图的码。 */
function normalizeLevelPairs(s: Set<string>): Set<string> {
  const out = new Set(s)
  for (const c of s) {
    if (c.endsWith(':manage')) out.add(c.slice(0, -':manage'.length) + ':read')
  }
  return out
}

/** 授权差集（对称差的单一事实源，pendingCount 与 save 共用——提示数与实际
 *  保存动作永远一致）：两侧码集合先做蕴含归一，再在矩阵可表达集合内计算；
 *  矩阵外存量码（超管自举通配 *:* 等，矩阵无对应勾选）原样保留，既不授予
 *  也不撤销。 */
function grantDiff(): { toGrant: string[]; toRevoke: string[] } {
  const wanted = normalizeLevelPairs(wantedCodes())
  const baseline = normalizeLevelPairs(baselineCodes.value)
  const toGrant = [...wanted].filter((c) => !baseline.has(c) && expressibleCodes.value.has(c))
  const toRevoke = [...baseline].filter((c) => !wanted.has(c) && expressibleCodes.value.has(c))
  return { toGrant, toRevoke }
}

/** 待保存变更数（授权差集的条数）。 */
const pendingCount = computed(() => {
  const { toGrant, toRevoke } = grantDiff()
  return toGrant.length + toRevoke.length
})

/** 按当前直接授予码刷新档位与基线（骨架行 base 空串，无档位状态）。 */
function refreshLevels(codes: string[]) {
  const set = new Set(codes)
  const next: Record<string, Level> = {}
  const nextActions: Record<string, Record<string, boolean>> = {}
  for (const r of serviceRows.value) {
    if (!r.base) continue
    next[r.base] = (set.has(r.manageCode) ? 2 : set.has(r.readCode) ? 1 : 0) as Level
    const acts: Record<string, boolean> = {}
    for (const c of pageActionCodes(r)) acts[c] = set.has(c)
    nextActions[r.base] = acts
  }
  levels.value = next
  actionChecks.value = nextActions
  wildLevel.value = (set.has('*:manage') ? 2 : set.has('*:read') ? 1 : 0) as Level
  baselineCodes.value = new Set(codes)
}

function onLevelChange(base: string, v: Level) {
  if (inheritedLevel(base) > 0) return // 锁定行不可编辑
  levels.value[base] = v
}

function onWildLevelChange(v: Level) {
  wildLevel.value = v
}

/** 动作码是否经部门继承（锁定展示，调整需改部门授权）。 */
function actionInherited(code: string): boolean {
  return overview.value.viaOrgs.some((o) => o.codes.includes(code))
}

/** 动作码继承来源部门名（展示用；无来源返回空串）。 */
function inheritedOrgNameOf(code: string): string {
  return overview.value.viaOrgs.find((o) => o.codes.includes(code))?.orgName ?? ''
}

/** 动作码勾选变化（继承锁定行忽略）。 */
function onActionChange(base: string, code: string, v: boolean) {
  if (actionInherited(code)) return
  actionChecks.value[base] ??= {}
  actionChecks.value[base][code] = v
}

/** 动作码的人话短名（去掉服务段前缀，如 kafka-topics:export → export）。 */
function actionShortName(code: string): string {
  const i = code.indexOf(':')
  return i > 0 ? code.slice(i + 1) : code
}

/** 菜单行的功能码全集（去重保序）：页面级按钮行的授权码（按钮实体化后
 *  唯一来源，旧 actionCodes 字符串目录已废弃移除）。矩阵勾选遍历此集合。 */
function pageActionCodes(r: { pageButtons?: MenuButton[] }): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const b of r.pageButtons ?? []) if (!seen.has(b.permissionCode)) { seen.add(b.permissionCode); out.push(b.permissionCode) }
  return out
}

/** 功能码勾选展示名：按钮行用按钮名（如 新建 Topic），旧目录码退回短名。 */
function actionLabel(code: string, r: { pageButtons?: MenuButton[] }): string {
  const b = (r.pageButtons ?? []).find((x) => x.permissionCode === code)
  return b ? b.title : actionShortName(code)
}

/** 动作码 tooltip：只显示继承来源（无部门继承返回空串——tooltip 禁用；
 *  按钮与三档关系的完整说明看矩阵底部图例，不逐格重复）。 */
function actionTip(code: string, label: string): string {
  const org = inheritedOrgNameOf(code)
  return org ? `当前经部门「${org}」授予（锁定，如需调整请到「部门」目标下修改）` : ''
}

/** 资源级按钮 tooltip 语义说明（chip 悬停）。 */
function resourceBtnTip(rb: MenuButton): string {
  return `${rb.title}：随该菜单数据权限档位自动生效，无需单独勾选`
}

/** 有未保存更改时切换授权对象/级别前确认（丢失编辑防线）。 */
async function confirmIfDirty(): Promise<boolean> {
  if (pendingCount.value === 0) return true
  try {
    await ElMessageBox.confirm(
      `当前有 ${pendingCount.value} 处未保存的授权更改，切换后将丢弃。确定切换？`,
      '未保存的更改',
      { type: 'warning', confirmButtonText: '丢弃并切换', cancelButtonText: '留在本页' },
    )
    return true
  } catch {
    return false
  }
}

async function loadTargets() {
  loadingTarget.value = true
  try {
    if (targetType.value === 'user') {
      if (!userId.value) return
      overview.value = await permissionOverview(userId.value)
      refreshLevels(overview.value.direct)
    } else {
      if (!orgId.value) return
      overview.value = { direct: [], viaOrgs: [], roles: [] }
      const direct = await listPermissionGrants('org', orgId.value)
      orgCodes.value = direct.codes
      // 成员有效权限来源：沿父链逐级拉直接授予（层级浅，顺序请求足够；
      // orgs 已随下拉加载，链路解析可用）。
      const byId = new Map(orgTreePickerRef.value?.orgs.map((o) => [o.id, o]) ?? [])
      const ancestors: { orgId: string; orgName: string; codes: string[] }[] = []
      let cur = byId.get(orgId.value)?.parentId
      for (let guard = 0; cur && guard < 20; guard++) {
        const parent = byId.get(cur)
        if (!parent) break
        const resp = await listPermissionGrants('org', cur)
        ancestors.push({ orgId: cur, orgName: parent.name, codes: resp.codes })
        cur = parent.parentId
      }
      orgOverview.value = { direct: direct.codes, ancestors }
      refreshLevels(direct.codes)
    }
  } catch (err) {
    handleApiError(err)
  } finally {
    loadingTarget.value = false
  }
}

/** 左树选中（部门/用户合一）：有未保存更改先确认（拒绝则回滚树高亮），
 *  确认后切换目标并加载其授权。 */
async function onTreeSelect(payload: { type: 'user' | 'org'; id: string }) {
  if (!(await confirmIfDirty())) {
    orgTreePickerRef.value?.setCurrentKey(currentKey.value)
    return
  }
  if (payload.type === 'user') {
    targetType.value = 'user'
    userId.value = payload.id
  } else {
    targetType.value = 'org'
    orgId.value = payload.id
  }
  void loadTargets()
}

// ---------------------------------------------------------------------------
// 菜单反向视图（某菜单的直接授权对象）
// ---------------------------------------------------------------------------
const subjectDialog = ref<{ visible: boolean; menuTitle: string; readCode: string; manageCode: string }>({
  visible: false,
  menuTitle: '',
  readCode: '',
  manageCode: '',
})
function openSubjects(base: string, title: string) {
  const r = serviceRows.value.find((x) => x.base === base)
  if (!r) return
  subjectDialog.value = { visible: true, menuTitle: title, readCode: r.readCode, manageCode: r.manageCode }
}

async function save() {
  if (!canManage.value) return
  const target = targetType.value === 'user' ? userId.value : orgId.value
  if (!target) {
    ElMessage.warning('请先选择授权对象')
    return
  }
  // 差集与 pendingCount 同源（grantDiff）：蕴含归一 + 矩阵外存量码保留。
  const { toGrant, toRevoke } = grantDiff()

  try {
    if (toGrant.length) await grantPermissions({ targetType: targetType.value, targetId: target, permissions: toGrant })
    let revoked = 0
    for (const c of toRevoke) {
      await revokePermission({ targetType: targetType.value, targetId: target, permission: c })
      revoked++
    }
    ElMessage.success(`已保存授权：新增 ${toGrant.length} 项${revoked ? `、移除 ${revoked} 项` : ''}，即时生效`)
    await loadTargets()
  } catch (err) {
    handleApiError(err)
  }
}

/** 重置编辑态回基线（丢弃未保存更改）。 */
function resetEdits() {
  refreshLevels([...baselineCodes.value])
}

onMounted(async () => {
  await useSession().ready() // iframe 握手竞态门控
  void loadCatalog()
  // 部门数据由 OrgTreeSelect 组件内部懒加载（onMounted 门控后拉取 /admin/orgs），
  // 父链解析经 orgTreeSelectRef.orgs 读取——无需本页重复拉取。
})

async function loadCatalog() {
  try {
    const resp = await listMenus()
    // 按钮行（type=button）按父菜单行 id 分组：页面级（relation 空）→ 功能码
    // 勾选；资源级（relation 非空）→ 随数据权限三档、矩阵只读展示。
    const buttonsByMenu = new Map<number, MenuButton[]>()
    for (const m of resp.items) {
      if (m.type !== 'button' || !m.parentId) continue
      const list = buttonsByMenu.get(m.parentId) ?? []
      list.push({ permissionCode: m.permissionCode, title: m.title, relation: m.relation ?? '', resourceType: m.resourceType ?? '' })
      buttonsByMenu.set(m.parentId, list)
    }
    // 每个启用的菜单一行（含空码行）：readCode = 菜单行权限码，manageCode =
    // {服务段}:manage（页面写操作同构）。空码行分两类处理：
    //   - 挂有按钮行：readCode 向上继承首个非空码父级（按钮码才是授予对象，
    //     如 RocketMQ Topic 管理空码 + 新建按钮；可见性继承同侧边栏语义）；
    //   - 无按钮：作为树骨架行保留（base 空串，渲染「全员可见/随父级可见」
    //     标签、无档位
    //     控件）。骨架行不能跳过——否则挂在它下面的子菜单成孤儿不展示
    //    （如 NuJob 根行空码会丢「作业列表」）。
    const rows: MenuGrantRow[] = []
    const codeById = new Map<number, string>()
    for (const m of resp.items) codeById.set(m.id, m.permissionCode || '')
    const inheritedCode = (m: (typeof resp.items)[number]): string => {
      let cur: (typeof resp.items)[number] | undefined = m
      const seen = new Set<number>()
      while (cur) {
        if (seen.has(cur.id)) return ''
        seen.add(cur.id)
        const code = codeById.get(cur.id) || ''
        if (code) return code
        cur = resp.items.find((x) => x.id === cur!.parentId)
      }
      return ''
    }
    for (const m of resp.items) {
      if (!m.enabled || m.type === 'button') continue
      const btns = buttonsByMenu.get(m.id) ?? []
      let readCode = m.permissionCode || ''
      if (!readCode && btns.length) readCode = inheritedCode(m)
      const base = readCode ? readCode.split(':')[0] : ''
      if (base === '*') continue
      rows.push({
        id: m.id,
        parentId: m.parentId ?? 0,
        base,
        service: m.path.split('/')[1] || '',
        title: m.title,
        path: m.path,
        sort: m.sort,
        readCode,
        manageCode: base ? base + ':manage' : '',
        resourceType: m.resourceType ?? '',
        pageButtons: btns.filter((b) => !b.relation),
        resourceButtons: btns.filter((b) => !!b.relation),
      })
    }
    serviceRows.value = rows.sort(
      (a, b) => a.sort - b.sort || a.service.localeCompare(b.service),
    )
    refreshLevels([...baselineCodes.value])
  } catch (err) {
    handleApiError(err)
  }
}
</script>

<template>
  <div class="page">
    <h2 class="page-title">人员授权</h2>

    <div class="layout">
      <!-- 左栏：组织架构树（部门 + 用户合一，单选定位授权对象） -->
      <aside class="left-pane">
        <div class="left-head">
          <span class="left-title">组织架构</span>
          <el-button link type="primary" size="small" @click="previewVisible = true">模拟用户视角</el-button>
        </div>
        <OrgTreePicker
          ref="orgTreePickerRef"
          :model-value="currentKey"
          @select="onTreeSelect"
        />
      </aside>

      <!-- 右栏：授权对象工作区 -->
      <main class="right-pane">
        <!-- 对象摘要卡 -->
        <section class="obj-card">
          <el-tag :type="targetType === 'user' ? 'primary' : 'warning'" size="small" class="obj-type">
            {{ targetType === 'user' ? '用户' : '部门' }}
          </el-tag>
          <div class="obj-meta">
            <div class="obj-name">{{ objectName }}</div>
            <div class="obj-sub">
              <template v-if="targetSelected">
                直接授权 {{ directCount }} 项<template v-if="inheritedCount"> · 经组织继承 {{ inheritedCount }} 项</template>
              </template>
              <template v-else>在左侧组织架构中选中用户或部门后，在此配置其菜单权限</template>
            </div>
          </div>
        </section>

    <!-- 当前有效权限总览 -->
    <section class="sec" v-loading="loadingTarget">
      <h3 class="sec-title">当前有效权限</h3>
      <div v-if="targetType === 'user' && userId" class="overview">
        <div class="ov-block">
          <div class="ov-label">直接授予</div>
          <div class="ov-codes">
            <template v-if="!isOverviewExpanded('user-direct')">
              <el-tag v-for="c in overview.direct.slice(0, OV_PREVIEW_LIMIT)" :key="c" :type="isWildcardCode(c) ? 'danger' : undefined" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button v-if="overview.direct.length > OV_PREVIEW_LIMIT" link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded('user-direct')">
                还有 {{ overview.direct.length - OV_PREVIEW_LIMIT }} 项 ▾
              </el-button>
            </template>
            <template v-else>
              <el-tag v-for="c in overview.direct" :key="c" :type="isWildcardCode(c) ? 'danger' : undefined" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded('user-direct')">收起 ▴</el-button>
            </template>
            <span v-if="!overview.direct.length" class="empty">无</span>
          </div>
          <!-- 通配码说明：管理员从码串无法理解其含义，逐码附一句口径，
               解释"为什么持有它之后其余菜单码全部多余"。 -->
          <div v-if="wildcardCodes.length" class="wildcard-note">
            <div v-for="c in wildcardCodes" :key="c" class="wildcard-line">
              <el-tag size="small" type="danger" class="code-tag">{{ c }}</el-tag>
              <span>{{ wildcardHint(c) }}</span>
            </div>
          </div>
        </div>
        <div class="ov-block" v-for="o in overview.viaOrgs" :key="o.orgId">
          <div class="ov-label">经部门「{{ o.orgName }}」继承</div>
          <div class="ov-codes">
            <template v-if="!isOverviewExpanded(`via-${o.orgId}`)">
              <el-tag v-for="c in o.codes.slice(0, OV_PREVIEW_LIMIT)" :key="c" type="warning" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button v-if="o.codes.length > OV_PREVIEW_LIMIT" link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded(`via-${o.orgId}`)">
                还有 {{ o.codes.length - OV_PREVIEW_LIMIT }} 项 ▾
              </el-button>
            </template>
            <template v-else>
              <el-tag v-for="c in o.codes" :key="c" type="warning" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded(`via-${o.orgId}`)">收起 ▴</el-button>
            </template>
            <span v-if="!o.codes.length" class="empty">无</span>
          </div>
        </div>
        <div class="ov-block" v-if="overview.roles.length">
          <div class="ov-label">过渡期角色（仅展示，不再参与权限派生）</div>
          <div class="ov-codes">
            <el-tag v-for="r in overview.roles" :key="r" type="info" class="code-tag">{{ r }}</el-tag>
          </div>
        </div>
      </div>
      <div v-else-if="targetType === 'org' && orgId" class="overview">
        <div class="ov-block">
          <div class="ov-label">本部门直接授予</div>
          <div class="ov-codes">
            <template v-if="!isOverviewExpanded('org-direct')">
              <el-tag v-for="c in orgOverview.direct.slice(0, OV_PREVIEW_LIMIT)" :key="c" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button v-if="orgOverview.direct.length > OV_PREVIEW_LIMIT" link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded('org-direct')">
                还有 {{ orgOverview.direct.length - OV_PREVIEW_LIMIT }} 项 ▾
              </el-button>
            </template>
            <template v-else>
              <el-tag v-for="c in orgOverview.direct" :key="c" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded('org-direct')">收起 ▴</el-button>
            </template>
            <span v-if="!orgOverview.direct.length" class="empty">无</span>
          </div>
        </div>
        <div class="ov-block" v-for="a in orgOverview.ancestors" :key="a.orgId">
          <div class="ov-label">经上级「{{ a.orgName }}」继承</div>
          <div class="ov-codes">
            <template v-if="!isOverviewExpanded(`anc-${a.orgId}`)">
              <el-tag v-for="c in a.codes.slice(0, OV_PREVIEW_LIMIT)" :key="c" type="warning" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button v-if="a.codes.length > OV_PREVIEW_LIMIT" link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded(`anc-${a.orgId}`)">
                还有 {{ a.codes.length - OV_PREVIEW_LIMIT }} 项 ▾
              </el-button>
            </template>
            <template v-else>
              <el-tag v-for="c in a.codes" :key="c" type="warning" class="code-tag">{{ codeDisplayName(c) }}</el-tag>
              <el-button link type="primary" size="small" class="ov-more" @click="toggleOverviewExpanded(`anc-${a.orgId}`)">收起 ▴</el-button>
            </template>
            <span v-if="!a.codes.length" class="empty">无</span>
          </div>
        </div>
        <p class="ov-note">以上为该部门成员的实际有效权限；矩阵编辑的仅是「本部门直接授予」，继承部分请在对应上级部门页签修改。</p>
      </div>
      <span v-else class="empty">先在上方选择授权对象</span>
    </section>

    <!-- 菜单权限矩阵 -->
    <section class="sec" v-if="canManage">
      <h3 class="sec-title">菜单权限</h3>
      <div class="matrix-head">
        <el-input
          v-model="filterText"
          placeholder="搜索菜单名称 / 权限码 / 路径"
          clearable
          class="matrix-search"
        />
        <span class="matrix-hint">可查看 = 页面入口可见 + 查看列表数据；可管理 = 兜底全权（新建/编辑/删除/数据权限等全部写操作）；按钮权限独立于三档，给仅查看用户补特定按钮（仅可查看档展示；无权限页面不可见、可管理档已包含全部按钮，均自动隐藏）；灰色锁定为部门继承。悬停 ？ 图标看各档位具体含义。</span>
      </div>

      <!-- 批量设置（折叠）：通配「全部菜单」是低频的一把梭操作，收进折叠
           面板避免与逐菜单三档抢语义位；展开状态保留编辑能力。 -->
      <div class="wildcard-toggle" @click="wildcardOpen = !wildcardOpen">
        <span>批量设置全部菜单级别</span>
        <span class="wildcard-arrow" :class="{ open: wildcardOpen }">▾</span>
      </div>
      <div v-if="wildcardOpen" class="wildcard-row">
        <div class="wildcard-title">全部菜单</div>
        <div class="wildcard-desc">一次设置所有菜单的级别</div>
        <div class="level-group">
          <el-radio-group
            :model-value="wildLevel"
            size="small"
            @update:model-value="(v: any) => onWildLevelChange(v as Level)"
          >
            <el-radio-button :value="0">无权限</el-radio-button>
            <el-radio-button :value="1">可查看</el-radio-button>
            <el-radio-button :value="2">可管理</el-radio-button>
          </el-radio-group>
          <el-popover
            placement="top"
            :width="260"
            trigger="hover"
            popper-class="perm-popover"
          >
            <template #reference>
              <el-icon class="level-info"><InfoFilled /></el-icon>
            </template>
            <div class="perm-popover-body">
              <p><b>可查看</b>：所有菜单入口可见 + 查看列表数据。<span class="muted">不能新建/编辑/删除。</span></p>
              <p><b>可管理</b>：所有菜单 + 兜底全权。<span class="muted">全部写操作 + 数据权限管理。</span></p>
              <p class="muted">⚠️ 会覆盖下方逐菜单独自设的级别。</p>
            </div>
          </el-popover>
        </div>
      </div>

      <div class="tree-wrap" v-loading="loadingTarget">
        <el-tree
          :data="filteredMenuTree"
          node-key="key"
          default-expand-all
          :expand-on-click-node="false"
          :props="{ children: 'children' }"
          class="menu-tree"
          empty-text="没有匹配的菜单"
        >
          <template #default="{ data }">
            <!-- 菜单节点（树与注册表同构，无服务分组层） -->
            <div class="menu-node">
              <span
                class="menu-title"
                :title="data.base ? `${data.title} · ${data.base}` : `${data.title}（${data.parentId ? '随父级可见' : '全员可见'}）`"
                >{{ data.title }}</span>
              <!-- 空码行可见性分两种：顶级空码行 = 真·全员可见；非顶级空码
                   子行继承父级可见性（父级持码/资源解锁才对用户下发，见
                   memenuslogic 第二遍组装规则），并非全员可见。 -->
              <span
                v-if="!data.base"
                class="menu-res-tag"
                :title="data.parentId
                  ? '该菜单未配置权限码：可见性继承父菜单——用户能看到父级（持父级码或经资源关系解锁）才能看到本页，父级不可见则本页不下发'
                  : '该菜单未配置权限码且为顶级行：全员可见，无需授权'"
                >{{ data.parentId ? '随父级可见' : '全员可见' }}</span
              >
              <span
                v-if="data.resourceType"
                class="menu-res-tag"
                :title="`该菜单按数据权限控制可见性：用户对该菜单下任一资源实例（如某个 Topic）有查看/编辑权时，菜单入口自动出现。一个菜单可关联多个资源实例——无需单独授「可查看」`"
              >按数据权限自动可见</span>
              <span class="menu-chip">{{ data.path }}</span>
              <!-- 按钮权限入口：跟菜单信息在一起（与右侧「授权对象」分开），
                   仅页面级按钮才有交互价值；资源级按钮随数据权限档位自动生效，不在矩阵展示。
                   仅可查看档（effectiveLevel=1）展示按钮勾选：无权限档没有 read 码、页面入口
                   不可见，按钮码无意义；可管理档 manage 码在后端守卫兜底全部全局操作
                   （requireCodeOrManage：动作码或管理码任一），勾选无权限增量。 -->
              <el-button
                v-if="pageActionCodes(data).length && effectiveLevel(data.base) === 1"
                link type="primary" size="small" class="btn-toggle"
                :class="{ open: isBtnRowExpanded(data.base) }"
                @click.stop="toggleBtnRow(data.base)"
              >
                {{ isBtnRowExpanded(data.base) ? '收起按钮 ▴' : `${pageActionCodes(data).length} 个按钮权限 ▾` }}
              </el-button>
              <!-- 档位控件仅码行渲染（骨架行全员可见，无授予语义） -->
              <span v-if="data.base" class="level-group">
                <el-tooltip
                  :disabled="inheritedLevel(data.base) === 0"
                  :content="inheritedTip(data.base)"
                  placement="top"
                >
                  <span>
                    <el-radio-group
                      :model-value="effectiveLevel(data.base)"
                      size="small"
                      :disabled="inheritedLevel(data.base) > 0"
                      @update:model-value="(v: any) => onLevelChange(data.base, v as Level)"
                    >
                      <el-radio-button :value="0">无权限</el-radio-button>
                      <el-radio-button :value="1">可查看</el-radio-button>
                      <el-radio-button :value="2">可管理</el-radio-button>
                    </el-radio-group>
                  </span>
                </el-tooltip>
                <!-- 档位语义直显：选中档位时旁边直接显示说明，不依赖 hover info popover -->
                <span v-if="effectiveLevel(data.base) === 1" class="level-desc">页面入口可见 + 查看列表</span>
                <span v-else-if="effectiveLevel(data.base) === 2" class="level-desc">全部写操作 + 数据权限管理</span>
                <!-- 档位详细说明 popover：完整三档语义 + 按钮权限关系 -->
                <el-popover
                  placement="top"
                  :width="260"
                  trigger="hover"
                  popper-class="perm-popover"
                >
                  <template #reference>
                    <el-icon class="level-info"><InfoFilled /></el-icon>
                  </template>
                  <div class="perm-popover-body">
                    <p><b>可查看</b>：页面入口可见 + 查看列表数据。<span class="muted">不能新建/编辑/删除。</span></p>
                    <p><b>可管理</b>：上述全部 + <b>兜底全权</b>。<span class="muted">新建/编辑/删除/冻结/重置等全部写操作 + 数据权限管理。</span></p>
                    <p v-if="pageActionCodes(data).length"><b>按钮权限</b>：独立于三档的功能码，给仅查看用户补特定按钮。<span class="muted">仅可查看档展示；无权限页面不可见、可管理档已包含全部按钮（后端守卫 manage 兜底），均自动隐藏。</span></p>
                  </div>
                </el-popover>
                <el-button link type="primary" size="small" class="subj-btn" @click="openSubjects(data.base, data.title)">
                  授权对象
                </el-button>
              </span>
              <!-- 展开后才渲染 checkbox 区：避免一屏被淹没，按需下钻；
                   仅可查看档（effectiveLevel=1）渲染——无权限档进不了页面、可管理档 manage
                   兜底，两者勾选均无意义 -->
              <div v-if="pageActionCodes(data).length && isBtnRowExpanded(data.base) && effectiveLevel(data.base) === 1" class="btn-row">
                <el-tooltip
                  v-for="ac in pageActionCodes(data)"
                  :key="ac"
                  :content="actionTip(ac, actionLabel(ac, data))"
                  :disabled="!actionTip(ac, actionLabel(ac, data))"
                  placement="top"
                >
                  <span>
                    <el-checkbox
                      size="small"
                      class="btn-cb"
                      :model-value="!!actionChecks[data.base]?.[ac] || actionInherited(ac)"
                      :disabled="actionInherited(ac)"
                      @change="(v: boolean) => onActionChange(data.base, ac, v)"
                    >{{ actionLabel(ac, data) }}</el-checkbox>
                  </span>
                </el-tooltip>
              </div>
            </div>
          </template>
        </el-tree>
      </div>
    </section>
    <p v-else class="readonly-note">当前账号无「人员授权」管理权限，仅可查看。</p>
      </main>
    </div>

    <!-- 吸底操作条：待保存变更随手可见、随手可存 -->
    <div v-if="canManage" class="save-bar" :class="{ active: pendingCount > 0 }">
      <span class="save-info">
        <template v-if="pendingCount > 0">有 <b>{{ pendingCount }}</b> 处未保存更改</template>
        <template v-else>更改已保存</template>
      </span>
      <div class="save-actions">
        <el-button :disabled="pendingCount === 0" @click="resetEdits">重置</el-button>
        <el-button type="primary" :disabled="pendingCount === 0" :loading="loadingTarget" @click="save">
          保存授权
        </el-button>
      </div>
    </div>

    <!-- 模拟用户视角（只读预览，非授权入口） -->
    <PermissionPreviewDialog v-model:visible="previewVisible" />
    <!-- 菜单授权对象反向视图 -->
    <GrantSubjectsDialog
      v-model:visible="subjectDialog.visible"
      :menu-title="subjectDialog.menuTitle"
      :read-code="subjectDialog.readCode"
      :manage-code="subjectDialog.manageCode"
      :orgs="orgs"
    />
  </div>
</template>

<style scoped>
.page { padding: 16px 16px 88px; }
.page-title { margin: 0 0 12px; font-size: 16px; }
/* 左右布局：左栏组织树固定宽，右栏授权工作区滚动 */
.layout { display: flex; align-items: stretch; gap: 16px; height: calc(100vh - 150px); min-height: 460px; }
.left-pane { width: 280px; flex: none; display: flex; flex-direction: column; gap: 8px; min-height: 0; }
.left-pane :deep(.org-picker) { flex: 1; min-height: 0; }
.left-head { display: flex; align-items: center; justify-content: space-between; flex: none; }
.left-title { font-size: 14px; font-weight: 600; }
.right-pane { flex: 1; min-width: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 16px; padding-right: 4px; }
.obj-card { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-fill-color-light); flex: none; }
.obj-type { flex: none; }
.obj-meta { min-width: 0; }
.obj-name { font-size: 14px; font-weight: 600; line-height: 1.4; }
.obj-sub { font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.5; }
.sec { margin-bottom: 0; }
.sec-title { margin: 0 0 10px; font-size: 15px; color: var(--el-text-color-primary); }
.overview { display: flex; flex-direction: column; gap: 8px; }
.ov-block { display: flex; align-items: baseline; gap: 10px; }
.ov-label { min-width: 130px; font-size: 13px; color: var(--el-text-color-secondary); }
.ov-codes { display: flex; flex-wrap: wrap; gap: 6px; }
.ov-note { margin: 4px 0 0; font-size: 12px; color: var(--el-text-color-secondary); }
/* 通配码说明块：直接授予分组下方逐码一句口径 */
.wildcard-note { margin-top: 6px; padding: 6px 8px; border-radius: 4px; background: var(--el-color-danger-light-9); }
.wildcard-line { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--el-text-color-regular); line-height: 1.8; }
.code-tag { font-family: ui-monospace, 'SF Mono', Menlo, monospace; }
.empty { font-size: 13px; color: var(--el-text-color-secondary); }
/* 「还有 X 项」展开标记：跟 tag 同行，链接样式低饱和不抢眼 */
.ov-more { margin-left: 4px; padding: 0; height: auto; min-height: 0; font-size: 12px; vertical-align: middle; }
.ov-more:hover { text-decoration: none; }
.matrix-head { display: flex; align-items: center; gap: 12px; margin-bottom: 10px; }
.matrix-search { width: 260px; }
.matrix-hint { font-size: 12px; color: var(--el-text-color-secondary); }
.tree-wrap { border: 1px solid var(--el-border-color-lighter); border-radius: 6px; padding: 4px 8px; max-height: 480px; overflow: auto; }
.menu-tree { --el-tree-node-content-height: 40px; }
.menu-tree :deep(.el-tree-node__content) { padding-right: 8px; }
.menu-tree :deep(.el-tree-node__children) { overflow: visible; }
.menu-node { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; width: 100%; min-width: 0; font-size: 13px; }
.menu-title { flex-shrink: 0; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 500; }
.menu-res-tag { flex-shrink: 0; padding: 1px 6px; font-size: 11px; line-height: 16px; color: var(--el-color-warning-dark-2); background: var(--el-color-warning-light-9); border: 1px solid var(--el-color-warning-light-5); border-radius: 4px; white-space: nowrap; cursor: default; }
.menu-chip { flex-shrink: 0; padding: 1px 8px; font-size: 12px; color: var(--el-text-color-regular); background: var(--el-fill-color-light); border: 1px solid var(--el-border-color-lighter); border-radius: 4px; white-space: nowrap; }
.level-group { margin-left: auto; display: flex; align-items: center; gap: 12px; flex-shrink: 0; }
.level-group :deep(.el-radio-group) { --el-border-radius-base: 4px; }
.wildcard-toggle { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--el-text-color-secondary); cursor: pointer; margin: 0 0 8px; user-select: none; }
.wildcard-toggle:hover { color: var(--el-color-primary); }
.wildcard-arrow { display: inline-block; font-size: 11px; transition: transform 0.15s; }
.wildcard-arrow.open { transform: rotate(180deg); }
.wildcard-row { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; padding: 10px 12px; border: 1px solid var(--el-color-primary-light-7); border-radius: 6px; background: var(--el-color-primary-light-9); }
.wildcard-title { font-size: 13px; font-weight: 600; }
.wildcard-desc { font-size: 12px; color: var(--el-text-color-secondary); flex: 1; }
.readonly-note { font-size: 13px; color: var(--el-text-color-secondary); }
.subj-btn { margin-left: 4px; font-size: 12px; }
/* 按钮权限入口：跟菜单信息（menu-chip）在一起，与右侧 level-group 分开；
 *   menu-node gap 已提供间距，level-group margin-left:auto 自动拉开距离 */
.btn-toggle { font-size: 12px; padding: 0; height: auto; min-height: 0; }
.btn-toggle.open { color: var(--el-text-color-secondary); }
/* 档位语义直显：选中档位时 radio 旁边的小字说明，不依赖 hover */
.level-desc { font-size: 12px; color: var(--el-text-color-secondary); white-space: nowrap; }
/* 档位语义 info 图标：hover 弹出 popover 说明只读/管理实际意味着什么 */
.level-info { margin-left: 2px; color: var(--el-text-color-secondary); cursor: help; font-size: 14px; flex: none; }
.level-info:hover { color: var(--el-color-primary); }
.perm-popover-body { font-size: 13px; line-height: 1.6; }
.perm-popover-body p { margin: 0 0 6px; }
.perm-popover-body p:last-child { margin-bottom: 0; }
.perm-popover-body b { color: var(--el-text-color-primary); }
.perm-popover-body .muted { color: var(--el-text-color-secondary); }
/* 展开后的 checkbox 区：去掉浅底色缩进，紧凑堆叠 */
.btn-row { flex-basis: 100%; display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 4px 20px 6px; }
.btn-cb { margin-right: 2px; height: auto; }
.btn-cb :deep(.el-checkbox__label) { font-size: 12px; padding-left: 4px; }
.btn-cb :deep(.el-checkbox__inner) { width: 14px; height: 14px; }
/* 吸底操作条：仅在有权限时渲染；有更改时高亮 */
.save-bar {
  position: fixed;
  left: 50%;
  transform: translateX(-50%);
  bottom: 20px;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 10px 18px;
  border-radius: 8px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  box-shadow: var(--el-box-shadow-light);
}
.save-bar.active { border-color: var(--el-color-primary-light-5); }
.save-info { font-size: 13px; color: var(--el-text-color-secondary); }
.save-bar.active .save-info { color: var(--el-text-color-primary); }
.save-info b { color: var(--el-color-primary); }
.save-actions { display: flex; gap: 8px; }
</style>
