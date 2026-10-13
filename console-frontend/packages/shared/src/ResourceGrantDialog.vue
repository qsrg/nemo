<template>
  <!-- 资源级授权对话框（基础能力层）：任意已注册业务资源的"谁可看/谁可改"
       名单配置组件。传入资源对象（type+id+name）+ 关系集合即可复用。
       设计要点（组织树选择 + 批量 + 继承式等级）：
       - 授权等级单选（viewer/editor/owner…）：高等级隐含低等级（如可管理含
         可编辑与删除），同一对象只需配置一次，避免"可看/可改/可删"逐档
         分别配置的 3 次操作；注意删除与名单管理为 owner 专属（can_delete/
         can_manage_grants 收口 owner，editor 不含删除）；
       - 批量选择：组织架构树（部门 + 用户混合节点）——先定位组织再选人，
         勾选部门 = org:{id}#member 授权（引擎祖先展开，部门含子树全部成员继承），
         勾选用户 = user:{id} 单用户授权；树父子联动（勾父=视觉全选），
         提交时只保留最顶层勾选部门并剔除已被部门覆盖的用户，避免重复元组；
       - 名单聚合：同一对象多条关系显示最高等级，移除即删除该对象全部关系；
       - 管理权由后端按资源类型校验（如 topic → owner/editor/manage 码）；
         manageable=false 时只读展示名单（查看权限语义）；
       - 批量资源模式（resources 非空）：一次对多个同类型资源授予同一批
         目标/等级——元组跨 object 一次 POST（后端 relations 写接口原生
         支持多 object 整批原子），名单展示区隐藏（聚合名单需逐对象查询，
         暂不提供），单资源模式的名单/回显/移除行为不变。 -->
  <el-dialog :model-value="modelValue" :title="title" width="640px" @update:model-value="(v: boolean) => $emit('update:modelValue', v)">
    <!-- 授权等级：继承式单选（档位多于 1 个才显示；单档只展示名单） -->
    <div v-if="availableRelations.length > 1" class="grant-level">
      <span class="grant-level-label">授权等级</span>
      <el-radio-group v-model="level" size="small">
        <el-radio-button v-for="r in availableRelations" :key="r" :value="r">{{ relationLabel(r) }}</el-radio-button>
      </el-radio-group>
    </div>
    <p v-if="levelNote" class="grant-level-note">{{ levelNote }}</p>

    <!-- ① 批量选择区（manageable 才显示）：组织架构树 -->
    <div v-if="manageable" class="grant-pick">
      <div class="grant-pick-head">
        <span>① 选择对象（组织架构树：勾选部门 = 授权其子树全部成员）</span>
        <el-button link type="primary" :disabled="!hasPicked" @click="clearPicked">清空</el-button>
      </div>
      <el-tree
        v-if="!loadingTree"
        ref="treeRef"
        :data="treeData"
        show-checkbox
        node-key="key"
        default-expand-all
        :props="{ label: 'name', children: 'children' }"
        class="grant-org-tree"
        @check="onTreeCheck"
      >
        <template #default="{ data }">
          <span class="grant-node">
            <span>{{ data.name }}</span>
            <span v-if="data.kind === 'org'" class="grant-node-tag">部门</span>
            <span v-else class="grant-node-tag grant-node-tag-user">用户</span>
          </span>
        </template>
      </el-tree>
      <div class="grant-pick-footer">
        <span class="grant-pick-count">
          已选 {{ checkedOrgCount }} 个部门 · {{ checkedUserCount }} 个用户
        </span>
        <el-button type="primary" :disabled="!canBatchAdd" :loading="adding" @click="onBatchAdd">
          批量授予「{{ relationLabel(level) }}」
        </el-button>
      </div>
    </div>

    <!-- ② 已授权名单（单资源模式；按当前选中等级过滤：切档位
         只显示该等级的授权对象）。批量资源模式隐藏名单——聚合名单需逐对象
         查询（GET relations 单 object），展示"覆盖 N/M"语义复杂，暂不提供。 -->
    <template v-if="!isBatch">
      <div class="grant-list-head">② 已授权名单（{{ relationLabel(level) }}）</div>
      <el-table v-loading="loading" :data="aggRows" size="small" empty-text="该等级暂无授予" max-height="220">
        <el-table-column label="类型" width="70">
          <template #default="{ row }">{{ row.targetType === 'org' ? '部门' : '用户' }}</template>
        </el-table-column>
        <el-table-column label="目标" min-width="150">
          <template #default="{ row }">{{ targetName(row) }}</template>
        </el-table-column>
        <el-table-column v-if="manageable" label="操作" width="80">
          <template #default="{ row }">
            <el-button link type="danger" @click="onRemove(row)">移除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 经组织归属继承可见（只读）：org 元组是创建时定的资源归属，
           非授权对象——其成员（含子树）经 member from org 自动获得查看权，
           不在上方名单配置、也不在此移除（改归属请改资源本身）。 -->
      <div v-if="inheritedOrgs.length" class="inherited-box">
        <div class="inherited-head">经组织归属「{{ inheritedOrgs.map(orgName).join('、') }}」继承可见</div>
        <div class="inherited-note">
          该组织（含子树）成员无需在此授权即可查看本资源——他们出现在名单列表里但不在上方授予记录中。
        </div>
      </div>
    </template>
    <!-- 批量资源模式：展示本次授予作用的目标资源清单（只读，替代名单区） -->
    <template v-else>
      <div class="grant-list-head">② 将授予以下 {{ resources.length }} 个资源「{{ relationLabel(level) }}」</div>
      <div class="grant-batch-res">
        <el-tag v-for="r in resources" :key="r.id" size="small" class="grant-batch-tag">{{ r.name }}</el-tag>
      </div>
    </template>
    <p class="grant-note">
      {{ manageable
        ? '勾选部门即授权其子树全部成员（用户自动继承）；高等级包含低等级，变更即时生效。'
        : '此处仅展示名单；配置需要对应资源的管理权限。' }}
    </p>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { ElTree } from 'element-plus'
