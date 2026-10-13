<template>
  <div class="page">
    <div class="page-head">
      <h3 class="page-title">资源属性</h3>
      <span class="demo-tag" title="ABAC 方案 A：鉴权属性统一存 authz 引擎，业务表不再承载鉴权门槛字段">
        主体 / 资源属性 · ABAC 方案 A
      </span>
    </div>

    <el-tabs v-model="tab">
      <el-tab-pane label="资源属性" name="resource">
        <div class="attr-toolbar">
          <el-input
            v-model="resourceInput"
            class="owner-input"
            maxlength="128"
            placeholder="资源标识，如 topic:1（类型:id）"
            @keyup.enter="loadAttrs"
          />
          <el-button type="primary" :loading="loading" @click="loadAttrs">查询</el-button>
        </div>
      </el-tab-pane>
      <el-tab-pane label="主体属性" name="subject">
        <div class="attr-toolbar">
          <el-input
            v-model="subjectInput"
            class="owner-input"
            maxlength="128"
            placeholder="主体标识，如 user:zhangsan / org:tech-frontend#member"
            @keyup.enter="loadAttrs"
          />
          <el-button type="primary" :loading="loading" @click="loadAttrs">查询</el-button>
        </div>
      </el-tab-pane>
    </el-tabs>

    <div v-if="ownerLoaded" class="attr-editor">
      <div class="attr-editor-head">
        <span class="muted">属性编辑（值存字符串；数字比较由策略层转换）</span>
        <el-button link type="primary" @click="addRow">+ 新增属性</el-button>
      </div>
      <el-table :data="rows" size="default" empty-text="暂无属性（点击上方「查询」加载）">
        <el-table-column label="属性键" width="240">
          <template #default="{ row }">
            <el-input v-model="row.key" maxlength="64" placeholder="如 securityLevel" />
          </template>
        </el-table-column>
        <el-table-column label="属性值">
          <template #default="{ row }">
            <el-input v-model="row.value" maxlength="256" placeholder="如 3" />
          </template>
        </el-table-column>
        <el-table-column label="" width="90">
          <template #default="{ row, $index }">
            <el-button link type="danger" :disabled="!row.key || !savedKeys.has(row.key)" @click="removeRow($index, row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="attr-editor-foot">
        <el-button type="primary" :loading="saving" :disabled="!rows.length" @click="saveAll">保存全部</el-button>
        <span class="muted">保存 = 对每个键幂等覆盖；删除 = 立即删除该键</span>
      </div>
      <p class="page-note">
        属性示例：资源 <code>topic:1</code> 设 <code>securityLevel=4</code>、主体
        <code>user:zhangsan</code> 设 <code>securityLevel=2</code>，配合「ABAC 策略」页的
        <code>user.securityLevel &lt; resource.securityLevel</code> 策略即可复现旧「最低等级」效果。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  deleteResourceAttribute,
  deleteSubjectAttribute,
  getResourceAttributes,
  getSubjectAttributes,
  setResourceAttributes,
  setSubjectAttributes,
  type AttrMap,
} from '../api/attributes'
import { showApiError } from '../api/client'

/** 主体/资源切换：主体 = user:/org: 前缀；资源 = 类型:id。 */
const tab = ref<'subject' | 'resource'>('resource')
const subjectInput = ref('')
const resourceInput = ref('')

const loading = ref(false)
const saving = ref(false)
/** 当前已加载的 owner（非空即展示编辑器）。 */
const ownerLoaded = ref('')

/** 编辑行（key/value）。 */
interface Row {
  key: string
  value: string
  _saved: boolean
}
const rows = ref<Row[]>([])
/** 已落库的键（删除按钮仅对已存在键可用）。 */
const savedKeys = computed(() => new Set(rows.value.filter((r) => r._saved).map((r) => r.key)))

function owner() {
  return tab.value === 'subject' ? subjectInput.value.trim() : resourceInput.value.trim()
}

async function loadAttrs() {
  const o = owner()
  if (!o) {
    ElMessage.warning('请输入主体/资源标识')
    return
  }
  loading.value = true
  try {
    const attrs: AttrMap =
      tab.value === 'subject'
        ? (await getSubjectAttributes(o)).attrs
        : (await getResourceAttributes(o)).attrs
    rows.value = Object.entries(attrs).map(([key, value]) => ({ key, value, _saved: true }))
    ownerLoaded.value = o
  } catch (err) {
    showApiError(err)
    ownerLoaded.value = ''
  } finally {
    loading.value = false
  }
}

function addRow() {
  rows.value.push({ key: '', value: '', _saved: false })
}

/** 删除：仅删除已落库键（立即调后端）；未保存新行直接从本地移除。 */
async function removeRow(index: number, row: Row) {
  const o = ownerLoaded.value
  if (!o) return
  if (row._saved) {
    try {
      if (tab.value === 'subject') {
        await deleteSubjectAttribute(o, row.key)
      } else {
        await deleteResourceAttribute(o, row.key)
      }
      ElMessage.success(`已删除 ${row.key}`)
    } catch (err) {
      showApiError(err)
      return
    }
  }
  rows.value.splice(index, 1)
}

/** 保存全部：对每个已填键做覆盖式 upsert（未传键保留）。 */
async function saveAll() {
  const o = ownerLoaded.value
  if (!o) return
  const attrs: AttrMap = {}
  for (const r of rows.value) {
    const key = r.key.trim()
    if (!key) continue
    attrs[key] = r.value
  }
  if (!Object.keys(attrs).length) {
    ElMessage.warning('请至少填写一个属性键')
    return
  }
  saving.value = true
  try {
    if (tab.value === 'subject') {
      await setSubjectAttributes(o, attrs)
    } else {
      await setResourceAttributes(o, attrs)
    }
    ElMessage.success('已保存')
    await loadAttrs()
  } catch (err) {
    showApiError(err)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.attr-toolbar {
  display: flex;
  gap: 8px;
  margin: 4px 0 12px;
}
.owner-input {
  max-width: 360px;
}
.attr-editor {
  margin-top: 8px;
}
.attr-editor-head,
.attr-editor-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.muted {
  color: #909399;
  font-size: 12px;
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
