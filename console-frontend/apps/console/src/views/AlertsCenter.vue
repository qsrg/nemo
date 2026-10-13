<template>
  <div class="alerts">
    <div class="page-head">
      <h2>告警中心</h2>
      <p class="sub">
        全部机房的告警与恢复记录
        <span class="demo-tag" title="告警为静态演示数据，后续接入监控服务">演示数据</span>
      </p>
    </div>

    <!-- 筛选行：状态 + 业务域 -->
    <div class="filters">
      <el-radio-group v-model="statusFilter">
        <el-radio-button value="firing">未恢复（{{ countOf('firing') }}）</el-radio-button>
        <el-radio-button value="resolved">已恢复（{{ countOf('resolved') }}）</el-radio-button>
        <el-radio-button value="all">全部（{{ ALERT_ITEMS.length }}）</el-radio-button>
      </el-radio-group>
      <el-select v-model="domainFilter" placeholder="全部业务域" clearable style="width: 160px">
        <el-option v-for="d in domains" :key="d" :value="d" :label="d" />
      </el-select>
    </div>

    <!-- 告警列表：点击行展开详情 -->
    <div class="alert-list">
      <template v-for="a in filtered" :key="a.id">
        <div
          class="alert-item"
          :class="[a.severity, { open: expanded.has(a.id), focus: a.id === focusId }]"
          @click="toggle(a.id)"
        >
          <span class="alert-sev" :title="a.severity === 'critical' ? '严重' : '次要'">
            {{ a.severity === 'critical' ? '严重' : '次要' }}
          </span>
          <span
            class="health-icon sm"
            :style="{ color: domainColor(a.domain).fg, background: domainColor(a.domain).bg }"
          >
            <el-icon><component :is="serviceIcons[a.icon] ?? serviceIcons.Menu" /></el-icon>
          </span>
          <div class="alert-body">
            <div class="alert-title">
              {{ a.title }}
              <span class="status-tag" :class="a.status">
                {{ a.status === 'firing' ? '告警中' : '已恢复' }}
              </span>
            </div>
            <div class="alert-meta">{{ a.service }} · {{ a.region }}机房 · {{ a.domain }}</div>
          </div>
          <div class="alert-time">
            <div>{{ a.startedAt }} 开始</div>
            <div class="sub-time">已持续 {{ a.duration }}</div>
          </div>
          <el-icon class="chev" :class="{ open: expanded.has(a.id) }"><CaretBottom /></el-icon>
        </div>
        <!-- 行内详情：触发条件 / 影响范围 / 处理建议 -->
        <div v-if="expanded.has(a.id)" class="alert-detail">
          <div v-for="d in a.details" :key="d.label" class="detail-row">
            <span class="detail-label">{{ d.label }}</span>
            <span class="detail-value">{{ d.value }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">处理建议</span>
            <span class="detail-value">{{ a.advice }}</span>
          </div>
          <div class="detail-actions">
            <el-button type="primary" size="small" @click="router.push(a.path)">
              去处理
            </el-button>
          </div>
        </div>
      </template>
      <p v-if="!filtered.length" class="empty-hint">当前筛选条件下没有告警</p>
    </div>
  </div>
</template>

<script setup lang="ts">
// 告警中心：全部机房的告警与恢复记录（基座本地页面，静态演示数据）。
// 首页「待关注」条目经 /alerts?focus=<id> 跳入并自动展开定位。
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { CaretBottom } from '@element-plus/icons-vue'
import { ALERT_ITEMS } from '../alerts'
import { serviceIcons, domainColor } from '../serviceIcons'

const route = useRoute()

type StatusFilter = 'firing' | 'resolved' | 'all'

const statusFilter = ref<StatusFilter>('firing')
const domainFilter = ref('')

// 首页带 focus 跳入：切到全部以便定位已恢复条目，并默认展开该条
const focusId = ref('')

const domains = computed(() => [...new Set(ALERT_ITEMS.map((a) => a.domain))])

function countOf(status: 'firing' | 'resolved') {
  return ALERT_ITEMS.filter((a) => a.status === status).length
}

const filtered = computed(() =>
  ALERT_ITEMS.filter(
    (a) =>
      (statusFilter.value === 'all' || a.status === statusFilter.value) &&
      (!domainFilter.value || a.domain === domainFilter.value),
  ),
)

// 展开状态：行内详情（单页少量告警，多开不做手风琴互斥）
const expanded = ref(new Set<string>([]))

function toggle(id: string) {
  const next = new Set(expanded.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expanded.value = next
}

onMounted(() => {
  const focus = typeof route.query.focus === 'string' ? route.query.focus : ''
  if (!focus) return
  const hit = ALERT_ITEMS.find((a) => a.id === focus)
  if (!hit) return
  focusId.value = focus
  statusFilter.value = 'all'
  domainFilter.value = ''
  expanded.value = new Set([focus])
  // 等列表渲染后滚动到定位条目
  requestAnimationFrame(() => {
    document.querySelector('.alert-item.focus')?.scrollIntoView({ block: 'center' })
  })
})
</script>

<style scoped>
.alerts { padding: 24px 28px; max-width: 920px; }
.page-head { margin-bottom: 16px; }
h2 { margin: 0 0 4px; font-size: 20px; }
.sub {
  margin: 0; color: var(--el-text-color-secondary); font-size: 13px;
  display: flex; align-items: center; gap: 8px;
}
.demo-tag {
  font-size: 11px; padding: 1px 6px; border-radius: 4px; flex: none;
  color: var(--el-text-color-secondary); border: 1px dashed var(--el-border-color);
}
.filters { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }

.alert-list {
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light);
  border-radius: 8px; overflow: hidden;
}
.alert-item {
  display: flex; align-items: center; gap: 12px; padding: 12px 16px;
  cursor: pointer; border-bottom: 1px solid var(--el-border-color-lighter);
  box-shadow: inset 3px 0 0 var(--el-color-warning);
}
.alert-item:last-of-type { border-bottom: none; }
.alert-item:hover { background: var(--el-fill-color-light); }
/* 从首页定位进入的条目短暂高亮 */
.alert-item.focus { background: var(--el-color-primary-light-9); }
.alert-item.critical { box-shadow: inset 3px 0 0 var(--el-color-danger); }
.alert-item.open { background: var(--el-fill-color-light); }
.alert-sev {
  flex: none; font-size: 11px; padding: 2px 6px; border-radius: 4px;
  color: var(--el-color-warning); background: var(--el-color-warning-light-9);
}
.critical .alert-sev { color: var(--el-color-danger); background: var(--el-color-danger-light-9); }
.health-icon.sm { width: 28px; height: 28px; border-radius: 6px; font-size: 13px; }
.alert-body { flex: 1; min-width: 0; }
.alert-title { font-size: 13px; font-weight: 500; display: flex; align-items: center; gap: 8px; }
/* 状态徽标：告警中暖色、已恢复冷色 */
.status-tag {
  flex: none; font-size: 11px; padding: 1px 6px; border-radius: 4px; font-weight: 400;
  color: var(--el-color-danger); background: var(--el-color-danger-light-9);
}
.status-tag.resolved { color: var(--el-color-success); background: var(--el-color-success-light-9); }
.alert-meta { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px; }
.alert-time {
  flex: none; text-align: right; font-size: 12px; color: var(--el-text-color-secondary);
  font-variant-numeric: tabular-nums; line-height: 1.6;
}
.sub-time { color: var(--el-text-color-placeholder); }
.chev {
  color: var(--el-text-color-placeholder); font-size: 12px; flex: none;
  transition: transform 0.2s;
}
.chev.open { transform: rotate(180deg); }

/* 行内详情 */
.alert-detail {
  padding: 4px 16px 14px 60px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-lighter);
}
.detail-row { display: flex; gap: 12px; padding: 6px 0; font-size: 13px; }
.detail-label { flex: none; width: 64px; color: var(--el-text-color-secondary); }
.detail-value { flex: 1; min-width: 0; color: var(--el-text-color-primary); }
.detail-actions { padding-top: 10px; }
.empty-hint { margin: 0; padding: 32px 16px; text-align: center; font-size: 13px; color: var(--el-text-color-secondary); }
</style>