import { createMicroHttp } from './microHttp'
import { useSession } from './useSession'
import type { PagedResult } from './types'

/** 通用 relations 元组（原始引擎形态：user 为 user:{id} / org:{id}#member）。 */
interface GrantTuple {
  user: string
  relation: string
  object: string
}

/** 解析后的名单行（目标类型/ID + 关系）。 */
interface GrantRow {
  targetType: 'user' | 'org'
  targetId: string
  relation: string
}

/** 组织目录条目（GET /v1/admin/orgs 响应项）。 */
interface OrgOption {
  id: string
  name: string
  parentId: string
}

/** 组织树节点：部门节点（kind=org，children 含子部门与用户）或用户叶子（kind=user）。
 *  key 为复合唯一键（org:{id} / user:{userId}），供 el-tree node-key 使用。 */
interface TreeNode {
  key: string
  /** 实体 id（部门 id / 用户 userId） */
  id: string
  name: string
  kind: 'org' | 'user'
  /** 所属部门 id（仅用户节点有，用于提交时剔除被部门覆盖的用户） */
  orgId?: string
  children?: TreeNode[]
}

/** 组织成员条目（GET /admin/orgs/:id/members 响应项；displayName 为
 *  orgsync 同步的 CAS 权威显示名，未同步为空串）。 */
interface MemberItem {
  orgId: string
  userId: string
  displayName?: string
}

/** 批量资源模式的目标资源（id+name）；非空数组启用批量授予。
 *  （调用方本地定义同构接口传入即可，TS 结构化兼容。） */
