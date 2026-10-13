<template>
  <div class="page">
    <div class="page-head">
      <h3 class="page-title">ABAC 策略</h3>
      <span
        class="demo-tag"
        title="deny 拒绝优先于一切授权；allow 正向门槛——适用资源须命中至少一条才放行。两者叠加于 ReBAC 授权之上"
      >
        deny 拒绝 · allow 门槛 · 叠加于授权
      </span>
      <el-button v-if="canManage" type="primary" @click="openCreate">新建策略</el-button>
    </div>

    <el-table v-loading="loading" :data="policies" size="default" empty-text="暂无策略（默认全放行）">
      <el-table-column prop="name" label="策略名" min-width="140" />
      <el-table-column label="条件" min-width="260">
        <template #default="{ row }">
          <code class="cond">{{ conditionText(row) }}</code>
        </template>
      </el-table-column>
      <el-table-column prop="resourceType" label="资源类型" width="100">
        <template #default="{ row }">{{ row.resourceType || '全部' }}</template>
      </el-table-column>
      <el-table-column label="效果" width="110">
        <template #default="{ row }">
          <el-tag v-if="row.effect === 'allow'" size="small" type="success">门槛</el-tag>
          <el-tag v-else size="small" type="danger">拒绝</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="90">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" :loading="togglingId === row.id" @change="(v: boolean) => onToggle(row, v)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150">
        <template #default="{ row }">
          <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
          <el-button link type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <p class="page-note">
      策略叠加在资源鉴权链路（ReBAC 授权之上）：
      <b>拒绝（deny）</b>——条件命中即拒绝，优先于一切授权（含管理码持有者）；
      <b>门槛（allow）</b>——适用的资源须至少命中一条才放行，表达「只有 trade 业务线的人可看」这类正向属性门槛，属性不满足或没配属性的人一律被挡。
      「资源属性」形式的门槛仅对设置了该属性的资源生效（按资源开门槛）；「字面量」形式对整个资源类型生效。
      deny 优先于 allow；缺属性视为不命中；enabled 开关即回滚开关——关闭全部策略即回到纯 RBAC+ReBAC。
    </p>

    <!-- 新建/编辑共用对话框 -->
    <el-dialog v-model="dialogVisible" :title="editing ? '编辑策略' : '新建策略'" width="560px">
      <el-form label-width="108px" @submit.prevent="onSubmit">
        <el-form-item label="策略名" required>
          <el-input v-model="form.name" maxlength="128" placeholder="如 高安全等级限制" />
        </el-form-item>
        <el-form-item label="效果" required>
          <el-radio-group v-model="form.effect">
            <el-radio-button value="deny">拒绝</el-radio-button>
            <el-radio-button value="allow">门槛</el-radio-button>
          </el-radio-group>
          <span class="muted">
            {{ form.effect === 'allow' ? '适用的资源须命中条件才可访问（正向门槛）' : '条件命中即拒绝（优先于一切授权）' }}
          </span>
        </el-form-item>
        <el-form-item label="资源类型">
          <el-input v-model="form.resourceType" maxlength="64" placeholder="如 topic；留空 = 全部资源类型" />
        </el-form-item>
        <el-form-item label="主体属性" required>
          <el-input v-model="form.subjectAttr" maxlength="64" placeholder="如 securityLevel / bizLine" />
        </el-form-item>
        <el-form-item label="操作符" required>
          <el-select v-model="form.op" class="op-select">
            <el-option v-for="op in OPTS" :key="op" :value="op" :label="op" />
          </el-select>
        </el-form-item>
        <el-form-item label="资源属性">
          <el-input v-model="form.resourceAttr" maxlength="64" placeholder="与「字面量」二选一；如 securityLevel" />
        </el-form-item>
        <el-form-item label="字面量">
          <el-input v-model="form.value" maxlength="256" placeholder="资源属性留空时与主体属性比较；如 trade" />
        </el-form-item>
        <el-form-item label="立即启用">
          <el-switch v-model="form.enabled" />
          <span class="muted">启用即参与鉴权（建议先建后开，确认无误再开）</span>
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
import { hasPermission, sessionReady } from '@nemo/shared'
import { baseProps, showApiError } from '../api/client'
import { createPolicy, deletePolicy, listPolicies, updatePolicy, type Policy } from '../api/policies'

