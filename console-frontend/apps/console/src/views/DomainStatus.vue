<template>
  <div class="domain-status">
    <div class="page-head">
      <button class="back" type="button" @click="router.push('/workbench')">
        <el-icon><ArrowLeft /></el-icon>
        返回工作台
      </button>
      <div class="head-row">
        <span
          class="health-icon"
          :style="{ color: domainColor(domain).fg, background: domainColor(domain).bg }"
        >
          <el-icon><component :is="domainIcon(domain)" /></el-icon>
        </span>
        <h2>{{ domain }}</h2>
        <span class="demo-tag" title="态势为静态演示数据，后续接入监控服务">演示数据</span>
      </div>
      <p v-if="info" class="sub">
        集群 {{ info.clusterCount }} · 实例 {{ info.instances }} · 部署 {{ info.regionStrip.length }} 个机房 ·
        <span :class="{ 'warn-text': info.alarmCount }">未恢复告警 {{ info.alarmCount }}</span>
      </p>
    </div>

    <template v-if="info">
      <!-- 服务区块：有未恢复告警的置顶并默认展开机房明细，健康服务默认收起 -->
      <section v-for="s in orderedServices" :key="s.key" class="svc">
        <div class="svc-head" @click="toggle(s.key)">
          <span
            class="health-icon sm"
            :style="{ color: domainColor(s.domain).fg, background: domainColor(s.domain).bg }"
          >
            <el-icon><component :is="serviceIcons[s.icon] ?? serviceIcons.Menu" /></el-icon>
          </span>
          <div class="svc-title">
            <span class="svc-name">{{ s.title }}</span>
            <span class="status-tag" :class="s.alarmCount ? 'firing' : 'ok'">
              {{ s.alarmCount ? `告警中 ${s.alarmCount}` : '正常' }}
            </span>
          </div>
          <div class="svc-meta">集群 {{ s.clusterCount }} · 实例 {{ s.instances }} · {{ s.regions.length }} 机房</div>
          <el-icon class="chev" :class="{ open: expanded.has(s.key) }"><CaretBottom /></el-icon>
        </div>

        <!-- 机房明细：服务 × 机房部署矩阵，机房行可再下钻集群 -->
        <table v-if="expanded.has(s.key)" class="region-table">
          <thead>
            <tr>
              <th class="expand-cell"></th>
              <th>机房</th>
              <th>集群</th>
              <th>实例</th>
              <th>状态</th>
              <th>未恢复告警</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="r in s.regions" :key="r.name">
              <tr class="region-row" @click="toggleRegion(s.key, r.name)">
                <td class="expand-cell">
                  <el-icon class="chev sm" :class="{ open: expandedRegions.has(s.key + ':' + r.name) }">
                    <CaretBottom />
                  </el-icon>
                </td>
                <td>{{ r.name }}</td>
                <td class="num">{{ r.clusterCount }}</td>
                <td class="num">{{ r.instances }}</td>
                <td>
                  <span class="dot" :class="r.alerts.length ? 'warn' : 'ok'" />
                  {{ r.alerts.length ? '告警' : '正常' }}
                </td>
                <td>
                  <button
                    v-for="a in r.alerts"
                    :key="a.id"
                    class="alert-link"
                    :class="{ 'critical-link': a.severity === 'critical' }"
                    type="button"
                    :title="`查看告警详情：${a.title}`"
                    @click.stop="router.push({ path: '/alerts', query: { focus: a.id } })"
                  >
                    {{ a.title }}
                  </button>
                  <span v-if="!r.alerts.length" class="muted">—</span>
                </td>
              </tr>
              <!-- 集群明细：该机房的全部集群 -->
              <tr v-if="expandedRegions.has(s.key + ':' + r.name)" class="cluster-row">
                <td colspan="6" class="cluster-cell">
                  <table class="cluster-table">
                    <thead>
                      <tr>
                        <th>集群</th>
                        <th>实例</th>
                        <th>状态</th>
                        <th>未恢复告警</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="c in r.clusters" :key="c.name">
                        <td class="mono">{{ c.name }}</td>
                        <td class="num">{{ c.instances }}</td>
                        <td>
                          <span class="dot" :class="c.alerts.length ? 'warn' : 'ok'" />
                          {{ c.alerts.length ? '告警' : '正常' }}
                        </td>
                        <td>
                          <button
                            v-for="a in c.alerts"
                            :key="a.id"
                            class="alert-link"
                            :class="{ 'critical-link': a.severity === 'critical' }"
                            type="button"
                            @click.stop="router.push({ path: '/alerts', query: { focus: a.id } })"
                          >
                            {{ a.title }}
                          </button>
                          <span v-if="!c.alerts.length" class="muted">—</span>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </section>
    </template>
    <p v-else class="empty-hint">没有找到「{{ domain }}」的业务域，<button class="link" type="button" @click="router.push('/workbench')">返回工作台</button></p>
  </div>
</template>

<script setup lang="ts">
// 域态势页：首页态势卡的下钻落点，按生产多机房模型展示
// 「域 → 服务 → 机房」三级健康（基座本地页面，静态演示数据）。
// 有未恢复告警的服务置顶并默认展开机房明细，健康服务默认收起。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, CaretBottom } from '@element-plus/icons-vue'
import { serviceIcons, domainColor, domainIcon } from '../serviceIcons'
import { domainHealthOf } from '../health'
import { useMenusStore } from '../stores/menus'