interface BatchResource {
  id: string
  name: string
}

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    /** 资源类型（引擎对象类型，如 topic）——须已入后端可写/管理白名单 */
    resourceType: string
    /** 资源 ID（引擎对象 id 段）；批量模式忽略 */
    resourceId: string
    /** 资源展示名（对话框标题用）；批量模式忽略 */
    resourceName: string
    /** 关系集合（如 ['viewer','editor']），单关系时隐藏等级选择只展示名单。
     *  可选——不传时组件按 resourceType 自动拉取该类型注册的可授档位
     *  （GET /v1/me/grant-relations），业务页零硬编码；显式传入优先，用于
     *  覆盖/调试。 */
    relations?: string[]
    /** 当前用户是否可配置（manage 码/资源管理权）；false 为只读查看 */
    manageable?: boolean
    title?: string
    /** 批量资源集合：非空进入批量授予模式（与 resourceId 互斥使用，
     *  由调用方按场景二选一传入） */
    resources?: BatchResource[]
  }>(),
  { manageable: false, title: '', resources: () => [], relations: () => [] },
)
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  /** 批量/单资源授予成功后触发，携带生效的资源数量（调用方做结果提示） */
  (e: 'granted', resourceCount: number): void
}>()

const object = computed(() => `${props.resourceType}:${props.resourceId}`)
/** 批量资源模式：resources 非空时启用（单资源模式名单/回显/移除不受影响）。 */
const isBatch = computed(() => props.resources.length > 0)
/** 本次授予作用的引擎对象集合：批量模式为全部选中资源，单资源模式为单个。 */
const objects = computed<string[]>(() =>
  isBatch.value ? props.resources.map((r) => `${props.resourceType}:${r.id}`) : [object.value],
)

// ---------- 授权等级（继承式单选） ----------
/** 后端按资源类型注册出口派生的可授档位（GET /v1/me/grant-relations）。
 *  业务页显式传 relations 时优先使用 props，否则打开弹窗时自动拉取。 */
const remoteRelations = ref<string[]>([])
const availableRelations = computed<string[]>(() =>
  props.relations.length > 0 ? props.relations : remoteRelations.value,
)
const level = ref('viewer')

/** 单个 can_* 出口及其对应的真实按钮名（后端从菜单/按钮声明反查）。 */
interface GrantActionDetail {
  relation: string
  titles?: string[]
}

/** 每个档位包含的实际操作出口（relation → actionDetails），后端按注册类型派生。 */
const levelActions = ref<Record<string, GrantActionDetail[]>>({})

/** can_* 操作出口的中文释义（兜底：后端未带按钮名时用全局抽象词）。 */
const ACTION_LABELS: Record<string, string> = {
  can_view: '查看资源',
  can_edit: '修改配置',
  can_delete: '删除资源',
  can_share: '共享给他人',
  can_manage_grants: '配置权限名单',
}

/** 拉取资源类型可授档位（登录即可读）；失败回退 viewer 单档并提示——
 *  静默降级会让管理员误以为该类型只有「可查看」档，宁可吵一次。 */
async function fetchRelations(): Promise<void> {
  if (props.relations.length > 0) return
  try {
    const resp = await http.get<{
      relations: string[]
      levels?: { relation: string; actions: string[]; actionDetails?: GrantActionDetail[] }[]
    }>(`/me/grant-relations?resourceType=${encodeURIComponent(props.resourceType)}`)
    remoteRelations.value = resp?.relations?.length ? resp.relations : ['viewer']
    const map: Record<string, GrantActionDetail[]> = {}
    for (const lv of resp?.levels ?? []) {
      // actionDetails 优先；旧后端/兜底时用 actions 空 titles 回退
      map[lv.relation] = lv.actionDetails?.length
        ? lv.actionDetails
        : (lv.actions ?? []).map((a) => ({ relation: a, titles: [] }))
    }
    levelActions.value = map
  } catch (err) {
    remoteRelations.value = ['viewer']
    ElMessage.warning(
      err instanceof Error
        ? `授权档位加载失败，暂仅显示「可查看」档：${err.message}`
        : '授权档位加载失败，暂仅显示「可查看」档',
    )
  }
}

