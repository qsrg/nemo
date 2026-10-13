<template>
  <div class="workbench">
    <div class="page-head">
      <h2>工作台</h2>
      <p class="sub">
        {{ today }} · {{ totalAlarms ? `${totalAlarms} 条告警待关注` : '全部机房运行正常' }}
        <span class="demo-tag" title="态势为静态演示数据，后续接入监控服务">演示数据</span>
      </p>
    </div>

    <!-- 平台态势：域健康带（静态占位，阶段二接入监控服务真实数据）。
         每域一枚卡片：聚合数字 + 机房分布块——方块数量即机房数，
         颜色编码机房健康，悬停看机房明细 -->
    <section class="sec">
      <h3 class="sec-title">平台态势</h3>
      <div class="health-strip">
        <div
          v-for="d in domainHealth"
          :key="d.domain"
          class="health-card"
          @click="router.push(`/domain/${encodeURIComponent(d.domain)}`)"
        >
          <span
            class="health-icon"
            :style="{ color: domainColor(d.domain).fg, background: domainColor(d.domain).bg }"
          >
            <el-icon><component :is="domainIcon(d.domain)" /></el-icon>
          </span>
          <div class="health-body">
            <div class="health-domain">
              {{ d.domain }}
              <span class="dot" :class="d.alarmCount ? 'warn' : 'ok'" :title="d.alarmCount ? '存在告警' : '运行正常'" />
            </div>
            <div class="health-nums">
              实例 <b class="num">{{ d.instances }}</b>
              <span class="sep">·</span>
              告警 <b class="num" :class="{ warn: d.alarmCount }">{{ d.alarmCount }}</b>
            </div>
            <!-- 机房分布：一格一个机房，数量即部署广度，颜色即机房健康 -->
            <div class="region-strip">
              <span
                v-for="r in d.regionStrip"
                :key="r.name"
                class="region-cell"
                :class="r.alarms ? 'warn' : 'ok'"
                :title="`${r.name} · 实例 ${r.instances}${r.alarms ? ` · 告警 ${r.alarms}` : ''}`"
              />
            </div>
          </div>
          <el-icon class="chev"><ArrowRight /></el-icon>
        </div>
      </div>
    </section>

    <!-- 待关注：告警本体列表（静态占位，与 ALARM_SNAPSHOT 的告警数对应；
         阶段二与态势数据同源接入）。条目单击直达对应服务页 -->
    <section class="sec">
      <h3 class="sec-title">
        待关注
        <span v-if="alerts.length" class="alert-count">{{ alerts.length }}</span>
      </h3>
      <div v-if="alerts.length" class="alert-list">
        <div
          v-for="a in alerts"
          :key="a.id"
          class="alert-item"
          :class="a.severity"
          @click="router.push({ path: '/alerts', query: { focus: a.id } })"
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
            <div class="alert-title">{{ a.title }}</div>
            <div class="alert-meta">{{ a.service }} · {{ a.region }}机房</div>
          </div>
          <span class="alert-time">{{ a.startedAt }} 开始</span>
        </div>
      </div>
      <p v-else class="empty-hint">暂无告警</p>
    </section>

    <!-- 最近使用：消费 prefs.recents（进入服务时自动记录） -->
    <section class="sec">
      <h3 class="sec-title">最近使用</h3>
      <div v-if="recents.length" class="recent-row">
        <div
          v-for="s in recents"
          :key="s.key"
          class="recent-card"
          @click="router.push(s.menus[0].path)"
        >
          <span
            class="health-icon"
            :style="{ color: domainColor(s.domain).fg, background: domainColor(s.domain).bg }"
          >
            <el-icon><component :is="serviceIcons[s.icon ?? ''] ?? serviceIcons.Menu" /></el-icon>
          </span>
          <div class="recent-body">
            <div class="recent-title">{{ s.title }}</div>
            <div class="recent-sub">{{ s.menus[0].title }}</div>
          </div>
        </div>
      </div>
      <p v-else class="empty-hint">从左侧菜单进入服务，最近使用会出现在这里</p>
    </section>
  </div>
</template>

<script setup lang="ts">
// 工作台首页 = 平台态势 + 待关注 + 最近使用。
// 全量服务目录只保留左侧菜单一处，首页不再重复；服务发现靠菜单 + AI（⌘K）。
// 态势数据来自 health.ts（域 ← 服务 ← 机房三级聚合，服务级明细在域态势页）。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRight } from '@element-plus/icons-vue'
import type { ServiceDef } from '../menus'
import { useMenusStore } from '../stores/menus'
import { serviceIcons, domainColor, domainIcon } from '../serviceIcons'
import { domainHealthList } from '../health'
import { firingAlerts } from '../alerts'
import { usePrefsStore } from '../stores/prefs'

const router = useRouter()
const prefs = usePrefsStore()
// 菜单动态注册表（Phase C 改造①）：最近使用与平台态势都只展示库内可见服务
const menusStore = useMenusStore()

// 服务可见键集（me/menus 服务端过滤口径）：态势数据按此过滤——无权限服务
// 不出现在首页态势卡（与左侧菜单同一套可见性；拉取失败回退静态时与菜单
// 同为全量，行为一致）
const visibleKeys = computed(() => new Set(menusStore.items.map((s) => s.key)))

const today = new Date().toLocaleDateString('zh-CN', {
  month: 'long',
  day: 'numeric',
  weekday: 'long',
})

// 域级态势：由服务级机房快照聚合（health.ts），仅可见服务；点击进入域
// 态势页看机房明细
const domainHealth = computed(() => domainHealthList(visibleKeys.value))

