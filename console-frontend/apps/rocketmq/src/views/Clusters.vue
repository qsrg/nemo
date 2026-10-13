<template>
  <div class="page">
    <header class="page-head">
      <div class="head-left">
        <h3 class="page-title">集群管理</h3>
      </div>
      <!-- 权限身份胶囊：集群页只有 read/manage 两档（无按钮动作码），
           管理档持有全部写操作，所见即所得。 -->
      <span v-if="canManage" class="perm-pill grant">可管理 · 增删改</span>
      <span v-else class="perm-pill readonly">可查看</span>
    </header>

    <div class="toolbar">
      <el-button v-if="canManage" type="primary" @click="openCreate">新建集群</el-button>
      <el-tooltip placement="bottom-start">
        <template #content>
          集群为基础配置数据 + 授权锚点：集群可见即可查看该集群全部 Topic 并可操作；<br />
          新建/删除集群由管理档维护；授权名单在行内「权限」配置（owner 或管理码）。
        </template>
        <span class="hint-mark">权限说明</span>
      </el-tooltip>
    </div>

    <section class="table-card">
      <el-table v-loading="loading" :data="clusters" class="topic-table">
        <template #empty>
          <el-empty :image-size="72" description="暂无集群">
            <p class="empty-note">集群为基础配置数据，由管理档用户维护</p>
          </el-empty>
        </template>
        <el-table-column prop="code" label="集群编码" min-width="140" />
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
        <el-table-column prop="createdBy" label="创建人" width="100" />
        <el-table-column prop="updatedAt" label="更新时间" width="170" />
        <el-table-column label="操作" width="170" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="canManage || row.canManageGrants"
              link
              type="primary"
              @click="openGrants(row)"
            >
              权限
            </el-button>
            <template v-if="canManage">
              <el-button link type="danger" @click="onDelete(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 新建集群对话框 -->
    <el-dialog v-model="dialogVisible" title="新建集群" width="480px">
      <el-form label-width="80px" @submit.prevent="onSubmit">
        <el-form-item label="集群编码" required>
          <el-input v-model="form.code" maxlength="64" placeholder="集群唯一编码，如 rmq-prod" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" placeholder="用途说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit">创建</el-button>
      </template>
    </el-dialog>

    <!-- 集群权限对话框：通用资源授权组件（基础能力层）——资源类型
         rmq_cluster + viewer 单关系。owner 或管理码可配置。 -->
    <ResourceGrantDialog
      v-model="grantVisible"
      resource-type="rmq_cluster"
      :resource-id="grantId"
      :resource-name="grantName"
      manageable
      :title="`集群权限：${grantName}`"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ResourceGrantDialog, useApiError, usePermission } from '@nemo/shared'
import {
  createCluster,
  deleteCluster,
  listClusters,
  type RocketmqCluster,
} from '../api/rocketmq'

const MANAGE_CODE = 'rocketmq-topics:manage'
const { has } = usePermission()
const canManage = computed(() => has(MANAGE_CODE))

const clusters = ref<RocketmqCluster[]>([])
const loading = ref(false)

async function loadClusters() {
  loading.value = true
  try {
    const resp = await listClusters()
    clusters.value = resp.items
  } catch (err) {
    handleApiError(err)
  } finally {
    loading.value = false
  }
}

// 新建集群
const dialogVisible = ref(false)
const submitting = ref(false)
const form = reactive({ code: '', remark: '' })

function openCreate() {
  form.code = ''
  form.remark = ''
  dialogVisible.value = true
}

async function onSubmit() {
  const code = form.code.trim()
  if (!code) {
    ElMessage.warning('集群编码必填')
    return
  }
  submitting.value = true
  try {
    await createCluster({ code, remark: form.remark.trim() })
    ElMessage.success('创建成功')
    dialogVisible.value = false
    await loadClusters()
  } catch (err) {
    handleApiError(err)
  } finally {
    submitting.value = false
  }
}

async function onDelete(row: RocketmqCluster) {
  try {
    await ElMessageBox.confirm(
      `确定删除集群「${row.code}」？集群下仍有 Topic 时会被拒绝。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    await deleteCluster(row.id)
    ElMessage.success('删除成功')
    await loadClusters()
  } catch (err) {
    handleApiError(err)
  }
}

// 集群权限
const grantVisible = ref(false)
const grantId = ref('')
const grantName = ref('')

function openGrants(row: RocketmqCluster) {
  grantId.value = row.id
  grantName.value = row.code
  grantVisible.value = true
}

function handleApiError(err: unknown) {
  const { apiErrorMessage } = useApiError()
  ElMessage.error(apiErrorMessage(err))
}

onMounted(loadClusters)
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
.perm-pill {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 10px;
}
.perm-pill.grant {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
}
.perm-pill.readonly {
  background: var(--el-color-info-light-9);
  color: var(--el-color-info);
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
.empty-note {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