/** 当前档位说明：优先展示后端反查的真实按钮名（"编辑 Topic、冻结 Topic"），
 *  后端没给按钮名时回退到全局抽象词（"修改配置"）。管理员一眼对上页面按钮。 */
const levelNote = computed<string>(() => {
  const details = levelActions.value[level.value]
  if (details?.length) {
    const parts: string[] = []
    for (const d of details) {
      if (d.titles && d.titles.length) parts.push(...d.titles)
      else parts.push(ACTION_LABELS[d.relation] ?? d.relation)
    }
    const labels = parts.filter(Boolean)
    if (labels.length) return `当前「${relationLabel(level.value)}」：${labels.join('、')}`
  }
  return availableRelations.value.length > 1
    ? '高等级包含低等级权限；同一对象只需配置一次，按最高等级授权即可。'
    : ''
})

/** 按当前可用档位重置等级（取最低档）。 */
function resetLevel() {
  level.value = availableRelations.value[0] ?? 'viewer'
}

function relationLabel(r: string): string {
  return r === 'viewer' ? '可查看' : r === 'editor' ? '可编辑' : r === 'owner' ? '可管理' : r === 'sharer' ? '可共享' : r
}

// ---------- 组织架构树（部门 + 用户混合节点，全量加载） ----------
const http = createMicroHttp()
const { ready } = useSession()
const orgs = ref<OrgOption[]>([])
const membersByOrg = ref<Map<string, MemberItem[]>>(new Map())
/** 组织树首载是否就绪：数据拉取完成前不挂载树（保证 default-expand-all 生效）。 */
const loadingTree = ref(true)
/** 用户所属部门映射（全量加载时填充；onTreeCheck 剔除被部门覆盖的用户用）。 */
const userOrgMap = ref<Map<string, string>>(new Map())
const treeRef = ref<InstanceType<typeof ElTree>>()

/** 组织名反查：展示授予目标名用。 */
function orgName(id: string): string {
  return orgs.value.find((o) => o.id === id)?.name ?? id
}

/** 全量树：部门树 + 各部门直接成员（子部门在前、成员在后；成员展示
 *  displayName，CAS 权威，未同步回退 userId）。
 *  成员取自引擎 org:xxx#member 元组（CAS 同步的权威组织归属）。 */
const treeData = computed<TreeNode[]>(() => {
  const byId = new Map<string, OrgOption>()
  for (const o of orgs.value) byId.set(o.id, { ...o })
  const nodeOf = new Map<string, TreeNode>()
  const ensureNode = (org: OrgOption): TreeNode => {
    let n = nodeOf.get(org.id)
    if (!n) {
      n = { key: `org:${org.id}`, id: org.id, name: org.name, kind: 'org', children: [] }
      nodeOf.set(org.id, n)
    }
    return n
  }
  const roots: TreeNode[] = []
  for (const o of orgs.value) {
    const node = ensureNode(o)
    const members = membersByOrg.value.get(o.id) ?? []
    for (const m of members) {
      node.children!.push({
        key: `user:${m.userId}`,
        id: m.userId,
        name: m.displayName ? `${m.displayName}（${m.userId}）` : m.userId,
        kind: 'user',
        orgId: m.orgId,
      })
    }
    const p = o.parentId ? byId.get(o.parentId) : undefined
    if (p) ensureNode(p).children!.push(node)
    else roots.push(node)
  }
  return roots
})

