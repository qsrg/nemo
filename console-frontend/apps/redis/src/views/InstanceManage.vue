<template>
  <div class="page">
    <header class="page-head">
      <div class="head-left">
        <h3 class="page-title">实例管理</h3>
        <!-- 可见范围说明：本页无归属部门列，可见性 = 引擎关系（owner/被授予） -->
        <el-tooltip content="可见范围 = 您是 owner 或被授予（可查看/可编辑）的实例；持管理码时全部可见" placement="top">
          <span class="demo-mark">授权可见</span>
        </el-tooltip>
      </div>
      <!-- 权限身份胶囊（右对齐）：当前登录者在本页的能力档位，
           与下方按钮显隐同一判定源，所见即所得。 -->
      <span v-if="canManage" class="perm-pill grant">可管理 · 增删改/授权名单</span>
      <span v-else-if="canCreate" class="perm-pill partial">部分功能权 · 新建</span>
      <span v-else class="perm-pill readonly">可查看</span>
    </header>

    <!-- 集群级联：先选集群再出实例（列表恒在具体集群上下文内）。集群=
         基础配置数据（不进权限引擎）：读无守卫（页面可见即可读），新建/
         删除挂 redis:manage 码（管理档维护，与 kafka 集群同口径）。 -->
    <div class="cluster-bar">
      <span class="cluster-label">集群</span>
      <el-select
        v-model="currentClusterId"
        class="cluster-select"
        placeholder="选择集群"
        :loading="clustersLoading"
        @change="onClusterChange"
      >
        <el-option v-for="c in clusters" :key="c.id" :value="c.id" :label="clusterLabel(c)" />
      </el-select>
      <template v-if="canManage">
        <el-button @click="openCreateCluster">新建集群</el-button>
        <el-button v-if="currentClusterId" type="danger" plain @click="onDeleteCluster">删除集群</el-button>
      </template>
      <!-- 集群权限：配置该集群的授权名单（谁能看到此集群——含空集群）。
           入口 = manage 码或当前集群 owner（行级 canManageGrants 徽标驱动，
           与实例行内「权限配置」同一判定源）；经通用 ResourceGrantDialog
           写 redis_cluster viewer 元组。 -->
      <el-button v-if="currentClusterCanManage" link type="primary" @click="openClusterGrants">集群权限</el-button>
      <span class="cluster-hint">实例归属集群：切换集群即刷新该集群下的实例</span>
    </div>

    <div class="toolbar">
      <!-- 全局功能按钮（无实例）：新建=create 动作码或 manage 码——管理档
           天然兜底；归属当前选中集群（未选集群时禁点）。行内资源操作
           （编辑/删除/配置权限）走资源关系，不建码。 -->
      <el-button v-if="canManage || canCreate" type="primary" :disabled="!currentClusterId" @click="openCreate">新建实例</el-button>
      <!-- 批量权限配置：勾选多个实例一次授予（目标集合在打开前按行级
           canManageGrants 过滤，无可管理行时禁点）。页面级操作入口，不新增
           权限码——操作权仍由选中行的 owner/manage 关系驱动（与行内按钮
           同一判定源）。 -->
      <el-button type="primary" plain :disabled="selectedRows.length === 0" @click="openBatchGrants">
        批量权限配置{{ selectedRows.length ? `（${selectedRows.length}）` : '' }}
      </el-button>
      <el-tooltip placement="bottom-start">
        <template #content>
          新建需 redis:create 或 redis:manage 码；<br />
          编辑/删除/配置权限按实例关系（owner/被授予者）判定；<br />
          功能码可在「人员授权」页勾选，名单在行内「权限配置」维护；<br />
          勾选多行可批量授予名单（仅对您有权管理的实例生效）。
        </template>
        <span class="hint-mark">权限说明</span>
      </el-tooltip>
    </div>

    <section class="table-card">
      <el-table ref="tableRef" v-loading="loading" :data="instances" class="instance-table" @selection-change="onSelectionChange">
        <!-- 空态分两态：无集群（引导先建集群）与集群内无实例（可见范围语义） -->
        <template #empty>
          <el-empty :image-size="72" :description="clusters.length ? '该集群暂无实例' : '暂无集群'">
            <p v-if="!clusters.length" class="empty-note">
              {{ canManage ? '先创建集群，再在该集群下新建实例' : '请先联系管理员创建集群' }}
            </p>
            <p v-else class="empty-note">可见范围 = 您是 owner 或被授予的实例（持有管理码时全部可见）</p>
          </el-empty>
        </template>
        <!-- 多选列：批量权限配置的行选择（分页翻页重新拉数据，选中态随之清空） -->
        <el-table-column type="selection" width="42" />
        <el-table-column label="实例名" min-width="150">
          <template #default="{ row }">
            <span class="instance-name">{{ row.name }}</span>
            <!-- owner 徽标：把行内按钮差异（为何别人有编辑/配置权限而我没有）
                 的根因直接可视在行上，悬停见完整解释 -->
            <el-tooltip v-if="row.owner" content="你是该实例的 owner：可编辑、删除并配置权限名单" placement="top">
              <span class="owner-flag">owner</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="addr" label="地址" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <code class="addr-code">{{ row.addr || '—' }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="{ 'desc-empty': !row.description }">{{ row.description || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="createdBy" label="创建人" width="110" />
        <el-table-column label="操作" width="260" class-name="op-col">
          <template #default="{ row }">
            <!-- 行内按钮显隐直接吃行级 canX（后端徽标已并入 manage 码回退，
                 与路由守卫同源，前端不再自行推导）。 -->
            <el-button v-if="row.canEdit" link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="row.canManageGrants" link type="primary" @click="openGrants(row)">权限配置</el-button>
            <el-button v-if="row.canDelete" link type="danger" @click="onDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 分页：服务端分页（纯引擎可见 ID 集合 IN 下推），total 为近似值 -->
    <div class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        background
        @current-change="loadInstances"
        @size-change="onSizeChange"
      />
    </div>

    <!-- 新建/编辑共用对话框（编辑不传 name：对象身份不可改）。新建归属
         当前选中集群（只读展示——集群上下文由顶部选择器决定）。 -->
    <el-dialog v-model="dialogVisible" :title="editing ? '编辑实例' : '新建实例'" width="480px">
      <el-form label-width="104px" @submit.prevent="onSubmit">
        <el-form-item v-if="!editing" label="所属集群">
          <span class="cluster-ref">{{ currentClusterCode }}</span>
        </el-form-item>
        <el-form-item label="实例名" required>
          <el-input v-model="form.name" :disabled="editing" maxlength="128" placeholder="如 cache-order" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.addr" maxlength="255" placeholder="host:port，如 10.0.0.8:6379" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" maxlength="512" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">
          {{ editing ? '保存' : '创建' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 新建集群对话框（redis:manage 持有者）：code 唯一且创建后不可改
         （对象身份不可变，与实例 name 同规则）。 -->
    <el-dialog v-model="clusterDialogVisible" title="新建集群" width="480px">
      <el-form label-width="104px" @submit.prevent="onSubmitCluster">
        <el-form-item label="集群编码" required>
          <el-input v-model="clusterForm.code" maxlength="64" placeholder="如 bj-cache-cluster（唯一，创建后不可改）" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="clusterForm.remark" maxlength="512" placeholder="用途 / 机房等说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="clusterDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="clusterSubmitting" @click="onSubmitCluster">创建</el-button>
      </template>
    </el-dialog>

    <!-- 集群权限对话框：通用资源授权组件（基础能力层）——资源类型
         redis_cluster + viewer 单关系（集群无"编辑"操作，可见即可）。
         名单/添加/移除由组件与后端通用 relations API 承载；manage 码持有者
         或集群 owner 可配置（后端按注册表 manage_code + owner 关系校验）。 -->
    <ResourceGrantDialog
      v-model="clusterGrantVisible"
      resource-type="redis_cluster"
      :resource-id="clusterGrantId"
      :resource-name="clusterGrantName"
      manageable
      :title="`集群权限：${clusterGrantName}`"
    />

    <!-- 权限配置/查看对话框：通用资源授权组件（基础能力层）——资源类型
         redis_instance；授权档位由后端按该类型注册的 can_* 出口自动派生
         （viewer/editor/owner…），业务页零硬编码；manage 码持有者或 owner 可
         配置，只读用户只读查看（后端 /v1/authz/relations 按资源类型校验
         管理权，前端 manageable 仅控制交互入口）。
         batchResources 非空 = 批量模式（行内勾选的实例集合，一次跨 object
         授予）；单资源模式（行内按钮）照旧传 resource-id/resource-name。 -->
    <ResourceGrantDialog
      v-model="grantDialogVisible"
      resource-type="redis_instance"
      :resource-id="grantInstanceId"
      :resource-name="grantInstanceName"
      :manageable="isBatchGrant || canManage || grantOwner"
      :resources="batchResources"
      :title="isBatchGrant
        ? `批量权限配置：${batchResources.length} 个实例`
        : `${canManage || grantOwner ? '权限配置' : '权限查看'}：${grantInstanceName}`"
      @granted="onGrantsApplied"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ResourceGrantDialog, useApiError, usePermission, useSession } from '@nemo/shared'
import {
  createCluster,
  createInstance,
  deleteCluster,
  deleteInstance,
  listClusters,
  listInstances,
  updateInstance,
  type RedisCluster,
  type RedisInstance,
} from '../api/redis'

/** 菜单管理码 base：根行 permissionCode=redis:read 控菜单可见（页面能进来
 *  即已授），manage（redis:manage）控"管理员全权"。 */
const SERVICE = 'redis'

/** 当前用户是否持有 manage 码（基座 session.permissions 同步，实时刷新）。
 *  usePermission.canManage 是 (service)=>boolean 函数，须包 computed 才能
 *  供模板 v-if / prop 布尔使用（裸函数恒 truthy，会导致 manageable 收到
 *  Function 的 prop 类型警告、只读用户误显管理入口）。 */
const { canManage: hasManageCode, has } = usePermission()
const canManage = computed(() => hasManageCode(SERVICE))
/** 动作码（按钮级功能权限）：无实例全局操作（新建）。 */
const canCreate = computed(() => has('redis:create'))

/** 统一错误提示（403 → 无权限提示，其余透出后端 message）。 */
const handleApiError = useApiError()

const loading = ref(false)
const instances = ref<RedisInstance[]>([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

// ---------------------------------------------------------------------------
// 集群级联（先选集群再出实例）：集群为基础配置数据（不进权限引擎），读无
// 守卫（页面可见即可读），新建/删除挂 redis:manage 码（canManage 显隐）。
// 列表恒在具体集群上下文内——clusterId 必传，切换集群即刷新。
// ---------------------------------------------------------------------------
const clusters = ref<RedisCluster[]>([])
const clustersLoading = ref(false)
/** 当前选中集群（空串 = 未选：集群列表为空时，实例区空态引导建集群）。 */
const currentClusterId = ref('')
/** 当前集群展示 label（code + 备注），选择器与新建实例对话框共用。 */
function clusterLabel(c: RedisCluster): string {
  return c.remark ? `${c.code} · ${c.remark}` : c.code
}
const currentClusterCode = computed(() =>
  clusters.value.find((c) => c.id === currentClusterId.value)?.code ?? '',
)
/** 当前集群是否可配置授权名单（manage 码或集群 owner——行级徽标直接消费）。 */
const currentClusterCanManage = computed(() =>
  clusters.value.find((c) => c.id === currentClusterId.value)?.canManageGrants ?? false,
)

// ---------------------------------------------------------------------------
// 集群权限（授权名单）：谁能看到此集群（含空集群——不再依赖"先有可见实例"）。
// 走通用 ResourceGrantDialog 单资源模式（redis_cluster + viewer 单关系），
// 本页只记录打开的集群目标。
// ---------------------------------------------------------------------------
const clusterGrantVisible = ref(false)
const clusterGrantId = ref('')
const clusterGrantName = ref('')

function openClusterGrants() {
  const target = clusters.value.find((c) => c.id === currentClusterId.value)
  if (!target) return
  clusterGrantId.value = target.id
  clusterGrantName.value = target.code
  clusterGrantVisible.value = true
}

async function loadClusters() {
  clustersLoading.value = true
  try {
    const resp = await listClusters()
    clusters.value = resp.items
    // 选中态自愈：默认选第一个；已选集群被删（或不存在）时回落到第一个。
    if (!clusters.value.some((c) => c.id === currentClusterId.value)) {
      currentClusterId.value = clusters.value[0]?.id ?? ''
    }
  } catch (err) {
    handleApiError(err)
  } finally {
    clustersLoading.value = false
  }
}

/** 切换集群：重置页码与多选态后拉取该集群实例（分页谓词随集群变化）。 */
function onClusterChange() {
  page.value = 1
  selectedRows.value = []
  void loadInstances()
}

async function loadInstances() {
  if (!currentClusterId.value) {
    instances.value = []
    total.value = 0
    return
  }
  loading.value = true
  try {
    const resp = await listInstances(currentClusterId.value, page.value, pageSize.value)
    instances.value = resp.items
    total.value = resp.total
  } catch (err) {
    handleApiError(err)
  } finally {
    loading.value = false
  }
}

/** 切换每页条数回到第一页再拉（避免落在越界页上）。 */
function onSizeChange() {
  page.value = 1
  void loadInstances()
}

// ---------------------------------------------------------------------------
// 新建 / 编辑
// ---------------------------------------------------------------------------
const dialogVisible = ref(false)
const editing = ref(false)
const submitting = ref(false)
const form = reactive({ name: '', addr: '', description: '' })

/** 编辑目标 id：打开对话框时记录，避免与列表选中态耦合。 */
let selectedId = ''

function openCreate() {
  editing.value = false
  Object.assign(form, { name: '', addr: '', description: '' })
  dialogVisible.value = true
}

function openEdit(row: RedisInstance) {
  editing.value = true
  selectedId = row.id // 记录编辑目标 id（提交时传给后端）
  Object.assign(form, { name: row.name, addr: row.addr, description: row.description })
  dialogVisible.value = true
}

async function onSubmit() {
  const name = form.name.trim()
  if (!editing.value && !name) {
    ElMessage.warning('请输入实例名')
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      await updateInstance(selectedId, {
        addr: form.addr.trim(),
        description: form.description.trim(),
      })
      ElMessage.success('已保存')
    } else {
      // 归属当前选中集群（顶部选择器决定，创建对话框只读展示）。
      await createInstance({
        clusterId: currentClusterId.value,
        name,
        addr: form.addr.trim(),
        description: form.description.trim(),
      })
      ElMessage.success('已创建（你是 owner）')
    }
    dialogVisible.value = false
    await loadInstances()
  } catch (err) {
    // 403 REDIS_DENIED 等统一透出后端 message。
    handleApiError(err)
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: RedisInstance) {
  try {
    await ElMessageBox.confirm(
      `确定删除实例「${row.name}」？其授权关系将一并移除。`,
      '删除实例',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消
  }
  try {
    await deleteInstance(row.id)
    ElMessage.success('已删除')
    await loadInstances()
  } catch (err) {
    handleApiError(err)
  }
}

// ---------------------------------------------------------------------------
// 权限配置/查看：状态交给通用 ResourceGrantDialog（基础能力层），本页只
// 记录打开的实例目标——名单拉取/添加/移除/部门反查全部由组件与后端
// 通用 relations API（/v1/authz/relations）承载。
// ---------------------------------------------------------------------------
const grantDialogVisible = ref(false)
const grantInstanceId = ref('')
const grantInstanceName = ref('')
/** 打开时记录当前行是否 owner：配置权限对话框仅 manage 码或 owner 可写名单。 */
const grantOwner = ref(false)

/** 批量权限配置：勾选行集合（selection-change 维护）与传入对话框的
 *  可管理子集。非空 = 对话框进批量模式（跨 object 一次授予）。 */
const selectedRows = ref<RedisInstance[]>([])
const batchResources = ref<{ id: string; name: string }[]>([])
/** 对话框当前是否为批量模式：决定 manageable/title 走批量分支。 */
const isBatchGrant = computed(() => batchResources.value.length > 0)
const tableRef = ref<InstanceType<typeof import('element-plus')['ElTable']>>()

/** 表格多选变化：仅记录，按钮 disabled 与打开时过滤消费。 */
function onSelectionChange(rows: RedisInstance[]) {
  selectedRows.value = rows
}

/** 批量权限配置入口：只对选中集合中可管理的行（canManageGrants = manage
 *  码或 owner，与行内按钮同源）执行；无权行跳过并提示——避免整批被后端
 *  原子判权拒绝（relations 写接口任一对象无权则全部不写）。 */
function openBatchGrants() {
  const manageable = selectedRows.value.filter((r) => r.canManageGrants)
  if (manageable.length === 0) {
    ElMessage.warning('选中的实例均无名单管理权（需 manage 码或为 owner）')
    return
  }
  const skipped = selectedRows.value.length - manageable.length
  if (skipped > 0) {
    ElMessage.warning(`选中的 ${selectedRows.value.length} 个实例中有 ${skipped} 个无名单管理权，将跳过`)
  }
  batchResources.value = manageable.map((r) => ({ id: r.id, name: r.name }))
  grantDialogVisible.value = true
}

/** 批量授予成功回调（单资源模式也会触发，count=1）：清空表格选中态，
 *  授权数据由引擎即时生效，列表无需刷新。 */
function onGrantsApplied() {
  tableRef.value?.clearSelection()
}

function openGrants(row: RedisInstance) {
  // 单资源入口先清批量态，避免同一对话框实例残留上一次批量目标
  batchResources.value = []
  grantInstanceId.value = row.id
  grantInstanceName.value = row.name
  grantOwner.value = row.owner
  grantDialogVisible.value = true
}

// ---------------------------------------------------------------------------
// 集群管理（新建/删除，redis:manage 持有者）：code 唯一且创建后不可改；
// 删除非空集群由后端 409 拒绝（先迁走/删实例）。
// ---------------------------------------------------------------------------
const clusterDialogVisible = ref(false)
const clusterSubmitting = ref(false)
const clusterForm = reactive({ code: '', remark: '' })

function openCreateCluster() {
  Object.assign(clusterForm, { code: '', remark: '' })
  clusterDialogVisible.value = true
}

async function onSubmitCluster() {
  const code = clusterForm.code.trim()
  if (!code) {
    ElMessage.warning('请输入集群编码')
    return
  }
  clusterSubmitting.value = true
  try {
    const created = await createCluster({ code, remark: clusterForm.remark.trim() })
    ElMessage.success(`集群「${code}」已创建，可在该集群下新建实例`)
    clusterDialogVisible.value = false
    await loadClusters()
    // 建完即切到新集群（选择器上下文就位，用户可直接新建实例）。
    currentClusterId.value = created.id
    onClusterChange()
  } catch (err) {
    handleApiError(err)
  } finally {
    clusterSubmitting.value = false
  }
}

async function onDeleteCluster() {
  const target = clusters.value.find((c) => c.id === currentClusterId.value)
  if (!target) return
  try {
    await ElMessageBox.confirm(
      `确定删除集群「${target.code}」？集群下仍有实例时将被拒绝。`,
      '删除集群',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消
  }
  try {
    await deleteCluster(target.id)
    ElMessage.success('集群已删除')
    await loadClusters() // 选中态在 loadClusters 内回落到第一个
    onClusterChange()
  } catch (err) {
    handleApiError(err)
  }
}

const { ready } = useSession()

onMounted(async () => {
  await ready() // iframe 握手竞态门控:token 到位后再发首个请求
  // 先加载集群并默认选中第一个，再拉该集群下的实例（列表依赖集群上下文）。
  await loadClusters()
  void loadInstances()
})
</script>

<style scoped>
/* =====================================================================
   视觉层：全部使用 Element Plus 语义变量着色（明暗主题自动适配），
   与 TopicManage 同款页面骨架。
   ===================================================================== */
.page {
  padding: 16px 18px 20px;
}

/* 进场编排：标题 → 工具条 → 表格 → 分页 依次上浮（尊重减弱动效偏好）。 */
@media (prefers-reduced-motion: no-preference) {
  .page-head,
  .toolbar,
  .table-card,
  .pager-row {
    animation: rise 0.38s cubic-bezier(0.22, 0.61, 0.36, 1) both;
  }
  .toolbar { animation-delay: 0.05s; }
  .table-card { animation-delay: 0.1s; }
  .pager-row { animation-delay: 0.16s; }
}
@keyframes rise {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: none; }
}

/* ---- 页头：左（标题 + 可见范围标记）右（权限身份胶囊）---- */
.page-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.head-left {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-right: auto;
}
.page-title {
  position: relative;
  margin: 0;
  padding-left: 12px;
  font-size: 17px;
  font-weight: 600;
}
.page-title::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 4px;
  height: 16px;
  border-radius: 2px;
  background: var(--el-color-primary);
}
/* 可见范围标记：轻量描边徽标，悬停见完整说明 */
.demo-mark {
  cursor: help;
  font-size: 12px;
  line-height: 1;
  padding: 4px 8px;
  border-radius: 4px;
  color: var(--el-color-info);
  border: 1px solid var(--el-border-color);
  background: var(--el-fill-color-light);
}
/* 权限身份胶囊：三档语义色（管理=主色 / 部分=警示 / 只读=中性） */
.perm-pill {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1px solid transparent;
}
.perm-pill.grant {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary-light-7);
  background: var(--el-color-primary-light-9);
}
.perm-pill.partial {
  color: var(--el-color-warning);
  border-color: var(--el-color-warning-light-5);
  background: var(--el-color-warning-light-9);
}
.perm-pill.readonly {
  color: var(--el-text-color-secondary);
  border-color: var(--el-border-color);
  background: var(--el-fill-color-light);
}

/* ---- 集群工具条（先选集群再出实例的级联入口） ---- */
.cluster-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.cluster-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  flex-shrink: 0;
}
.cluster-select {
  width: 260px;
}
.cluster-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
/* 新建实例对话框中的所属集群只读展示（等宽字体标识） */
.cluster-ref {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 13px;
  color: var(--el-text-color-regular);
}

/* ---- 工具条 ---- */
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.hint-mark {
  cursor: help;
  font-size: 12px;
  color: var(--el-color-info);
  text-decoration: underline dotted;
  text-underline-offset: 3px;
}

/* ---- 表格卡片 ---- */
.table-card {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;
  background: var(--el-bg-color);
}
.instance-name {
  font-weight: 600;
}
/* owner 徽标：主色描边小标签，与名称同行 */
.owner-flag {
  margin-left: 6px;
  font-size: 11px;
  line-height: 1;
  padding: 3px 6px;
  border-radius: 4px;
  color: var(--el-color-primary);
  border: 1px solid var(--el-color-primary-light-7);
  background: var(--el-color-primary-light-9);
  vertical-align: 1px;
}
/* 地址列：等宽字体呈现 host:port */
.addr-code {
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  color: var(--el-text-color-regular);
}
.desc-empty {
  color: var(--el-text-color-placeholder);
}
/* 空态补充说明 */
.empty-note {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* ---- 分页行 ---- */
.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}
</style>