const totalAlarms = computed(() => domainHealth.value.reduce((n, d) => n + d.alarmCount, 0))

// —— 告警本体：与告警中心（alerts.ts）共用同一份静态数据，
// 仅展示未恢复（firing）的条目；单击进入告警中心并定位展开
const alerts = computed(() => firingAlerts)

// 最近访问的服务（prefs 内已按新访问置顶）；动态菜单解析服务定义，
// 可见性以 me/menus 服务端过滤为准（菜单级权限码，服务存在即有可见菜单）
const recents = computed(() =>
  prefs.recents
    .map((k) => menusStore.items.find((s) => s.key === k))
    .filter((s): s is ServiceDef => !!s)
    .slice(0, 6),
)
</script>

<style scoped>
.workbench { padding: 24px 28px; }
.page-head { margin-bottom: 20px; }
h2 { margin: 0 0 4px; font-size: 20px; }
.sub {
  margin: 0; color: var(--el-text-color-secondary); font-size: 13px;
  display: flex; align-items: center; gap: 8px;
}
/* 态势数据来源标识：静态占位阶段明示 */
.demo-tag {
  font-size: 11px; padding: 1px 6px; border-radius: 4px; flex: none;
  color: var(--el-text-color-secondary); border: 1px dashed var(--el-border-color);
}
.sec { margin-bottom: 28px; }
.sec-title { margin: 0 0 12px; font-size: 14px; font-weight: 600; }

/* —— 平台态势：域健康带 —— */
.health-strip { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 12px; }
.health-card {
  display: flex; align-items: center; gap: 10px; padding: 12px 14px; cursor: pointer;
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light); border-radius: 8px;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.health-card:hover { border-color: var(--el-color-primary-light-5); box-shadow: var(--el-box-shadow-light); }
.health-icon {
  width: 36px; height: 36px; border-radius: 8px; flex: none;
  display: inline-flex; align-items: center; justify-content: center; font-size: 17px;
}
.health-body { flex: 1; min-width: 0; }
.health-domain { display: flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 600; }
/* 状态点：全绿安静，告警点亮警示色 */
.dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
.dot.ok { background: var(--el-color-success); }
.dot.warn { background: var(--el-color-warning); }
.health-nums { margin-top: 3px; font-size: 12px; color: var(--el-text-color-secondary); }
.num { font-variant-numeric: tabular-nums; color: var(--el-text-color-primary); font-weight: 600; }
.num.warn { color: var(--el-color-warning); }
.sep { margin: 0 6px; }
/* 机房分布块：数量 = 机房数，颜色 = 机房健康 */
.region-strip { display: flex; gap: 3px; margin-top: 6px; }
.region-cell {
  width: 16px; height: 6px; border-radius: 2px;
  background: var(--el-color-success-light-7);
}
.region-cell.ok:hover { background: var(--el-color-success-light-5); }
.region-cell.warn { background: var(--el-color-warning); }
.chev { color: var(--el-text-color-placeholder); font-size: 13px; flex: none; }
.health-card:hover .chev { color: var(--el-color-primary); }

/* —— 待关注：告警本体列表 —— */
.alert-count {
  display: inline-flex; align-items: center; justify-content: center;
  min-width: 18px; height: 18px; padding: 0 5px; margin-left: 6px;
  border-radius: 9px; font-size: 12px; font-weight: 600;
  color: #fff; background: var(--el-color-warning);
  font-variant-numeric: tabular-nums; vertical-align: 2px;
}
.alert-list {
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light);
  border-radius: 8px; overflow: hidden;
}
.alert-item {
  display: flex; align-items: center; gap: 12px; padding: 12px 16px;
  cursor: pointer; border-bottom: 1px solid var(--el-border-color-lighter);
  box-shadow: inset 3px 0 0 var(--el-color-warning);
}
.alert-item:last-child { border-bottom: none; }
.alert-item:hover { background: var(--el-fill-color-light); }
/* 严重级用危险色竖条与徽标 */
.alert-item.critical { box-shadow: inset 3px 0 0 var(--el-color-danger); }
.alert-sev {
  flex: none; font-size: 11px; padding: 2px 6px; border-radius: 4px;
  color: var(--el-color-warning); background: var(--el-color-warning-light-9);
}
.critical .alert-sev { color: var(--el-color-danger); background: var(--el-color-danger-light-9); }
.health-icon.sm { width: 28px; height: 28px; border-radius: 6px; font-size: 13px; }
.alert-body { flex: 1; min-width: 0; }
.alert-title { font-size: 13px; font-weight: 500; }
.alert-meta { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px; }
.alert-time { flex: none; font-size: 12px; color: var(--el-text-color-secondary); font-variant-numeric: tabular-nums; }

/* —— 最近使用 —— */
.recent-row { display: flex; flex-wrap: wrap; gap: 12px; }
.recent-card {
  display: flex; align-items: center; gap: 10px; padding: 10px 14px; cursor: pointer;
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light); border-radius: 8px;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.recent-card:hover { border-color: var(--el-color-primary-light-5); box-shadow: var(--el-box-shadow-light); }
.recent-body { min-width: 0; }
.recent-title { font-size: 13px; font-weight: 500; }
.recent-sub { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px; }
.empty-hint {
  margin: 0; padding: 18px 16px; font-size: 13px; color: var(--el-text-color-secondary);
  background: var(--el-bg-color); border: 1px dashed var(--el-border-color-light); border-radius: 8px;
}
</style>