async function loadTree() {
  loadingTree.value = true
  try {
    const resp = await http.get<PagedResult<OrgOption>>('/admin/orgs')
    orgs.value = resp.items
    // 并行拉各部门直接成员（部门数量少；单部门成员失败不阻断整体）
    const ids = resp.items.map((o) => o.id)
    const results = await Promise.all(
      ids.map((id) =>
        http
          .get<{ members: MemberItem[] }>(`/admin/orgs/${encodeURIComponent(id)}/members`)
          .then((r) => r.members ?? [])
          .catch(() => [] as MemberItem[]),
      ),
    )
    const map = new Map<string, MemberItem[]>()
    ids.forEach((id, i) => map.set(id, results[i]))
    membersByOrg.value = map
    const userOrg = new Map<string, string>()
    for (const [orgId, ms] of map) {
      for (const m of ms) userOrg.set(m.userId, m.orgId || orgId)
    }
    userOrgMap.value = userOrg
  } catch (err) {
    ElMessage.error(err instanceof Error ? `加载组织架构失败：${err.message}` : '加载组织架构失败')
  } finally {
    loadingTree.value = false
  }
}

/** 勾选解析：树父子联动（勾父 = 视觉全选子树），提交语义按继承收拢——
 *  - 部门：只保留最顶层勾选（有勾选祖先部门的剔除，避免父+子重复元组）；
 *  - 用户：剔除已被勾选部门覆盖的用户（部门授权含其子树成员）。
 *  部分选择（勾父后取消某子）时父变半选不入列，只发全选节点，语义一致。 */
const checkedOrgIds = ref<string[]>([])
const checkedUserIds = ref<string[]>([])

function onTreeCheck() {
  const keys = (treeRef.value?.getCheckedKeys() ?? []) as string[]
  const orgKeys = keys.filter((k) => k.startsWith('org:')).map((k) => k.slice(4))
  const userKeys = keys.filter((k) => k.startsWith('user:')).map((k) => k.slice(5))

  const parentOf = new Map<string, string>()
  for (const o of orgs.value) if (o.parentId) parentOf.set(o.id, o.parentId)
  const orgIdSet = new Set(orgKeys)
  const hasCheckedAncestor = (id: string): boolean => {
    let p = parentOf.get(id)
    while (p) {
      if (orgIdSet.has(p)) return true
      p = parentOf.get(p)
    }
    return false
  }
  const topOrgs = orgKeys.filter((k) => !hasCheckedAncestor(k))

  // 用户所属部门映射（懒加载时填充，仅含已展开部门的成员；未加载用户
  // 不会出现在 getCheckedKeys，覆盖剔除只对已加载成员生效）
  const userOrg = userOrgMap.value
  const covered = new Set(topOrgs)
  const topUsers = userKeys.filter((uid) => {
    const oid = userOrg.get(uid)
    if (!oid) return true
    let p: string | undefined = oid
    while (p) {
      if (covered.has(p)) return false
      p = parentOf.get(p)
    }
    return true
  })

  checkedOrgIds.value = topOrgs
  checkedUserIds.value = topUsers
}

const checkedOrgCount = computed(() => checkedOrgIds.value.length)
const checkedUserCount = computed(() => checkedUserIds.value.length)
const hasPicked = computed(() => checkedOrgIds.value.length + checkedUserIds.value.length > 0)
const canBatchAdd = computed(() => hasPicked.value)

// ---------- 批量授权 ----------
const adding = ref(false)

async function onBatchAdd() {
  // 目标 × 授权对象笛卡尔展开：一次 POST 携带跨 object 的多条元组
  // （后端 relations 写接口逐条判权、整批原子——任一对象无权则全部不写；
  // 调用方已在打开对话框前按行级管理权过滤，正常路径不会触发整批拒绝）。
  const tuples: GrantTuple[] = objects.value.flatMap((obj) => [
    ...checkedOrgIds.value.map((oid) => ({ user: `org:${oid}#member`, relation: level.value, object: obj })),
    ...checkedUserIds.value.map((uid) => ({ user: `user:${uid}`, relation: level.value, object: obj })),
  ])
  adding.value = true
  try {
    await http.post('/authz/relations', { tuples })
    emit('granted', objects.value.length)
    if (isBatch.value) {
      ElMessage.success(`已为 ${objects.value.length} 个资源授予「${relationLabel(level.value)}」`)
      // 批量授予任务完成即可关闭（无名单区，无需停留在弹窗）
      emit('update:modelValue', false)
    } else {
      ElMessage.success(`已为 ${tuples.length} 个对象授予「${relationLabel(level.value)}」`)
    }
    clearPicked()
    if (!isBatch.value) await load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? `批量授予失败：${err.message}` : '批量授予失败')
  } finally {
    adding.value = false
  }
}

