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
    </div>

    <section class="table-card">
      <el-table v-loading="loading" :data="clusters" class="topic-table">
        <template #empty>
          <el-empty :image-size="72" description="暂无集群">
            <p class="empty-note">集群为基础配置数据，由管理档用户维护</p>
          </el-empty>
        </template>
        <el-table-column prop="code" label="集群编码" min-width="140" />
        <el-table-column prop="dc" label="机房" min-width="120" />
        <el-table-column prop="address" label="地址" min-width="180" show-overflow-tooltip />
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
        <el-table-column prop="createdBy" label="创建人" width="100" />
        <el-table-column prop="updatedAt" label="更新时间" width="170" />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <template v-if="canManage">
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button link type="danger" @click="onDelete(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <!-- 分页：服务端分页 -->
    <div class="pager-row">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        background
        @current-change="loadClusters"
        @size-change="onSizeChange"
      />
    </div>

    <!-- 新建/编辑共用对话框（编辑不传 code：对象身份不可变） -->
    <el-dialog v-model="dialogVisible" :title="editing ? '编辑集群' : '新建集群'" width="520px">
      <el-form label-width="96px" @submit.prevent="onSubmit">
        <el-form-item label="集群编码" required>
          <el-input v-model="form.code" :disabled="editing" maxlength="64" placeholder="如 prod-ali-bj" />
        </el-form-item>
        <el-form-item label="机房">
          <el-input v-model="form.dc" maxlength="64" placeholder="如 北京·阿里" />
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.address" maxlength="255" placeholder="如 bj-ali-001" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" maxlength="512" type="textarea" :rows="2" />
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
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useApiError, usePermission } from '@nemo/shared'
import {
  createCluster,
  deleteCluster,
  listClusters,
  updateCluster,
  type KafkaCluster,
} from '../api/clusters'

/** 菜单管理码：read 控菜单可见性（页面能进来即已授），manage 控全部写操作
 *  （新建/编辑/删除，无按钮动作码——集群是基础配置数据）。 */
const SERVICE = 'kafka-clusters'

/** 当前用户是否持有 manage 码（基座 session.permissions 同步，实时刷新）。
 *  usePermission.canManage 是 (service)=>boolean 函数，须包 computed 才能
 *  供模板 v-if / prop 布尔使用（裸函数恒 truthy）。 */
const { canManage: hasManageCode } = usePermission()
const canManage = computed(() => hasManageCode(SERVICE))

/** 统一错误提示（403 → 无权限提示，其余透出后端 message）。 */
const handleApiError = useApiError()

const loading = ref(false)
const submitting = ref(false)
const clusters = ref<KafkaCluster[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

/** 分页条数变化回到第一页（与列表页同口径）。 */
function onSizeChange() {
  page.value = 1
  loadClusters()
}

/** 拉取集群列表。 */
async function loadClusters() {
  loading.value = true
  try {
    const resp = await listClusters(page.value, pageSize.value)
    clusters.value = resp.items
    total.value = resp.total
  } catch (e) {
    handleApiError(e)
  } finally {
    loading.value = false
  }
}

const dialogVisible = ref(false)
const editing = ref(false)
const editingId = ref(0)
const form = reactive({ code: '', dc: '', address: '', remark: '' })

/** 打开新建对话框。 */
function openCreate() {
  editing.value = false
  editingId.value = 0
  form.code = ''
  form.dc = ''
  form.address = ''
  form.remark = ''
  dialogVisible.value = true
}

/** 打开编辑对话框（code 置灰不可改：对象身份不可变）。 */
function openEdit(row: KafkaCluster) {
  editing.value = true
  editingId.value = row.id
  form.code = row.code
  form.dc = row.dc
  form.address = row.address
  form.remark = row.remark
  dialogVisible.value = true
}

/** 提交：新建必填 code（后端 409 兜底重名）；编辑不传 code。 */
async function onSubmit() {
  if (!form.code.trim() && !editing.value) {
    ElMessage.warning('集群编码不能为空')
    return
  }
  submitting.value = true
  try {
    if (editing.value) {
      await updateCluster(editingId.value, {
        dc: form.dc.trim(),
        address: form.address.trim(),
        remark: form.remark.trim(),
      })
      ElMessage.success('已保存')
    } else {
      await createCluster({
        code: form.code.trim(),
        dc: form.dc.trim(),
        address: form.address.trim(),
        remark: form.remark.trim(),
      })
      ElMessage.success('已创建')
      page.value = 1
    }
    dialogVisible.value = false
    await loadClusters()
  } catch (e) {
    handleApiError(e)
  } finally {
    submitting.value = false
  }
}

/** 删除确认（ElMessageBox 二次确认后调用接口）。 */
async function onDelete(row: KafkaCluster) {
  try {
    await ElMessageBox.confirm(`确认删除集群「${row.code}」？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return // 用户取消
  }
  try {
    await deleteCluster(row.id)
    ElMessage.success('已删除')
    await loadClusters()
  } catch (e) {
    handleApiError(e)
  }
}

onMounted(loadClusters)
</script>
