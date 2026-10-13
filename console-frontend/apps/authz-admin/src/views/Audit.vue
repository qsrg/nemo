<template>
  <div class="page">
    <div class="page-head">
      <h3 class="page-title">审计日志</h3>
      <!-- 只读查询页：无任何写操作；查询按 hasPermission('authz-audit:manage') 按钮级门控 -->
      <el-button v-if="canManage" type="primary" :loading="loading" @click="onQuery">查询</el-button>
    </div>

    <!-- 过滤语义提示（审查 B11-①）：操作类型/日期后端不支持、仅当前页内过滤，
         操作人为服务端全量过滤——避免「在过滤全量」的错觉 -->
    <el-alert
      class="filter-hint"
      type="info"
      :closable="false"
      title="操作类型 / 日期仅在当前页内过滤；操作人与定时任务为服务端全量过滤"
    />
    <!-- 过滤栏：操作人走后端过滤（userId 参数）；操作类型/时间范围后端不支持，
         为当前页内客户端过滤（见 fetchAudit 内 filteredEntries） -->
    <el-form inline class="filters" @submit.prevent="onQuery">
      <el-form-item label="操作人">
        <el-input
          v-model="filterOperator"
          placeholder="操作人 ID（精确）"
          clearable
          style="width: 200px"
          @keyup.enter="onQuery"
          @clear="onQuery"
        />
      </el-form-item>
      <el-form-item label="操作类型">
        <el-select v-model="filterAction" placeholder="全部" clearable style="width: 180px">
          <el-option v-for="action in AUDIT_ACTIONS" :key="action" :label="actionLabel(action)" :value="action" />
        </el-select>
      </el-form-item>
      <el-form-item label="日期">
        <el-date-picker
          v-model="filterRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="开始"
          end-placeholder="结束"
          style="width: 240px"
        />
      </el-form-item>
      <!-- 定时同步每周期落一条审计，默认隐藏防刷屏；排查同步问题时可打开 -->
      <el-form-item label="定时任务">
        <el-switch v-model="showSystemJobs" @change="onQuery" active-text="显示" inactive-text="隐藏" />
      </el-form-item>
    </el-form>

    <el-table v-loading="loading" :data="filteredEntries" empty-text="暂无审计记录" @row-click="openDetail">
      <el-table-column label="时间" width="150">
        <template #default="{ row }">
          {{ formatTime(row.createdAt) }}
        </template>
      </el-table-column>
      <el-table-column label="操作人" width="120" show-overflow-tooltip>
        <template #default="{ row }">
          <!-- 姓名反查（B 阶段改造 2.6）：查不到回退裸 ID -->
          {{ operatorName(row.operator) }}
        </template>
      </el-table-column>
      <el-table-column label="动作" width="130">
        <template #default="{ row }">
          <el-tag size="small" :type="row.operator === 'system' ? 'info' : 'primary'">{{ actionLabel(row.action) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="target" label="目标" min-width="150" show-overflow-tooltip />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag size="small" :type="statusType(row.status)">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <!-- 摘要列：解析快照提炼关键字段短句（裸 JSON 在详情抽屉看） -->
      <el-table-column label="摘要" min-width="260" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="snapshot-cell">{{ snapshotSummary(row) }}</span>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="page"
      v-model:page-size="pageSize"
      class="pager"
      layout="total, sizes, prev, pager, next"
      :total="total"
      :page-sizes="[10, 20, 50, 100]"
      @current-change="fetchAudit"
      @size-change="onSizeChange"
    />

    <!-- 快照详情抽屉：JSON pretty 展示（解析失败回退原文）；标题带条目标识（审查 B11-④） -->
    <el-drawer v-model="detailVisible" :title="detail ? `审计详情 · ${detail.action} · ${detail.id}` : '审计详情'" size="480px">
      <template v-if="detail">
        <el-descriptions :column="1" border size="small" class="detail-desc">
          <el-descriptions-item label="ID">{{ detail.id }}</el-descriptions-item>
          <el-descriptions-item label="时间">{{ formatTime(detail.createdAt) }}</el-descriptions-item>
          <el-descriptions-item label="操作人">{{ detail.operator }}</el-descriptions-item>
          <el-descriptions-item label="动作">
            <el-tag size="small">{{ actionLabel(detail.action) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="目标">{{ detail.target }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag size="small" :type="statusType(detail.status)">{{ statusLabel(detail.status) }}</el-tag>
          </el-descriptions-item>
        </el-descriptions>
        <pre class="snapshot-pretty">{{ prettySnapshot(detail.snapshot) }}</pre>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { hasPermission, sessionReady } from '@nemo/shared'
import { baseProps, showApiError } from '../api/client'
import { listAudit, AUDIT_ACTIONS, AUDIT_ACTION_LABELS, type AuditEntry } from '../api/audit'
import { fetchUserNames } from '../api/directory'

/** 按钮级权限：查询仅本菜单 authz-audit:manage 可操作（fail-closed，后端同样 403 兜底）。 */
const permissions = computed(() => baseProps()?.permissions ?? [])
const canManage = computed(() => hasPermission('authz-audit:manage', permissions.value))

// ---------------------------------------------------------------------------
// 列表加载：操作人 + 分页为后端语义；操作类型/日期为当前页内客户端过滤
//（后端 ListAuditReq 仅 page/pageSize/userId 三参数，见 api/audit.ts 契约核对）
// ---------------------------------------------------------------------------
const loading = ref(false)
const entries = ref<AuditEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
/** 裸 userId → 展示名（操作人列渲染用，fetchUserNames 带会话级缓存）。 */
const nameMap = ref<Record<string, string>>({})

/** 裸 userId 的展示名（反查缓存命中显示姓名，否则回退裸 ID）。 */
function operatorName(userId: string): string {
  return nameMap.value[userId] ?? userId
}

/** 服务端过滤参数：操作人（后端参数名 userId）。 */
const filterOperator = ref('')
/** 客户端过滤：操作类型（仅作用于当前页，避免误导「全量过滤」语义）。 */
const filterAction = ref('')
/** 客户端过滤：日期范围（按 createdAt 的日期部分）。 */
const filterRange = ref<[string, string] | null>(null)
/** 定时任务（system 操作人的周期审计）默认隐藏防刷屏；服务端过滤（切换即重查）。 */
const showSystemJobs = ref(false)

/** 动作可读名（AUDIT_ACTION_LABELS 未收录时原样展示）。 */
function actionLabel(action: string): string {
  return AUDIT_ACTION_LABELS[action] ?? action
}

/** 快照摘要：解析 JSON 取关键 key=value 短句（详情抽屉有完整格式化 JSON）。 */
function snapshotSummary(row: AuditEntry): string {
  let obj: Record<string, unknown>
  try {
    obj = JSON.parse(row.snapshot)
  } catch {
    return row.snapshot
  }
  const parts: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    if (v === null || v === undefined || v === '' || typeof v === 'object') continue
    parts.push(`${k}=${String(v).slice(0, 40)}`)
    if (parts.length >= 3) break
  }
  return parts.join('　') || row.target
}

const filteredEntries = computed(() =>
  entries.value.filter((e) => {
    if (filterAction.value && e.action !== filterAction.value) return false
    if (filterRange.value && filterRange.value.length === 2) {
      // 日期串（YYYY-MM-DD）直接字典序比较：两端同为本地日期切片，无需换算时区
      const day = e.createdAt.slice(0, 10)
      if (day < filterRange.value[0] || day > filterRange.value[1]) return false
    }
    return true
  }),
)

async function fetchAudit() {
  loading.value = true
  try {
    const resp = await listAudit({
      page: page.value,
      pageSize: pageSize.value,
      userId: filterOperator.value.trim() || undefined,
      excludeSystem: !showSystemJobs.value,
    })
    entries.value = resp.items
    total.value = resp.total
    // 姓名反查（B 阶段改造 2.6）：操作人裸 userId → 展示名（查不到回退裸 ID）。
    const operators = [...new Set(resp.items.map((e) => e.operator))]
    if (operators.length > 0) {
      const names = await fetchUserNames(operators)
      nameMap.value = { ...nameMap.value, ...names }
    }
    // 越界页回夹（审查 B11-③）：数据收缩（如过滤后）当前页超出总页数时回末页
    const maxPage = Math.max(1, Math.ceil(resp.total / pageSize.value))
    if (page.value > maxPage) {
      page.value = maxPage
      return fetchAudit()
    }
  } catch (err) {
    showApiError(err)
  } finally {
    loading.value = false
  }
}

/** 查询按钮：应用操作人过滤并回到第一页。 */
function onQuery() {
  page.value = 1
  void fetchAudit()
}

/** 换每页条数：回到第一页重查（el-pagination 的 size-change 已同步 v-model）。 */
function onSizeChange() {
  page.value = 1
  void fetchAudit()
}

// ---------------------------------------------------------------------------
// 详情抽屉
// ---------------------------------------------------------------------------
const detailVisible = ref(false)
const detail = ref<AuditEntry | null>(null)

function openDetail(row: AuditEntry) {
  detail.value = row
  detailVisible.value = true
}

function prettySnapshot(snapshot: string): string {
  try {
    return JSON.stringify(JSON.parse(snapshot), null, 2)
  } catch {
    return snapshot
  }
}

// ---------------------------------------------------------------------------
// 展示辅助
// ---------------------------------------------------------------------------
function formatTime(rfc3339: string): string {
  const d = new Date(rfc3339)
  if (Number.isNaN(d.getTime())) return rfc3339
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const STATUS_META: Record<string, { type: 'warning' | 'success' | 'danger' | 'info'; label: string }> = {
  pending: { type: 'warning', label: '待提交' },
  committed: { type: 'success', label: '已提交' },
  failed: { type: 'danger', label: '失败' },
}

function statusType(status: string) {
  return STATUS_META[status]?.type ?? 'info'
}

function statusLabel(status: string): string {
  return STATUS_META[status]?.label ?? status
}

onMounted(async () => {
  await sessionReady() // iframe 握手竞态门控
  void fetchAudit()
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
  margin-bottom: 4px;
}
.page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.filters {
  margin-bottom: 4px;
}
.filter-hint {
  margin-bottom: 8px;
}
.filters :deep(.el-form-item) {
  margin-bottom: 8px;
}
.snapshot-cell {
  font-family: monospace;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.pager {
  margin-top: 12px;
  justify-content: flex-end;
}
.detail-desc {
  margin-bottom: 12px;
}
/* 行可点开抽屉（审查 B11-④）：与静态表格行作视觉区分 */
:deep(.el-table__row) {
  cursor: pointer;
}
.snapshot-pretty {
  margin: 0;
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 60vh;
  overflow: auto;
}
</style>