/** 操作符白名单（后端校验同源：仅 lt/lte/gt/gte/eq/neq）。 */
const OPTS = ['lt', 'lte', 'gt', 'gte', 'eq', 'neq']

/** 按钮级权限：策略管理仅本菜单 authz-policies:manage 可操作。 */
const permissions = computed(() => baseProps()?.permissions ?? [])
const canManage = computed(() => hasPermission('authz-policies:manage', permissions.value))

const loading = ref(false)
const policies = ref<Policy[]>([])
const togglingId = ref('')

async function loadPolicies() {
  loading.value = true
  try {
    const resp = await listPolicies()
    policies.value = resp.items
  } catch (err) {
    showApiError(err)
  } finally {
    loading.value = false
  }
}

/** 条件展示：subjectAttr op (resourceAttr | value)。 */
function conditionText(row: Policy): string {
  const right = row.resourceAttr ? `resource.${row.resourceAttr}` : `"${row.value}"`
  return `user.${row.subjectAttr} ${row.op} ${right}`
}

onMounted(async () => {
  await sessionReady()
  await loadPolicies()
})

// ---------------------------------------------------------------------------
// 新建 / 编辑
// ---------------------------------------------------------------------------
const dialogVisible = ref(false)
const editing = ref(false)
const submitting = ref(false)
const form = reactive({
  name: '',
  effect: 'deny',
  resourceType: '',
  subjectAttr: '',
  op: 'lt',
  resourceAttr: '',
  value: '',
  enabled: false,
})

function openCreate() {
  editing.value = false
  Object.assign(form, { name: '', effect: 'deny', resourceType: '', subjectAttr: '', op: 'lt', resourceAttr: '', value: '', enabled: false })
  dialogVisible.value = true
}

function openEdit(row: Policy) {
  editing.value = true
  Object.assign(form, {
    name: row.name,
    effect: row.effect === 'allow' ? 'allow' : 'deny',
    resourceType: row.resourceType,
    subjectAttr: row.subjectAttr,
    op: row.op,
    resourceAttr: row.resourceAttr,
    value: row.value,
    enabled: row.enabled,
  })
  dialogVisible.value = true
}

async function onSubmit() {
  if (!form.name.trim() || !form.subjectAttr.trim()) {
    ElMessage.warning('策略名与主体属性必填')
    return
  }
  submitting.value = true
  const payload = {
    name: form.name.trim(),
    effect: form.effect,
    resourceType: form.resourceType.trim(),
    subjectAttr: form.subjectAttr.trim(),
    op: form.op,
    resourceAttr: form.resourceAttr.trim(),
    value: form.value,
    enabled: form.enabled,
  }
  try {
    if (editing.value) {
      await updatePolicy(currentId, payload)
      ElMessage.success('已保存')
    } else {
      await createPolicy(payload)
      ElMessage.success('已创建')
    }
    dialogVisible.value = false
    await loadPolicies()
  } catch (err) {
    showApiError(err)
  } finally {
    submitting.value = false
  }
}

let currentId = ''

/** 行内启用开关（幂等保存 enabled 字段）。 */
async function onToggle(row: Policy, v: boolean) {
  togglingId.value = row.id
  try {
    await updatePolicy(row.id, {
      name: row.name,
      resourceType: row.resourceType,
      effect: row.effect,
      subjectAttr: row.subjectAttr,
      op: row.op,
      resourceAttr: row.resourceAttr,
      value: row.value,
      enabled: v,
    })
    row.enabled = v
    ElMessage.success(v ? '已启用（即刻参与鉴权）' : '已停用（不再评估）')
  } catch (err) {
    showApiError(err)
  } finally {
    togglingId.value = ''
  }
}

async function onDelete(row: Policy) {
  try {
    await ElMessageBox.confirm(`确认删除策略「${row.name}」？删除后立即不再评估。`, '删除策略', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deletePolicy(row.id)
    ElMessage.success('已删除')
    await loadPolicies()
  } catch (err) {
    showApiError(err)
  }
}
</script>

<style scoped>
.cond {
  background: #f5f7fa;
  border-radius: 4px;
  padding: 2px 6px;
  font-size: 12px;
}
.op-select {
  width: 120px;
}
.muted {
  color: #909399;
  font-size: 12px;
  margin-left: 8px;
}
.demo-tag {
  color: #909399;
  font-size: 12px;
}
.page-note {
  color: #909399;
  font-size: 12px;
  margin-top: 12px;
}
</style>
