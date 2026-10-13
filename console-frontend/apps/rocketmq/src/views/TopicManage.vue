<template>
  <div class="page">
    <header class="page-head">
      <div class="head-left">
        <h3 class="page-title">Topic 管理</h3>
        <el-tooltip
          content="集群级授权：能看到集群即能看到该集群下全部 Topic 并可操作（操作按钮恒显示，后端按集群判定兜底）；授权名单在「集群权限」配置"
          placement="top"
        >
          <span class="model-mark">集群级授权</span>
        </el-tooltip>
      </div>
    </header>

    <!-- 集群选择条：先选集群，再展示该集群下的 Topic 列表。 -->
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
           入口 = 管理码或当前集群 owner（行级 canManageGrants 徽标驱动）；
           经通用 ResourceGrantDialog 写 rmq_cluster viewer 元组。 -->
      <el-button v-if="currentClusterCanManage" link type="primary" @click="openClusterGrants">集群权限</el-button>
      <span class="cluster-hint">Topic 归属集群：切换集群即刷新该集群下的 Topic</span>
    </div>

    <div class="toolbar">
      <!-- 全局功能按钮（无实例）：新建=create 动作码或 manage 码——管理档
           天然兜底；归属当前选中集群（未选集群时禁点）。 -->
      <el-button v-if="canManage || canCreate" type="primary" :disabled="!currentClusterId" @click="openCreate">
        新建 Topic
      </el-button>
      <el-tooltip placement="bottom-start">
        <template #content>
          新建需 rocketmq-topics:create 或管理码；<br />
          编辑/删除按 Topic 归属集群的可见性判定（集群可见即可操作）；<br />
          授权名单在「集群权限」配置，管理码持有者全权。
        </template>
        <span class="hint-mark">权限说明</span>
      </el-tooltip>
    </div>

    <section class="table-card">
      <el-table v-loading="loading" :data="topics" class="topic-table">
        <template #empty>
          <el-empty :image-size="72" :description="clusters.length ? '该集群暂无 Topic' : '暂无集群'">
            <p v-if="!clusters.length" class="empty-note">
              {{ canManage ? '先创建集群，再在该集群下新建 Topic' : '请先联系管理员创建集群' }}
            </p>
            <p v-else class="empty-note">可见范围 = 您可查看的集群下的全部 Topic（持有管理码时全部可见）</p>
          </el-empty>
        </template>
        <el-table-column label="Topic" min-width="160">
          <template #default="{ row }">
            <span class="topic-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="环境" width="90">
          <template #default="{ row }">
            <span class="env-badge" :class="`env-${envClass(row.env)}`">{{ row.env || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="描述" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="{ 'desc-empty': !row.description }">{{ row.description || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="归属组织" width="120">
          <template #default="{ row }">{{ row.orgId ? orgName(row.orgId) : '—' }}</template>
        </el-table-column>
        <el-table-column prop="createdBy" label="创建人" width="100" />
        <!-- 操作列：集群可见即可操作——按钮恒显示（后端集群级守卫兜底）。 -->
        <el-table-column label="操作" width="160" class-name="op-col">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="onDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 新建/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="dialogMode === 'create' ? '新建 Topic' : '编辑 Topic'" width="480px">
      <el-form label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" :disabled="dialogMode === 'edit'" placeholder="Topic 名称（编辑不可改）" />
        </el-form-item>
        <el-form-item v-if="dialogMode === 'create'" label="集群">
          <el-select v-model="form.clusterId" placeholder="Topic 归属集群" style="width: 100%">
            <el-option v-for="c in clusters" :key="c.id" :value="c.id" :label="clusterLabel(c)" />
          </el-select>
        </el-form-item>
        <el-form-item label="环境">
          <el-select v-model="form.env" allow-create filterable placeholder="选择或输入环境" style="width: 100%">
            <el-option v-for="e in envOptions" :key="e" :label="e" :value="e" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="用途说明" />
        </el-form-item>
        <el-form-item v-if="dialogMode === 'create'" label="归属组织">
          <OrgTreeSelect v-model="form.orgId" placeholder="不选默认你所在的部门" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">{{ dialogMode === 'create' ? '创建' : '保存' }}</el-button>
      </template>
    </el-dialog>

    <!-- 集群权限对话框：通用资源授权组件（基础能力层）——资源类型
         rmq_cluster + viewer 单关系（集群无"编辑"操作，可见即可）。
         名单/添加/移除由组件与后端通用 relations API 承载；管理码持有者
         或集群 owner 可配置（后端按注册表 manage_code + owner 关系校验）。 -->
    <ResourceGrantDialog
      v-model="clusterGrantVisible"
      resource-type="rmq_cluster"
      :resource-id="clusterGrantId"
      :resource-name="clusterGrantName"
      manageable
      :title="`集群权限：${clusterGrantName}`"
    />

    <!-- 新建集群对话框 -->
    <el-dialog v-model="clusterDialogVisible" title="新建集群" width="420px">
      <el-form label-width="70px">
        <el-form-item label="编码" required>
          <el-input v-model="clusterForm.code" placeholder="集群唯一编码，如 rmq-prod" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="clusterForm.remark" placeholder="用途说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="clusterDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="clusterSubmitting" @click="onSubmitCluster">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ResourceGrantDialog, OrgTreeSelect, createMicroHttp, usePermission } from '@nemo/shared'
import {
  createCluster,
  createRocketmqTopic,
  deleteCluster,
  deleteRocketmqTopic,
  listClusters,
  listRocketmqTopics,
  updateRocketmqTopic,
  type RocketmqCluster,
  type RocketmqTopic,
} from '../api/rocketmq'

// ---------------------------------------------------------------------------
// 权限（集群级模型）：集群可见即可操作——行内编辑/删除恒显示；
// 新建挂 create 码或管理码；集群 CRUD 与授权名单挂管理码/owner。
// ---------------------------------------------------------------------------
const MANAGE_CODE = 'rocketmq-topics:manage'
const CREATE_CODE = 'rocketmq-topics:create'

const { has } = usePermission()
const canManage = computed(() => has(MANAGE_CODE))
const canCreate = computed(() => has(CREATE_CODE))

// ---------------------------------------------------------------------------
// 集群：选择器数据源 + 权限入口
// ---------------------------------------------------------------------------
const http = createMicroHttp()
const clusters = ref<RocketmqCluster[]>([])
const clustersLoading = ref(false)
const currentClusterId = ref('')

/** 集群行标签：编码 + 备注摘要（备注空只显编码）。 */
function clusterLabel(c: RocketmqCluster): string {
  return c.remark ? `${c.code}（${c.remark}）` : c.code
}

/** 当前集群是否可配置授权名单（管理码或集群 owner——行级徽标直接消费）。 */
const currentClusterCanManage = computed(() =>
  clusters.value.find((c) => c.id === currentClusterId.value)?.canManageGrants ?? false,
)

async function loadClusters() {
  clustersLoading.value = true
  try {
    const resp = await listClusters()
    clusters.value = resp.items
    // 选中态自愈：默认选第一个；已选集群被删（或不存在）时回落到第一个。
    if (!clusters.value.some((c) => c.id === currentClusterId.value)) {
      currentClusterId.value = clusters.value[0]?.id ?? ''
    }
    if (currentClusterId.value) await load()
  } catch (err) {
    handleApiError(err)
  } finally {
    clustersLoading.value = false
  }
}

function onClusterChange() {
  void load()
}

// ---------------------------------------------------------------------------
// 集群权限（授权名单）：谁能看到此集群（含空集群——不再依赖"先有可见
// Topic"）。走通用 ResourceGrantDialog 单资源模式（rmq_cluster + viewer
// 单关系），本页只记录打开的集群目标。
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

// ---------------------------------------------------------------------------
// 集群 CRUD（管理档维护）
// ---------------------------------------------------------------------------
const clusterDialogVisible = ref(false)
const clusterSubmitting = ref(false)
const clusterForm = reactive({ code: '', remark: '' })

function openCreateCluster() {
  clusterForm.code = ''
  clusterForm.remark = ''
  clusterDialogVisible.value = true
}

async function onSubmitCluster() {
  const code = clusterForm.code.trim()
  if (!code) {
    ElMessage.warning('集群编码必填')
    return
  }
  clusterSubmitting.value = true
  try {
    await createCluster({ code, remark: clusterForm.remark.trim() })
    ElMessage.success('创建成功')
    clusterDialogVisible.value = false
    await loadClusters()
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
      `确定删除集群「${target.code}」？集群下仍有 Topic 时会被拒绝。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    await deleteCluster(target.id)
    ElMessage.success('删除成功')
    await loadClusters()
  } catch (err) {
    handleApiError(err)
  }
}

// ---------------------------------------------------------------------------
// Topic 列表与操作
// ---------------------------------------------------------------------------
const topics = ref<RocketmqTopic[]>([])
const loading = ref(false)
const envOptions = ['prod', 'staging', 'dev']

// 组织目录：列表"归属组织"列要显示组织名而非 orgId 码，全量拉一次建映射。
const orgNameMap = ref<Map<string, string>>(new Map())

async function loadOrgs() {
  try {
    const resp = await http.get<{ items: { id: string; name: string }[] }>('/admin/orgs')
    orgNameMap.value = new Map((resp.items ?? []).map((o) => [o.id, o.name]))
  } catch {
    // 组织目录拉取失败不阻断列表：列回退显示原始 orgId 码。
  }
}

/** orgId → 组织名；查不到回退原码（目录未加载/孤儿 id）。 */
function orgName(id: string): string {
  return orgNameMap.value.get(id) ?? id
}

async function load() {
  if (!currentClusterId.value) return
  loading.value = true
  try {
    const resp = await listRocketmqTopics(currentClusterId.value)
    topics.value = resp.items ?? []
  } catch (err) {
    handleApiError(err)
  } finally {
    loading.value = false
  }
}

// 对话框状态
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const editingId = ref('')
const saving = ref(false)
const form = reactive({ name: '', env: '', description: '', clusterId: '', orgId: '' })

function openCreate() {
  dialogMode.value = 'create'
  editingId.value = ''
  form.name = ''
  form.env = ''
  form.description = ''
  form.clusterId = currentClusterId.value
  form.orgId = ''
  dialogVisible.value = true
}

function openEdit(row: RocketmqTopic) {
  dialogMode.value = 'edit'
  editingId.value = row.id
  form.name = row.name
  form.env = row.env
  form.description = row.description
  form.clusterId = row.clusterId
  form.orgId = row.orgId
  dialogVisible.value = true
}

async function onSave() {
  if (!form.name.trim()) {
    ElMessage.warning('Topic 名称必填')
    return
  }
  if (dialogMode.value === 'create' && !form.clusterId) {
    ElMessage.warning('请选择归属集群')
    return
  }
  saving.value = true
  try {
    if (dialogMode.value === 'create') {
      await createRocketmqTopic({
        name: form.name.trim(),
        env: form.env,
        description: form.description,
        clusterId: form.clusterId,
        orgId: form.orgId || undefined,
      })
      ElMessage.success('创建成功')
    } else {
      await updateRocketmqTopic(editingId.value, {
        env: form.env,
        description: form.description,
      })
      ElMessage.success('保存成功')
    }
    dialogVisible.value = false
    await load()
  } catch (err) {
    handleApiError(err)
  } finally {
    saving.value = false
  }
}

async function onDelete(row: RocketmqTopic) {
  try {
    await ElMessageBox.confirm(
      `确定删除 Topic「${row.name}」？删除后不可恢复。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    await deleteRocketmqTopic(row.id)
    ElMessage.success('删除成功')
    await load()
  } catch (err) {
    handleApiError(err)
  }
}

/** 统一错误提示：共享基础能力层 useApiError（错误码文案 + 后端消息）。 */
function handleApiError(err: unknown) {
  const { apiErrorMessage } = useApiError()
  ElMessage.error(apiErrorMessage(err))
}

/** 环境徽标分类：已知环境映射语义色，其余走中性样式。 */
function envClass(env: string): string {
  const e = (env || '').trim().toLowerCase()
  if (e === 'prod' || e === 'production') return 'danger'
  if (e === 'staging' || e === 'stage' || e === 'uat') return 'warning'
  if (e === 'dev') return 'info'
  return 'neutral'
}

onMounted(() => {
  void loadClusters()
  void loadOrgs()
})
</script>

<style scoped>
.page {
  padding: 16px 20px 24px;
}
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.head-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.model-mark {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  cursor: help;
}
.cluster-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.cluster-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.cluster-select {
  width: 260px;
}
.cluster-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.hint-mark {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  cursor: help;
  border-bottom: 1px dashed var(--el-border-color);
}
.table-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 4px;
}
.topic-name {
  font-weight: 500;
}
.env-badge {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--el-color-info-light-9);
  color: var(--el-color-info);
}
.env-danger {
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger);
}
.env-warning {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning);
}
.env-info {
  background: var(--el-color-info-light-9);
  color: var(--el-color-info);
}
.desc-empty {
  color: var(--el-text-color-placeholder);
}
.empty-note {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