const route = useRoute()
const router = useRouter()

// 服务可见键集（me/menus 服务端过滤口径）：域态势页与菜单/首页同一套可见性，
// 无权限服务不展示（域内可见服务为空时按未找到处理）
const menusStore = useMenusStore()
const visibleKeys = computed(() => new Set(menusStore.items.map((s) => s.key)))

const domain = computed(() => String(route.params.domain ?? ''))
const info = computed(() => domainHealthOf(domain.value, visibleKeys.value))

// 异常优先排序：有未恢复告警的服务在前，其余保持注册表顺序
const orderedServices = computed(() =>
  info.value
    ? [...info.value.services].sort((a, b) => b.alarmCount - a.alarmCount)
    : [],
)

// 展开状态：有告警的服务默认展开，用户可手动收起/展开
const expanded = ref(new Set<string>())

// 机房行下钻集群明细：`服务key:机房名` 为键
const expandedRegions = ref(new Set<string>())

function toggle(key: string) {
  const next = new Set(expanded.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expanded.value = next
}

function toggleRegion(serviceKey: string, region: string) {
  const id = `${serviceKey}:${region}`
  const next = new Set(expandedRegions.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedRegions.value = next
}

onMounted(() => {
  expanded.value = new Set(orderedServices.value.filter((s) => s.alarmCount).map((s) => s.key))
})
</script>

<style scoped>
.domain-status { padding: 24px 28px; max-width: 920px; }
.page-head { margin-bottom: 20px; }
.back {
  display: inline-flex; align-items: center; gap: 4px; margin-bottom: 12px;
  border: none; background: none; padding: 4px 8px 4px 0; font-size: 13px;
  color: var(--el-text-color-secondary); cursor: pointer;
}
.back:hover { color: var(--el-color-primary); }
.head-row { display: flex; align-items: center; gap: 12px; }
h2 { margin: 0; font-size: 20px; }
.sub { margin: 10px 0 0; font-size: 13px; color: var(--el-text-color-secondary); }
.warn-text { color: var(--el-color-warning); font-weight: 600; }
.demo-tag {
  font-size: 11px; padding: 1px 6px; border-radius: 4px;
  color: var(--el-text-color-secondary); border: 1px dashed var(--el-border-color);
}

.health-icon {
  width: 36px; height: 36px; border-radius: 8px; flex: none;
  display: inline-flex; align-items: center; justify-content: center; font-size: 17px;
}
.health-icon.sm { width: 28px; height: 28px; border-radius: 6px; font-size: 13px; }

/* —— 服务区块 —— */
.svc {
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light);
  border-radius: 8px; margin-bottom: 12px; overflow: hidden;
}
.svc-head {
  display: flex; align-items: center; gap: 12px; padding: 12px 16px; cursor: pointer;
}
.svc-head:hover { background: var(--el-fill-color-light); }
.svc-title { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; }
.svc-name { font-size: 14px; font-weight: 600; }
/* 服务状态徽标：告警中暖色、正常冷色 */
.status-tag {
  flex: none; font-size: 11px; padding: 1px 6px; border-radius: 4px;
  color: var(--el-color-success); background: var(--el-color-success-light-9);
}
.status-tag.firing { color: var(--el-color-danger); background: var(--el-color-danger-light-9); }
.svc-meta { flex: none; font-size: 12px; color: var(--el-text-color-secondary); font-variant-numeric: tabular-nums; }
.chev { color: var(--el-text-color-placeholder); font-size: 12px; flex: none; transition: transform 0.2s; }
.chev.open { transform: rotate(180deg); }

/* —— 机房明细表 —— */
.region-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.region-table th, .region-table td {
  text-align: left; padding: 8px 16px; border-top: 1px solid var(--el-border-color-lighter);
}
.region-table th {
  font-weight: 400; font-size: 12px; color: var(--el-text-color-secondary);
  background: var(--el-fill-color-lighter);
}
.region-table .num { font-variant-numeric: tabular-nums; }
.expand-cell { width: 32px; padding-right: 0; }
.region-row { cursor: pointer; }
.region-row:hover { background: var(--el-fill-color-light); }
.chev.sm { font-size: 11px; }
/* 集群子表：嵌在机房行下方 */
.cluster-cell { padding: 0 !important; background: var(--el-fill-color-lighter); }
.cluster-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.cluster-table th, .cluster-table td {
  text-align: left; padding: 6px 16px 6px 48px; border-bottom: 1px solid var(--el-border-color-lighter);
}
.cluster-table th {
  font-weight: 400; font-size: 11px; color: var(--el-text-color-secondary);
}
.cluster-table tr:last-child td { border-bottom: none; }
.mono { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 12px; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; }
.dot.ok { background: var(--el-color-success); }
.dot.warn { background: var(--el-color-warning); }
/* 机房上的告警摘要：单击进告警中心定位 */
.alert-link {
  display: block; width: 100%; text-align: left; border: none; background: none;
  padding: 2px 0; font-size: 13px; color: var(--el-color-warning); cursor: pointer;
}
.critical-link { color: var(--el-color-danger); }
.alert-link:hover { text-decoration: underline; }
.muted { color: var(--el-text-color-placeholder); }
.empty-hint { padding: 40px 16px; text-align: center; font-size: 13px; color: var(--el-text-color-secondary); }
.link { border: none; background: none; color: var(--el-color-primary); cursor: pointer; font-size: 13px; padding: 0; }
</style>