function clearPicked() {
  treeRef.value?.setCheckedKeys([])
  checkedOrgIds.value = []
  checkedUserIds.value = []
}

/** 打开弹窗后把当前等级的已授权对象回显到组织树上，与下方名单对应。
 *  按当前选中等级（level）过滤名单：切"可查看/可编辑"时只回显该等级的已授权
 *  对象——不同等级的授权在树上分开勾选，避免混在一起看不出谁有哪档。
 *  setCheckedKeys 编程式设置不触发 check 事件，故手动调 onTreeCheck 同步计数。
 *  名单里但不在树成员中的对象（外部用户），setCheckedKeys 自动忽略不存在的 key。 */
function syncCheckedToTree() {
  if (!treeRef.value) return
  const keys: string[] = rows.value
    .filter((r) => r.relation === level.value)
    .map((r) => (r.targetType === 'user' ? `user:${r.targetId}` : `org:${r.targetId}`))
  treeRef.value.setCheckedKeys(keys)
  onTreeCheck()
}

// ---------- 名单（加载 + 聚合 + 移除） ----------
const rows = ref<GrantRow[]>([])
const loading = ref(false)

/** 名单行：按当前选中等级过滤（切档位只显示该等级的授权对象）；
 *  同对象同等级去重（理论上不会重复，兜底）。 */
const aggRows = computed(() => {
  const byKey = new Map<string, GrantRow>()
  for (const r of rows.value) {
    if (r.relation !== level.value) continue
    byKey.set(`${r.targetType}:${r.targetId}`, r)
  }
  return [...byKey.values()]
})

/** 经组织归属继承可见的组织 id 列表：rows 里 relation='org' 的元组是
 *  资源创建时定的归属（org:{id} org {obj}），其成员经 member from org
 *  自动获得查看权——单独分组只读展示，不混入授权名单（不是授予记录）。 */
const inheritedOrgs = computed<string[]>(() =>
  rows.value.filter((r) => r.relation === 'org').map((r) => r.targetId),
)

/** 目标展示名：部门经组织目录反查；用户为裸用户名。 */
function targetName(row: GrantRow): string {
  if (row.targetType === 'org') return orgName(row.targetId)
  return row.targetId
}

