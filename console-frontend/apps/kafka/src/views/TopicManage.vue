<template>
  <div class="page">
    <header class="page-head">
      <div class="head-left">
        <h3 class="page-title">Topic 管理</h3>
        <!-- 演示域标记：详情收进 tooltip，头部只留一枚轻量徽标 -->
        <el-tooltip content="消息域演示页：真实环境由独立 Kafka 管理服务承载，同模式接入 kit/authz 统一鉴权" placement="top">
          <span class="demo-mark">演示域</span>
        </el-tooltip>
      </div>
      <!-- 权限身份胶囊（右对齐）：当前登录者在本页的能力档位，
           与下方按钮显隐同一判定源，所见即所得。 -->
      <span v-if="canManage" class="perm-pill grant">可管理 · 增删改/授权名单</span>
      <span v-else-if="canCreate || canExport" class="perm-pill partial">部分功能权 · 新建/导出</span>
      <span v-else class="perm-pill readonly">可查看</span>
    </header>

    <div class="toolbar">
      <!-- 全局功能按钮（无实例）：新建=create 动作码或 manage 码；导出=export 动作码
           或 manage 码——管理档天然兜底，与新建/编辑/删除等操作对齐。 -->
      <el-button v-if="canManage || canCreate" type="primary" @click="openCreate">新建 Topic</el-button>
      <el-button v-if="canManage || canExport" @click="onExport">导出清单</el-button>
      <!-- 批量权限配置：勾选多个 Topic 一次授予（目标集合在打开前按行级
           canManageGrants 过滤，无可管理行时禁点）。页面级操作入口，不进
           menu-buttons 声明（与新建/导出同级，不挂资源级 relation）——操作权
           仍由选中行的 owner/manage 关系驱动，与行内按钮同一判定源。 -->
      <el-button type="primary" plain :disabled="selectedRows.length === 0" @click="openBatchGrants">
        批量权限配置{{ selectedRows.length ? `（${selectedRows.length}）` : '' }}
      </el-button>
      <!-- 权限说明收敛为悬停提示：功能码语义（create/export 与 manage 的兜底
           关系）按需展开，不再铺常驻长文案。 -->
      <el-tooltip placement="bottom-start">
        <template #content>
          新建需 kafka-topics:create 或 manage 码；<br />
          导出需 kafka-topics:export 或 manage 码；<br />
          功能码可在「人员授权」页逐菜单单独勾选；<br />
          勾选多行可批量授予名单（仅对您有权管理的 Topic 生效）。
        </template>
        <span class="hint-mark">权限说明</span>
      </el-tooltip>
    </div>

    <section class="table-card">
      <el-table ref="tableRef" v-loading="loading" :data="topics" class="topic-table" @selection-change="onSelectionChange">
        <!-- 空态：顺带解释可见范围语义（授权驱动列表，非全量目录） -->
        <template #empty>
          <el-empty :image-size="72" description="暂无 Topic">
            <p class="empty-note">可见范围 = 授权给您的 Topic（持有管理码时全部可见）</p>
          </el-empty>
        </template>
        <!-- 多选列：批量权限配置的行选择（分页翻页重新拉数据，选中态随之清空） -->
        <el-table-column type="selection" width="42" />
        <el-table-column label="Topic" min-width="150">
          <template #default="{ row }">
            <span class="topic-name">{{ row.name }}</span>
            <!-- owner 徽标：把行内按钮差异（为何别人有编辑/配置权限而我没有）
                 的根因直接可视在行上，悬停见完整解释 -->
            <el-tooltip v-if="row.owner" content="你是该 Topic 的 owner：可编辑、删除并配置权限名单" placement="top">
              <span class="owner-flag">owner</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="环境" width="88">
          <template #default="{ row }">
            <span class="env-badge" :class="`env-${envClass(row.env)}`">{{ row.env || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="{ 'desc-empty': !row.description }">{{ row.description || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="createdBy" label="创建人" width="88" />
        <el-table-column label="操作" width="300" class-name="op-col">
          <template #default="{ row }">
            <!-- 操作列按菜单声明渲染（authz /v1/authz/menu-buttons）：声明提供
                 按钮文案 + 模型出口（relation），显隐 = relation 匹配行级
                 canX（后端已并入 manage 码回退）；danger/低频操作收进「更多」
                 下拉（纯 UI 决策防溢出）。改菜单声明（增/删/改 relation）即
                 改页面按钮——声明是唯一真源，前端不再写死按钮清单。
                 纯 viewer 行只有 can_view 出口的声明按钮（查看消费者）。 -->
            <template v-for="btn in directButtons" :key="btn.permissionCode">
              <el-button
                v-if="canShowButton(btn, row)"
                link
                type="primary"
                @click="runButton(btn.permissionCode, row)"
              >{{ btn.title }}</el-button>
            </template>
            <!-- 更多▾：仅当行内存在可收纳（danger）操作时显示。 -->
            <el-dropdown
              v-if="moreButtons.some((b) => canShowButton(b, row))"
              trigger="click"
              popper-class="topic-op-dd"
              @command="(code: string) => runButton(code, row)"
            >
              <el-button link type="primary">更多 ▾</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <template v-for="(btn, idx) in moreButtons" :key="btn.permissionCode">
                    <el-dropdown-item
                      v-if="canShowButton(btn, row)"
                      :command="btn.permissionCode"
                      :divided="idx > 0"
                      :class="{ 'danger-item': isDanger(btn) }"
                    >{{ btn.title }}</el-dropdown-item>
                  </template>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 分页：服务端分页（可见性谓词下推 SQL），total 为满足谓词的近似值 -->
    <div class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        background
        @current-change="loadTopics"
        @size-change="onSizeChange"
      />
    </div>

    <!-- 新建/编辑共用对话框（编辑不传 name：对象身份不可改） -->
    <el-dialog v-model="dialogVisible" :title="editing ? '编辑 Topic' : '新建 Topic'" width="480px">
      <el-form label-width="104px" @submit.prevent="onSubmit">
        <el-form-item label="Topic 名" required>
          <el-input v-model="form.name" :disabled="editing" maxlength="128" placeholder="如 order-events" />
        </el-form-item>
        <el-form-item label="环境">
          <!-- 预设环境可直接选，也允许自由输入自定义标识（allow-create） -->
          <el-select
            v-model="form.env"
            filterable
            allow-create
            default-first-option
            clearable
            placeholder="选择或输入环境标识（prod / staging / dev）"
            style="width: 100%"
          >
            <el-option v-for="e in envOptions" :key="e" :label="e" :value="e" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" maxlength="512" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item v-if="!editing" label="可见组织">
          <!-- 归属组织（决定该组织及子树成员能否看到本 Topic）：不选回退
               创建人所在部门（存量行为）。编辑态不支持改归属（改归属需同步
               增删引擎元组）；要给更多组织/个人开可见，用行内「权限配置」。 -->
          <OrgTreeSelect v-model="form.orgId" placeholder="不选默认你所在的部门" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">
          {{ editing ? '保存' : '创建' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 权限配置/查看对话框：通用资源授权组件（基础能力层）——资源类型 kafka_topic；
         授权档位由后端按该类型注册的 can_* 出口自动派生（viewer/editor/owner…），
         业务页零硬编码；manage 码持有者可配置，read 用户只读查看
         （名实一致，后端 /v1/authz/relations 按资源类型校验管理权）。
         batchResources 非空 = 批量模式（行内勾选的 Topic 集合，一次跨 object
         授予）；单资源模式（行内按钮）照旧传 resource-id/resource-name。 -->
    <ResourceGrantDialog
      v-model="grantDialogVisible"
      resource-type="kafka_topic"
      :resource-id="grantTopicId"
      :resource-name="grantTopicName"
      :manageable="isBatchGrant || canManage || grantOwner"
      :resources="batchResources"
      :title="isBatchGrant
        ? `批量权限配置：${batchResources.length} 个 Topic`
        : `${canManage || grantOwner ? '权限配置' : '权限查看'}：${grantTopicName}`"
      @granted="onGrantsApplied"
    />

    <!-- 查看消费者：行级只读动作，鉴权复用 can_view（可见行即可查看）；
         demo 无 Kafka 集群返回空列表，展示空态。 -->
    <el-dialog v-model="consumersVisible" :title="`查看消费者：${consumersTopicName}`" width="560px">
      <el-table v-loading="consumersLoading" :data="consumers" empty-text="暂无消费者组（生产环境由 Kafka 集群实时查询）">
        <el-table-column prop="groupId" label="消费者组" min-width="180" />
        <el-table-column prop="state" label="状态" width="110" />
        <el-table-column prop="members" label="成员数" width="90" />
        <el-table-column prop="lag" label="消费延迟" width="100" />
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { OrgTreeSelect, ResourceGrantDialog, useApiError, usePermission, useSession } from '@nemo/shared'
import {
  createKafkaTopic,
  deleteKafkaTopic,
  exportKafkaTopics,
  resetKafkaTopicOffset,
  getTopicConsumers,
  getTopicMenuButtons,
  listKafkaTopics,
  updateKafkaTopic,
  type KafkaTopic,
  type MenuButtonDecl,
  type TopicConsumerGroup,
} from '../api/kafka'

/** 菜单管理码：read 控菜单可见性（页面能进来即已授），manage 控增删改。 */
const SERVICE = 'kafka-topics'

/** 当前用户是否持有 manage 码（基座 session.permissions 同步，实时刷新）。
 *  usePermission.canManage 是 (service)=>boolean 函数，须包 computed 才能
 *  供模板 v-if / prop 布尔使用（裸函数恒 truthy，会导致 manageable 收到
 *  Function 的 prop 类型警告、只读用户误显管理入口）。 */
const { canManage: hasManageCode, has } = usePermission()
const canManage = computed(() => hasManageCode(SERVICE))
/** 动作码（按钮级功能权限）：无实例全局操作（新建/导出），授给谁谁见对应
 *  按钮；行内资源操作（编辑/删除/重置位点）走资源关系，不建码。 */
const canExport = computed(() => has('kafka-topics:export'))
const canCreate = computed(() => has('kafka-topics:create'))

/** 环境预设选项（el-select 可自由创建新值，展示端对未知环境走中性色）。 */
const envOptions = ['prod', 'staging', 'dev']

// ---------------------------------------------------------------------------
// 菜单声明驱动（操作列按钮的唯一真源）：
// 按钮清单来自 authz /v1/authz/menu-buttons（按路由定位：path=/kafka/topics
// 菜单下的按钮行），
// 文案/出口（relation）/归属类型都吃菜单配置——改菜单声明即改页面按钮。
// 拉取失败 → 操作列 fail-closed（不渲染任何按钮，列表仍可读）。
// ---------------------------------------------------------------------------
const menuButtons = ref<MenuButtonDecl[]>([])

/** 资源级按钮（resourceType=kafka_topic）：行内操作列只渲染资源级声明；
 *  页面级按钮（新建/导出，relation 空）走顶部工具条，不进操作列。 */
const rowButtons = computed(() =>
  menuButtons.value.filter((b) => b.resourceType === 'kafka_topic'),
)
/** 直出按钮：非 danger 高频操作直接展示（防多按钮溢出换行成两栏）。 */
const directButtons = computed(() => rowButtons.value.filter((b) => !isDanger(b)))
/** 收纳按钮：danger/低频操作收进「更多」下拉。 */
const moreButtons = computed(() => rowButtons.value.filter((b) => isDanger(b)))

/** 声明按钮显隐：relation 匹配行级 canX（后端字段已并入 manage 码回退）；
 *  未知出口一律不渲染（fail-closed——声明先于实现，宁缺勿滥）。 */
function canShowButton(btn: MenuButtonDecl, row: KafkaTopic): boolean {
  const manage = canManage.value
  switch (btn.relation) {
    case 'can_view':
      return manage || row.canView
    case 'can_edit':
      return manage || row.canEdit
    case 'can_delete':
      return manage || row.canDelete
    case 'can_manage_grants':
      return manage || row.canManageGrants
    default:
      return false
  }
}

/** 收纳判定（纯 UI 决策，非权限）：删除/重置类低频危险操作进「更多」下拉。 */
function isDanger(btn: MenuButtonDecl): boolean {
  return /delete|remove|reset|freeze/i.test(btn.permissionCode)
}

/** 声明按钮动作分发：按 permissionCode（按钮授权码）映射到既有处理函数；
 *  未注册动作的声明按钮在 canShowButton 已被挡（不渲染），此处兜底告警。 */
function runButton(code: string, row: KafkaTopic) {
  const action = BUTTON_ACTIONS[code]
  if (action) action(row)
  else console.warn(`[TopicManage] 未注册按钮动作: ${code}`)
}

/** 声明按钮 → 行操作 handler 映射：新增菜单按钮时在此注册动作才可点。 */
const BUTTON_ACTIONS: Record<string, (row: KafkaTopic) => void> = {
  'kafka-topics:edit': (row) => openEdit(row),
  'kafka-topics:delete': (row) => void onDelete(row),
  'kafka-topics:manage-grants': (row) => openGrants(row),
  'kafka-topics:view-consumers': (row) => void openConsumers(row),
  'kafka-topics:reset-offset': (row) => void onResetOffset(row),
}

/** 环境徽标分类：已知环境映射语义色（prod=danger / staging=warning /
 *  dev=info），其余（含空值、自由输入值）走中性样式——表单宽松输入，
 *  展示端归类展示，不做强约束。 */
function envClass(env: string): string {
  const e = (env || '').trim().toLowerCase()
  if (e === 'prod' || e === 'production') return 'danger'
  if (e === 'staging' || e === 'stage' || e === 'uat') return 'warning'
  if (e === 'dev' || e === 'develop' || e === 'development' || e === 'test') return 'info'
  return 'other'
}

/** 统一错误提示（403 → 无权限提示，其余透出后端 message）。 */
const handleApiError = useApiError()

const loading = ref(false)
const topics = ref<KafkaTopic[]>([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

async function loadTopics() {
  loading.value = true
  try {
    const resp = await listKafkaTopics(page.value, pageSize.value)
    topics.value = resp.items
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
  void loadTopics()
}

// ---------------------------------------------------------------------------
// 新建 / 编辑
// ---------------------------------------------------------------------------
const dialogVisible = ref(false)
const editing = ref(false)
const submitting = ref(false)
const form = reactive({ name: '', env: '', description: '', orgId: '' })

function openCreate() {
  editing.value = false
  Object.assign(form, { name: '', env: '', description: '', orgId: '' })
  dialogVisible.value = true
}

function openEdit(row: KafkaTopic) {
  editing.value = true
  selectedId = row.id // 记录编辑目标 id（提交时传给后端）
  Object.assign(form, {
    name: row.name,
    env: row.env,
    description: row.description,
    orgId: '', // 编辑态不展示归属组织字段，仅清残留值
  })
  dialogVisible.value = true
}

async function onSubmit() {
  const name = form.name.trim()
  if (!name) {
    ElMessage.warning('请输入 Topic 名')
    return
  }
  // Kafka 主题命名惯例（字母/数字/._-）：前端即时反馈，后端仍兜底校验。
  if (!/^[a-zA-Z0-9._-]{1,128}$/.test(name)) {
    ElMessage.warning('Topic 名仅限字母、数字与 . _ -（Kafka 命名惯例）')
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      await updateKafkaTopic(selectedId, {
        env: form.env.trim(),
        description: form.description.trim(),
      })
      ElMessage.success('已保存')
    } else {
      await createKafkaTopic({
        name,
        env: form.env.trim(),
        description: form.description.trim(),
        orgId: form.orgId || undefined,
      })
      ElMessage.success('已创建（你是 owner）')
    }
    dialogVisible.value = false
    await loadTopics()
  } catch (err) {
    // 403 AUTHZ_DENIED（manage 码判定拒绝）等统一透出后端 message。
    handleApiError(err)
  } finally {
    submitting.value = false
  }
}

/** 编辑目标 id：打开对话框时记录，避免与列表选中态耦合。 */
let selectedId = ''

/** 动作码演示：导出清单（可见范围与列表一致，CSV 下载）。 */
async function onExport() {
  try {
    const csv = await exportKafkaTopics()
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'topics.csv'
    a.click()
    URL.revokeObjectURL(url)
    ElMessage.success('已导出当前可见的 Topic 清单')
  } catch (err) {
    handleApiError(err)
  }
}

/** 动作码演示：重置位点（demo 语义，确认后调用；对象权由后端叠加判定）。 */
async function onResetOffset(row: KafkaTopic) {
  try {
    await ElMessageBox.confirm(
      `确认重置 Topic「${row.name}」的消费者位点到最新？`,
      '重置位点',
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await resetKafkaTopicOffset(row.id)
    ElMessage.success('已受理（demo：未连接 Kafka 集群）')
  } catch (err) {
    handleApiError(err)
  }
}

async function onDelete(row: KafkaTopic) {
  try {
    await ElMessageBox.confirm(
      `确定删除 Topic「${row.name}」？其授权关系将一并移除。`,
      '删除 Topic',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消
  }
  try {
    await deleteKafkaTopic(row.id)
    ElMessage.success('已删除')
    await loadTopics()
  } catch (err) {
    handleApiError(err)
  }
}

// ---------------------------------------------------------------------------
// 权限配置/查看：状态交给通用 ResourceGrantDialog（基础能力层），本页只
// 记录打开的 topic 目标——名单拉取/添加/移除/部门反查全部由组件与后端
// 通用 relations API（/v1/authz/relations）承载。
// ---------------------------------------------------------------------------
const grantDialogVisible = ref(false)
const grantTopicId = ref('')
const grantTopicName = ref('')
/** 打开时记录当前行是否 owner：配置权限对话框仅 manage 码或 owner 可写名单。 */
const grantOwner = ref(false)

/** 批量权限配置：勾选行集合（selection-change 维护）与传入对话框的
 *  可管理子集。非空 = 对话框进批量模式（跨 object 一次授予）。 */
const selectedRows = ref<KafkaTopic[]>([])
const batchResources = ref<{ id: string; name: string }[]>([])
/** 对话框当前是否为批量模式：决定 manageable/title 走批量分支。 */
const isBatchGrant = computed(() => batchResources.value.length > 0)
const tableRef = ref<InstanceType<typeof import('element-plus')['ElTable']>>()

/** 表格多选变化：仅记录，按钮 disabled 与打开时过滤消费。 */
function onSelectionChange(rows: KafkaTopic[]) {
  selectedRows.value = rows
}

/** 批量权限配置入口：只对选中集合中可管理的行（canManageGrants = manage
 *  码或 owner，与行内按钮同源）执行；无权行跳过并提示——避免整批被后端
 *  原子判权拒绝（relations 写接口任一对象无权则全部不写）。 */
function openBatchGrants() {
  const manageable = selectedRows.value.filter((r) => r.canManageGrants)
  if (manageable.length === 0) {
    ElMessage.warning('选中的 Topic 均无名单管理权（需 manage 码或为 owner）')
    return
  }
  const skipped = selectedRows.value.length - manageable.length
  if (skipped > 0) {
    ElMessage.warning(`选中的 ${selectedRows.value.length} 个 Topic 中有 ${skipped} 个无名单管理权，将跳过`)
  }
  batchResources.value = manageable.map((r) => ({ id: r.id, name: r.name }))
  grantDialogVisible.value = true
}

/** 批量授予成功回调（单资源模式也会触发，count=1）：清空表格选中态，
 *  授权数据由引擎即时生效，列表无需刷新。 */
function onGrantsApplied() {
  tableRef.value?.clearSelection()
}

function openGrants(row: KafkaTopic) {
  // 单资源入口先清批量态，避免同一对话框实例残留上一次批量目标
  batchResources.value = []
  grantTopicId.value = row.id
  grantTopicName.value = row.name
  grantOwner.value = row.owner
  grantDialogVisible.value = true
}

const { ready } = useSession()

// ---------------------------------------------------------------------------
// 查看消费者（行级只读，鉴权在后端 topicCanView：can_view 或 manage 码）
// ---------------------------------------------------------------------------
const consumersVisible = ref(false)
const consumersLoading = ref(false)
const consumersTopicName = ref('')
const consumers = ref<TopicConsumerGroup[]>([])

async function openConsumers(row: KafkaTopic) {
  consumersTopicName.value = row.name
  consumersVisible.value = true
  consumersLoading.value = true
  try {
    const resp = await getTopicConsumers(row.id)
    consumers.value = resp.items
  } catch (err) {
    consumers.value = []
    handleApiError(err)
  } finally {
    consumersLoading.value = false
  }
}

onMounted(async () => {
  await ready() // iframe 握手竞态门控:token 到位后再发首个请求
  try {
    // 操作列按钮声明（唯一真源）：拉取失败 → 操作列不渲染（fail-closed），
    // 列表仍可读（只读视角不受影响）。
    const resp = await getTopicMenuButtons()
    menuButtons.value = resp.items
  } catch (err) {
    handleApiError(err)
  }
  void loadTopics()
})
</script>

<style scoped>
/* =====================================================================
   视觉层：全部使用 Element Plus 语义变量着色（明暗主题自动适配），
   不引入任何新依赖——提示图标用纯 CSS 绘制，动效纯 CSS 编排。
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

/* ---- 页头：左（标题 + 演示域标记）右（权限身份胶囊）---- */
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
  letter-spacing: 0.5px;
}
/* 主题色竖条 accent：页面身份的视觉锚点 */
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
.demo-mark {
  font-size: 11px;
  line-height: 18px;
  padding: 0 7px;
  border-radius: 4px;
  color: var(--el-text-color-secondary);
  border: 1px dashed var(--el-border-color);
  cursor: help;
  transition: color 0.2s, border-color 0.2s;
}
.demo-mark:hover {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary);
}
/* 权限身份胶囊：语义圆点 + 档位短语（原长句标签收敛为一眼可读） */
.perm-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  line-height: 20px;
  padding: 0 10px;
  border-radius: 999px;
  border: 1px solid transparent;
}
.perm-pill::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.perm-pill.grant {
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
  border-color: var(--el-color-success-light-7);
}
.perm-pill.partial {
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  border-color: var(--el-color-primary-light-7);
}
.perm-pill.readonly {
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color);
  border-color: var(--el-border-color-lighter);
}

/* ---- 工具条：主操作 + 可悬停展开的权限说明 ---- */
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}
/* CSS 手绘 "?" 气泡（不引入图标依赖） */
.hint-mark {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  cursor: help;
  transition: color 0.2s;
}
.hint-mark::before {
  content: '?';
  width: 14px;
  height: 14px;
  border: 1px solid currentColor;
  border-radius: 50%;
  font-size: 10px;
  line-height: 13px;
  text-align: center;
}
.hint-mark:hover {
  color: var(--el-color-primary);
}

/* ---- 表格卡片：圆角描边容器 + 表头底色 + 悬停投影 ---- */
.table-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  overflow: hidden;
  transition: box-shadow 0.25s ease;
}
.table-card:hover {
  box-shadow: var(--el-box-shadow-light);
}
.topic-table {
  --el-table-header-bg-color: var(--el-fill-color-light);
}
.topic-table :deep(th.el-table__cell) {
  font-weight: 600;
  color: var(--el-text-color-regular);
}
/* Topic 名：技术标识用等宽字族，扫读更快 */
.topic-name {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, 'Courier New', monospace;
  font-size: 13px;
  font-weight: 600;
}
/* owner 徽标：行内按钮差异（编辑/删除/配置权限）的根因可视 */
.owner-flag {
  margin-left: 8px;
  padding: 0 6px;
  font-size: 10px;
  line-height: 16px;
  font-weight: 500;
  border-radius: 999px;
  vertical-align: 1px;
  cursor: help;
  color: var(--el-color-success);
  background: var(--el-color-success-light-9);
  border: 1px solid var(--el-color-success-light-7);
}
/* 环境徽标：语义色归类（prod 红 / staging 橙 / dev 蓝 / 其他中性） */
.env-badge {
  display: inline-block;
  padding: 0 8px;
  font-size: 12px;
  line-height: 20px;
  border-radius: 4px;
  border: 1px solid transparent;
}
.env-badge.env-danger {
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
  border-color: var(--el-color-danger-light-7);
  font-weight: 600;
}
.env-badge.env-warning {
  color: var(--el-color-warning);
  background: var(--el-color-warning-light-9);
  border-color: var(--el-color-warning-light-7);
}
.env-badge.env-info {
  color: var(--el-color-info);
  background: var(--el-color-info-light-9);
  border-color: var(--el-color-info-light-7);
}
.env-badge.env-other {
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color);
  border-color: var(--el-border-color-lighter);
}
.desc-empty {
  color: var(--el-text-color-placeholder);
}
.empty-note {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* ---- 分页 ---- */
.pager-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

/* ---- 操作列「更多」下拉弹层（teleport 到 body，须用 :global 穿透）---- */
.op-col :deep(.el-dropdown) {
  vertical-align: top;
  margin-left: 12px;
}
:global(.topic-op-dd .danger-item) {
  color: var(--el-color-danger);
}
:global(.topic-op-dd .danger-item:not(.is-disabled):hover) {
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
}
</style>
