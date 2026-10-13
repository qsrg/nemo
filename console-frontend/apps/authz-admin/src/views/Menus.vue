<template>
  <div class="page">
    <div class="page-head">
      <h3 class="page-title">菜单管理</h3>
      <div class="head-actions">
        <el-input
          v-model="filterText"
          placeholder="搜索标题 / 路由 / 权限码"
          clearable
          class="head-search"
        />
        <el-button v-if="canManage" type="primary" @click="openCreate">新增菜单</el-button>
      </div>
    </div>

    <el-table
      v-loading="loading"
      :data="menuTree"
      row-key="id"
      :tree-props="{ children: 'children' }"
      default-expand-all
      size="default"
      empty-text="注册表为空：点击「新增菜单」创建第一条菜单"
    >
      <el-table-column prop="title" label="标题" min-width="180">
        <template #default="{ row }">
          <div v-if="row.isService" class="svc-node-title">
            <span class="svc-name">{{ row.title }}</span>
            <span class="svc-node-count">{{ row.menuCount }} 个菜单</span>
          </div>
          <span v-else class="row-title">
            <span v-if="row.type === 'button'" class="btn-dot" />
            {{ row.title }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="80">
        <template #default="{ row }">
          <el-tag v-if="!row.isService" :type="row.type === 'button' ? 'primary' : 'info'" size="small">
            {{ row.type === 'button' ? '按钮' : '菜单' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="path" label="路由" min-width="110" />
      <el-table-column prop="permissionCode" label="权限码" min-width="120">
        <template #default="{ row }">
          <span v-if="!row.isService">
            {{
              row.permissionCode ||
              (row.type === 'button'
                ? '（未授权）'
                : row.parentId
                  ? '（跟随父级）'
                  : '（全员可见）')
            }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="授权语义" min-width="150">
        <template #default="{ row }">
          <template v-if="!row.isService && row.type === 'button'">
            <el-tag v-if="row.relation" type="warning" size="small">{{ row.relation }}（资源级）</el-tag>
            <el-tag v-else type="success" size="small">功能码（页面级）</el-tag>
          </template>
          <template v-else-if="!row.isService">
            <span class="muted">—</span>
          </template>
        </template>
      </el-table-column>
      <el-table-column prop="resourceType" label="资源类型" width="110">
        <template #default="{ row }">
          <el-tag v-if="!row.isService && row.resourceType" type="warning" size="small">
            {{ row.resourceType }}
          </el-tag>
          <span v-else-if="!row.isService" class="muted">—</span>
        </template>
      </el-table-column>
      <el-table-column prop="sort" label="排序" width="70" />
      <el-table-column label="启用" width="80">
        <template #default="{ row }">
          <el-tag v-if="!row.isService" :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '启用' : '停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column v-if="canManage" label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <template v-if="!row.isService">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="onDelete(row)">删除</el-button>
          </template>
        </template>
      </el-table-column>
    </el-table>

    <p class="page-note">
      行身份 = 自增 id，父子引用 = parentId（顶级为 0）。按钮不入侧边栏，
      仅作为授权目录：页面级按钮（无实例操作）= 功能码，授权界面「功能权限」勾选；
      资源级按钮（有实例操作）= 资源关系，授权跟随「数据权限」三档。
      菜单权限码可空（空 = 全员可见）；变更在基座下次拉取菜单或重新登录后可见。
    </p>

    <!-- 新增/编辑共用对话框：编辑模式类型/父级不可改（行身份锚点） -->
    <el-dialog v-model="dialogVisible" :title="editing ? (form.type === 'button' ? '编辑按钮' : '编辑菜单') : (form.type === 'button' ? '新增按钮' : '新增菜单')" width="560px">
      <el-form label-width="88px" @submit.prevent="onSubmit">
        <el-form-item label="类型">
          <el-radio-group v-model="form.type" :disabled="editing" @change="onTypeChange">
            <el-radio-button value="menu">菜单</el-radio-button>
            <el-radio-button value="button">按钮</el-radio-button>
          </el-radio-group>
          <span class="form-note">按钮 = 授权实体（挂页面下，可选登记）；页面写完按钮，在此就近声明，授权界面自动出现</span>
        </el-form-item>
        <el-form-item :label="form.type === 'button' ? '所属菜单' : '父级'" :required="form.type === 'button'">
          <el-tree-select
            v-model="form.parentId"
            :data="parentTree"
            :render-after-expand="false"
            default-expand-all
            check-strictly
            node-key="value"
            :props="{ label: 'label', children: 'children', disabled: 'disabled' }"
            :disabled="editing"
            :placeholder="form.type === 'button' ? '展开树选择按钮所属菜单' : '展开树选择父级菜单（不选 = 顶级行）'"
            clearable
            style="width: 100%"
          />
          <span class="form-note" v-if="form.type === 'button'">
            按钮挂在页面（菜单）下，授权界面在该菜单行展示；展开服务分组选择具体菜单行
          </span>
          <span class="form-note" v-else>
            不选 = 顶级行（服务一级菜单，如 Kafka）；选一级菜单作父级 = 创建其子菜单；层级最多三级
          </span>
        </el-form-item>
        <el-form-item label="标题" required>
          <el-input v-model="form.title" maxlength="128" :placeholder="form.type === 'button' ? '按钮名（如 新建 Topic）' : '菜单展示名'" />
        </el-form-item>
        <el-form-item v-if="form.type === 'menu'" label="路由" required>
          <el-input v-model="form.path" maxlength="128" placeholder="/kafka/topics" />
        </el-form-item>
        <el-form-item label="权限标识" :required="form.type === 'button'">
          <el-input
            v-model="form.permissionCode"
            maxlength="128"
            :placeholder="form.type === 'menu' ? '按路由自动填充（如 kafka-topics:read），可清空' : '如 kafka-topics:create'"
          />
          <span v-if="form.type === 'menu'" class="form-note">
            清空后全员可见<template v-if="autoPermCode && !editing">；建议值：<code class="key-preview">{{ autoPermCode }}</code></template>
          </span>
          <span v-else-if="form.relation.trim()" class="form-note">
            授权随数据权限三档自动生效（不读此码）；码仅作按钮动作标识，需填写，如 kafka-topics:edit
          </span>
          <span v-else class="form-note">
            页面级按钮必填（如 kafka-topics:create）；按钮动作分发依赖此码
          </span>
        </el-form-item>
        <el-form-item v-if="form.type === 'menu'" label="图标">
          <el-input v-model="form.icon" maxlength="64" placeholder="Element Plus 图标名（可空 = 沿用服务图标）" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" :max="100000" />
        </el-form-item>
        <el-form-item label="资源类型">
          <el-autocomplete
            v-model="form.resourceType"
            :fetch-suggestions="queryResourceTypes"
            :trigger-on-focus="true"
            clearable
            :disabled="editing && !!form.resourceType"
            maxlength="255"
            placeholder="选择或输入，如 kafka_topic（可空 = 纯菜单/页面级按钮；多个用逗号分隔）"
          />
          <!-- 已有资源类型 = 已与后端鉴权模型绑定（决定菜单入口解锁与授权
               配置挂载），变更需后端同步改造，编辑时锁定防止误改 -->
          <span v-if="editing && form.resourceType" class="form-note">
            已绑定后端鉴权模型（菜单入口解锁、授权配置均按此类型），变更需后端同步改造，编辑时锁定；确有需要可新建菜单后迁移
          </span>
          <!-- 选了父级即展示范围过滤提示；form.parentId 变化触发 computed 重新求值 -->
          <span v-else-if="showScopeHint" class="form-note">
            {{
              resourceTypeScope.length
                ? '已按父级所属服务声明的资源类型过滤推荐（可手输新类型）'
                : '该服务尚未声明资源类型，可直接输入新类型'
            }}
          </span>
        </el-form-item>
        <el-form-item v-if="form.type === 'button'" label="资源关系">
          <!-- 下拉选项由后端白名单接口下发（GET /admin/menus/relations，代码
               常量随版本发布），前端不硬编码副本；可清空 = 页面级按钮。 -->
          <el-select
            v-model="form.relation"
            clearable
            filterable
            placeholder="选择资源关系（可空 = 页面级按钮；非空 = 资源级按钮，授权随数据权限三档）"
            style="width: 100%"
          >
            <el-option v-for="r in relationOptions" :key="r" :label="r" :value="r" />
          </el-select>
          <span class="form-note">清单为引擎出口白名单（can_view / can_edit / can_delete / can_share / can_manage_grants），新增语义随版本发布</span>
        </el-form-item>
        <el-form-item v-if="editing" label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">
          {{ editing ? '保存' : '创建' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { hasPermission, sessionReady } from '@nemo/shared'
import { baseProps, showApiError } from '../api/client'
import {
  createMenu,
  deleteMenu,
  listMenuRelations,
  listMenus,
  listResourceTypes,
  updateMenu,
  type MenuRow,
} from '../api/menus'

/** 写权限：菜单管理仅本菜单 authz-menus:manage 可见（fail-closed）。 */
const permissions = computed(() => baseProps()?.permissions ?? [])
const canManage = computed(() => hasPermission('authz-menus:manage', permissions.value))

const loading = ref(false)
const menus = ref<MenuRow[]>([])

/** 资源级按钮 relation 白名单（后端代码常量下发，前端不硬编码副本）。
 *  加载失败静默留空（编辑态下拉显示存量原始值仍可原样保存），并在
 *  打开对话框时懒重试——瞬时故障自愈，不阻塞菜单管理主流程。 */
const relationOptions = ref<string[]>([])

/** 拉取关系白名单下拉选项（失败不抛出，由调用方决定是否重试）。 */
async function loadRelationOptions() {
  try {
    relationOptions.value = (await listMenuRelations()).items
  } catch (err) {
    showApiError(err)
  }
}

/** 资源类型联想选项（新增菜单/按钮「资源类型」输入的数据源；下拉联想，可自由输入）。 */
const resourceTypeOptions = ref<string[]>([])

/** 拉取资源类型联想选项（失败静默，联想为空仍可手输）。 */
async function loadResourceTypeOptions() {
  try {
    resourceTypeOptions.value = (await listResourceTypes()).items.map((r) => r.name)
  } catch (err) {
    showApiError(err)
  }
}

/** 资源类型联想范围（computed 缓存，避免模板内调用函数求值中断渲染）：
 *  选了父级（菜单行选父菜单、按钮行选所属菜单）即收窄到父级所属服务——
 *  从父菜单沿 parentId 向上追到服务根行（顶级行），收集整棵子树（含按钮
 *  行）声明的资源类型并集。子菜单/按钮都作用于本服务的资源，推荐其他
 *  服务的类型（如在 kafka 页面挂 redis_instance）属于配置错误。服务内尚
 *  无任何声明时返回空列表（仍可手输新类型，如给服务挂第一个资源声明）。
 *  未选父级（顶级行）不收窄——新服务自带新类型，全量联想。父行缺失
 * （异常数据）回退全量。 */
const resourceTypeScope = computed<string[]>(() => {
  if (!form.parentId) return resourceTypeOptions.value
  const byId = new Map(menus.value.map((m) => [m.id, m]))
  if (!byId.has(form.parentId)) return resourceTypeOptions.value
  // 向上追溯服务根：顶级行（parentId=0）为止；沿链防环。
  let rootId = form.parentId
  const seen = new Set<number>([form.parentId])
  for (;;) {
    const cur = byId.get(rootId)
    if (!cur || !cur.parentId || seen.has(cur.parentId)) break
    seen.add(cur.parentId)
    rootId = cur.parentId
  }
  // 服务子树内全部行（菜单 + 按钮）声明的资源类型并集（保序去重）。
  const out: string[] = []
  const seenTypes = new Set<string>()
  const collect = (m: MenuRow) => {
    for (const t of (m.resourceType ?? '').split(',')) {
      const v = t.trim()
      if (v && !seenTypes.has(v)) {
        seenTypes.add(v)
        out.push(v)
      }
    }
  }
  const walk = (id: number) => {
    for (const m of menus.value) {
      if (m.parentId !== id) continue
      collect(m)
      walk(m.id)
    }
  }
  collect(byId.get(rootId)!)
  walk(rootId)
  return out
})

/** 是否显示范围过滤提示：选了父级即展示（菜单行与按钮行同规则；服务未
 *  声明类型时提示可手输，而非沉默空联想）。 */
const showScopeHint = computed<boolean>(() => !!form.parentId)

/** el-autocomplete 联想回调：按输入串前缀过滤（区分大小写前先小写比较）；
 *  按钮行按父菜单声明的资源类型收窄范围（见 resourceTypeScope）。 */
function queryResourceTypes(query: string, cb: (suggestions: { value: string }[]) => void) {
  const q = query.trim().toLowerCase()
  const scope = resourceTypeScope.value
  const hits = q
    ? scope.filter((n) => n.toLowerCase().includes(q))
    : scope
  cb(hits.slice(0, 20).map((value) => ({ value })))
}

/** 树节点：服务（虚拟分组，按路由首段）→ 菜单（真实行）→ 按钮（type=button 行）。
 *  服务分组节点 id 取负数（-1 起），避免与真实行自增 id 冲突（row-key 唯一）。 */
interface MenuTreeNode extends MenuRow {
  isService?: boolean
  menuCount?: number
  children?: MenuTreeNode[]
}

/** 矩阵搜索：按标题/路由/权限码/资源类型过滤（保留命中叶子的祖先链）。 */
const filterText = ref('')

/** 平铺菜单 → 树（纯真实层级）：顶级行（parentId=0/父缺失）平铺为一级菜单
 *  （服务名即一级菜单：Kafka/Redis/权限管理…），子菜单/按钮按 parentId 挂到
 *  父行下。无虚拟服务分组。有搜索词时剪枝：仅保留命中的行及其祖先链。 */
const menuTree = computed<MenuTreeNode[]>(() => {
  const kw = filterText.value.trim().toLowerCase()
  const hit = (m: MenuRow) =>
    !kw ||
    m.title.toLowerCase().includes(kw) ||
    m.path.toLowerCase().includes(kw) ||
    m.permissionCode.toLowerCase().includes(kw) ||
    m.resourceType.toLowerCase().includes(kw) ||
    m.relation.toLowerCase().includes(kw)

  // 菜单行按 id 建索引；按钮行按父行 id 暂存。
  const menuRows = menus.value.filter((m) => m.type !== 'button')
  const byId = new Map<number, MenuTreeNode>()
  for (const m of menuRows) byId.set(m.id, { ...m, children: [] })
  const buttonsByParent = new Map<number, MenuTreeNode[]>()
  for (const m of menus.value) {
    if (m.type !== 'button') continue
    const list = buttonsByParent.get(m.parentId) ?? []
    list.push({ ...m, children: [] })
    buttonsByParent.set(m.parentId, list)
  }

  // 递归挂载：菜单行子节点 = 直接后代菜单行 + 挂本行的按钮行。
  const attach = (node: MenuTreeNode): MenuTreeNode => {
    const subs = menuRows
      .filter((m) => m.parentId === node.id)
      .map((m) => byId.get(m.id))
      .filter((n): n is MenuTreeNode => !!n)
      .sort((a, b) => a.sort - b.sort || a.id - b.id)
      .map(attach)
    const btns = (buttonsByParent.get(node.id) ?? []).sort((a, b) => a.sort - b.sort || a.id - b.id)
    node.children = [...subs, ...btns]
    return node
  }

  // 顶级行（parentId=0 或父行缺失）平铺为一级菜单，子级已递归挂载。
  const roots = menuRows
    .filter((m) => !m.parentId || !byId.has(m.parentId))
    .map((m) => byId.get(m.id)!)
    .map(attach)

  // 搜索剪枝：命中行或子树有命中的分支保留。
  const prune = (nodes: MenuTreeNode[]): MenuTreeNode[] =>
    nodes
      .filter((n) => hit(n) || (n.children ?? []).some((c) => prune([c]).length > 0))
      .map((n) => ({ ...n, children: n.children ? prune(n.children) : undefined }))
  return prune(roots).sort((a, b) => a.title.localeCompare(b.title))
})

async function loadMenus() {
  loading.value = true
  try {
    const resp = await listMenus()
    menus.value = resp.items
  } catch (err) {
    showApiError(err)
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// 新增/编辑对话框
// ---------------------------------------------------------------------------
const dialogVisible = ref(false)
const editing = ref(false)
const submitting = ref(false)
/** 编辑目标行 id（PUT 定位用）。 */
const editingId = ref(0)

const form = reactive({
  /** 行类型：menu（默认）| button（按钮级授权实体）。创建后不可改。 */
  type: 'menu' as 'menu' | 'button',
  /** 父行 id：菜单行 = 父菜单（不选 = 顶级行/一级菜单）；按钮行 = 所属菜单行 id。
   *  不选用 undefined（el-tree-select 显示占位符；0 会被当节点值显示成 "0"），
   *  提交时换算为 0。 */
  parentId: undefined as number | undefined,
  title: '',
  icon: '',
  path: '',
  sort: 100,
  /** 权限标识：菜单行可空（空 = 全员可见）；页面级按钮必填；资源级按钮必填
   *  （仅作前端动作分派标识，授权判定走资源关系三档）。 */
  permissionCode: '',
  resourceType: '',
  /** 资源级按钮的资源关系（空 = 页面级按钮；非空 = 资源级，须关联资源类型）。 */
  relation: '',
  enabled: true,
})

/** 父级选择树（纯真实层级，与主表格同构）：顶级行（parentId=0/父缺失 =
 *  一级菜单/服务，如 Kafka、权限管理）平铺顶层可选作父级，子菜单按 parentId
 *  挂到父行下：
 *  - 菜单行：父级链深度 ≤ 2 的行可选作父级（子行深度 ≤ 3，与后端
 *    menuChainDepth ≤ 3 同口径）；深度 3 的行不可再作父级；
 *  - 按钮行：任意菜单行均可选（按钮可挂任意层，后端无深度限制）。 */
const parentTree = computed(() => {
  type TreeItem = { label: string; value: number; disabled?: boolean; children?: TreeItem[] }
  const isButton = form.type === 'button'
  const menuRows = menus.value.filter((m) => m.type !== 'button' && m.enabled)
  const byId = new Map<number, MenuRow>(menuRows.map((m) => [m.id, m]))

  /** 行深度：顶级行 = 1，逐父链 +1（父行停用/缺失按断链计，不影响展示）。 */
  const depthOf = (m: MenuRow): number => {
    let d = 1
    let cur = m
    while (cur.parentId && byId.has(cur.parentId)) {
      d++
      cur = byId.get(cur.parentId)!
    }
    return d
  }

  const toNode = (m: MenuRow): TreeItem => ({
    label: m.title,
    value: m.id,
    // 菜单类型：深度 ≥ 3 的行不可作父级（子行会超出三级上限）。
    disabled: !isButton && depthOf(m) >= 3,
    children: [],
  })
  const nodeById = new Map<number, TreeItem>(menuRows.map((m) => [m.id, toNode(m)]))

  // 子菜单按 parentId 挂到父行下；顶级行（或父行缺失）平铺顶层。
  for (const m of menuRows) {
    if (m.parentId && nodeById.has(m.parentId)) {
      nodeById.get(m.parentId)!.children!.push(nodeById.get(m.id)!)
    }
  }
  const topNodes = menuRows
    .filter((m) => !m.parentId || !byId.has(m.parentId))
    .map((m) => nodeById.get(m.id)!)

  const sortNodes = (nodes: TreeItem[]): TreeItem[] => {
    nodes.sort((a, b) => a.label.localeCompare(b.label))
    nodes.forEach((n) => sortNodes(n.children ?? []))
    return nodes
  }
  return sortNodes(topNodes)
})

/** 权限标识建议值：菜单行按路由默认填充（/kafka/topics → kafka-topics:read：
 *  去开头 / 、/ 转 -，加 :read 后缀）。用户手改后不再覆盖；可清空 = 全员可见。 */
const lastAutoPermCode = ref('')
const autoPermCode = computed(() => {
  if (form.type !== 'menu') return ''
  const p = form.path.trim().replace(/^\//, '').replace(/\//g, '-').replace(/-+$/, '')
  return p ? `${p}:read` : ''
})
watch(
  () => [form.type, form.path],
  () => {
    if (form.type !== 'menu' || editing.value) return
    const auto = autoPermCode.value
    if (!auto) return
    // 仅当权限码为空或仍等于上次自动值（用户未手改）时跟随更新。
    if (!form.permissionCode || form.permissionCode === lastAutoPermCode.value) {
      form.permissionCode = auto
    }
    lastAutoPermCode.value = auto
  },
)

/** 切换类型：清空父级（菜单/按钮的父级语义不同）。 */
function onTypeChange() {
  form.parentId = undefined
}

function openCreate() {
  editing.value = false
  editingId.value = 0
  Object.assign(form, {
    type: 'menu',
    parentId: undefined,
    title: '',
    icon: '',
    path: '',
    sort: 100,
    permissionCode: '',
    resourceType: '',
    relation: '',
    enabled: true,
  })
  // 白名单下拉选项缺失（首拉失败）时懒重试，避免下拉一直为空。
  if (!relationOptions.value.length) void loadRelationOptions()
  if (!resourceTypeOptions.value.length) void loadResourceTypeOptions()
  dialogVisible.value = true
}

function openEdit(row: MenuRow) {
  editing.value = true
  editingId.value = row.id
  Object.assign(form, {
    type: row.type === 'button' ? 'button' : 'menu',
    // 无父级（0）换算 undefined：el-tree-select 无匹配节点会显示原始值 0。
    parentId: row.parentId || undefined,
    title: row.title,
    icon: row.icon,
    path: row.path,
    sort: row.sort,
    permissionCode: row.permissionCode,
    resourceType: row.resourceType,
    relation: row.relation,
    enabled: row.enabled,
  })
  // 白名单下拉选项缺失（首拉失败）时懒重试；编辑态存量值不在清单内时
  // el-select 原样显示原始值，保存语义不变。
  if (!relationOptions.value.length) void loadRelationOptions()
  if (!resourceTypeOptions.value.length) void loadResourceTypeOptions()
  dialogVisible.value = true
}

async function onSubmit() {
  const parentId = form.parentId ?? 0
  if (!form.title.trim()) {
    ElMessage.warning('标题不能为空')
    return
  }
  if (form.type === 'button') {
    if (!parentId) {
      ElMessage.warning('按钮必须选择所属菜单')
      return
    }
    if (!form.relation.trim() && !form.permissionCode.trim()) {
      ElMessage.warning('页面级按钮必须填写权限标识（如 kafka-topics:create）')
      return
    }
    if (form.relation.trim() && !form.permissionCode.trim()) {
      ElMessage.warning('资源级按钮必须填写权限标识（前端动作分派标识，如 redis:edit）')
      return
    }
    if (form.relation.trim() && !form.resourceType.trim()) {
      ElMessage.warning('资源级按钮必须填写资源类型')
      return
    }
  } else if (!form.path.trim()) {
    ElMessage.warning('路由不能为空（菜单行为导航入口）')
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      await updateMenu(editingId.value, {
        type: form.type,
        title: form.title.trim(),
        icon: form.icon.trim(),
        path: form.path.trim(),
        sort: form.sort,
        permissionCode: form.permissionCode.trim(),
        resourceType: form.resourceType.trim(),
        relation: form.type === 'button' ? form.relation.trim() : '',
        enabled: form.enabled,
      })
      ElMessage.success('已保存')
    } else {
      await createMenu({
        type: form.type,
        parentId,
        title: form.title.trim(),
        icon: form.icon.trim(),
        path: form.path.trim(),
        sort: form.sort,
        permissionCode: form.permissionCode.trim(),
        resourceType: form.resourceType.trim(),
        relation: form.type === 'button' ? form.relation.trim() : '',
      })
      ElMessage.success('已创建')
    }
    dialogVisible.value = false
    await loadMenus()
  } catch (err) {
    // 409 重码 / 400 非法 / 404 未命中：message 语义明确，统一透出。
    showApiError(err)
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: MenuRow) {
  const isBtn = row.type === 'button'
  try {
    await ElMessageBox.confirm(
      isBtn
        ? `确定删除按钮「${row.title}」？授权界面将不再展示该授权点，已授出的功能码授予将一并清理。`
        : `确定删除菜单「${row.title}」？其下按钮行将级联删除，基座侧边栏不再展示该项。`,
      isBtn ? '删除按钮' : '删除菜单',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消
  }
  try {
    await deleteMenu(row.id)
    ElMessage.success('已删除')
    await loadMenus()
  } catch (err) {
    showApiError(err)
  }
}

onMounted(async () => {
  await sessionReady() // iframe 握手竞态门控:token 到位后再发首个请求
  void loadMenus()
  void loadRelationOptions()
  void loadResourceTypeOptions()
})
</script>

<style scoped>
.page {
  padding: 16px;
}
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.head-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}
.head-search {
  width: 240px;
}
.page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.page-note {
  margin-top: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}
.svc-node-title { display: flex; align-items: center; gap: 8px; }
.svc-name { font-weight: 600; font-family: ui-monospace, 'SF Mono', Menlo, monospace; }
.svc-node-count { font-size: 12px; color: var(--el-text-color-secondary); }
.row-title { display: inline-flex; align-items: center; }
.btn-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--el-color-primary);
  margin-right: 6px;
}
.form-note { display: block; font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.5; margin-top: 2px; }
.key-preview { font-family: ui-monospace, 'SF Mono', Menlo, monospace; color: var(--el-color-primary); }
</style>