/** 解析元组 user 字段 → 目标类型/ID（与后端 topicGrantSubjectOf 同构）。 */
function parseUser(user: string): { targetType: 'user' | 'org'; targetId: string } {
  if (user.startsWith('org:')) return { targetType: 'org', targetId: user.slice(4).replace(/#member$/, '') }
  return { targetType: 'user', targetId: user.replace(/^user:/, '') }
}

async function load() {
  // 防御：资源 ID 未就绪（组件常驻挂载、resourceId 为空）时直接返回，
  // 不发起 object=topic: 这种残缺请求（后端校验 "object 须为 type:id 形式"）。
  if (!props.resourceId) return
  loading.value = true
  try {
    const resp = await http.get<{ items: GrantTuple[]; total: number }>(
      `/authz/relations?object=${encodeURIComponent(object.value)}`,
    )
    rows.value = resp.items.map((t) => ({ ...parseUser(t.user), relation: t.relation }))
  } catch (err) {
    ElMessage.error(err instanceof Error ? `加载权限名单失败：${err.message}` : '加载权限名单失败')
  } finally {
    loading.value = false
  }
}

/** 移除 = 取消该对象全部关系（继承语义下显示最高等级，移除即恢复无授权）。 */
async function onRemove(row: GrantRow) {
  const rels = rows.value.filter((r) => r.targetType === row.targetType && r.targetId === row.targetId)
  try {
    for (const r of rels) {
      const user = r.targetType === 'org' ? `org:${r.targetId}#member` : `user:${r.targetId}`
      await http.delete(
        `/authz/relations?object=${encodeURIComponent(object.value)}&user=${encodeURIComponent(user)}&relation=${encodeURIComponent(r.relation)}`,
      )
    }
    ElMessage.success('已移除（对方列表即时消失）')
    await load()
  } catch (err) {
    ElMessage.error(err instanceof Error ? `移除失败：${err.message}` : '移除失败')
  }
}

// 切换授权等级时按新等级重新回显勾选（名单/树已加载，无需重新拉取；
// 批量资源模式无名单来源，不回显——树勾选始终从空开始）
watch(level, () => {
  if (!props.modelValue || isBatch.value || !props.resourceId) return
  syncCheckedToTree()
})

// 加载时机：弹窗可见才拉数据（组件常驻挂载，onMounted 时 resourceId 为空；
// 打开瞬间 resourceId 已由调用方先赋值，watch 双条件保证既不发残缺请求、
// 也不在关闭态空拉。会话门控由 useSession ready 前置完成）。
// 批量资源模式不读单资源名单（resourceId 可能为空），只加载组织树。
watch(
  [() => props.modelValue, () => props.resourceId],
  async ([visible, id]) => {
    if (!visible) {
      // 关闭弹窗清空名单/选择/等级，下次打开重新加载
      rows.value = []
      resetLevel()
      clearPicked()
      return
    }
    // 先拉档位（决定 level 初值），再并行加载名单与组织树
    await fetchRelations()
    resetLevel()
    if (isBatch.value) {
      await loadTree()
      return
    }
    if (!id) return
    // 名单与组织树并行加载；都完成后把名单回显勾选到树上（树渲染需 nextTick 等 treeRef 挂载）
    await Promise.all([load(), loadTree()])
    await nextTick()
    syncCheckedToTree()
  },
  { immediate: true },
)

// 首载会话门控：iframe 形态下 postMessage 会话未到前 token 为空，立刻请求会 401
void ready()
</script>

<style scoped>
.grant-level {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.grant-level-label {
  font-size: 13px;
  color: var(--el-text-color-primary);
  flex-shrink: 0;
}
.grant-level-note {
  margin: 4px 0 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.grant-pick {
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 12px;
}
.grant-pick-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.grant-org-tree {
  max-height: 240px;
  overflow-y: auto;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 6px;
}
.grant-node-tag {
  margin-left: 6px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light);
  border-radius: 4px;
  padding: 0 4px;
  line-height: 16px;
}
.grant-node-tag-user {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.grant-pick-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 8px;
}
.grant-pick-count {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.grant-list-head {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin-bottom: 8px;
}
.grant-note {
  margin-top: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
/* 经组织归属继承可见（只读说明块） */
.inherited-box {
  margin-top: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--el-color-warning-light-9);
  border: 1px solid var(--el-color-warning-light-7);
}
.inherited-head {
  font-size: 12px;
  font-weight: 600;
  color: var(--el-color-warning-dark-2);
}
.inherited-note {
  margin-top: 4px;
  font-size: 12px;
  color: var(--el-text-color-regular);
  line-height: 1.6;
}
/* 批量资源模式：目标资源清单（tag 换行流式排列，超长滚动） */
.grant-batch-res {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-height: 120px;
  overflow-y: auto;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  padding: 8px;
}
.grant-batch-tag {
  max-width: 100%;
}
.grant-batch-tag :deep(.el-tag__content) {
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
