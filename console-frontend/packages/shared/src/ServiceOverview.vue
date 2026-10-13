<template>
  <div class="service-overview">
    <template v-if="serviceKey">
      <div class="head">
        <h2>{{ title }}</h2>
        <span class="demo-tag" title="集群列表为静态演示数据，阶段二由服务自己的接口提供">演示数据</span>
      </div>
      <p class="sub">集群 {{ totalClusters }} · 实例 {{ totalInstances }} · 部署 {{ regionCount }} 个机房</p>

      <!-- 集群列表：与基座域态势页同源的「服务 → 机房 → 集群」视图 -->
      <el-table :data="rows" v-loading="loading" class="cluster-table">
        <el-table-column prop="name" label="集群" min-width="160">
          <template #default="{ row }">
            <span class="mono">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="region" label="机房" width="100" />
        <el-table-column prop="instances" label="实例" width="80" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <span class="status">{{ row.status }}</span>
          </template>
        </el-table-column>
      </el-table>
    </template>
    <div v-else class="empty">
      独立运行模式：未接入基座，无法识别服务。请从基座菜单进入本页，或单独调试时在地址栏使用
      <code>#/?service=redis</code> 形式。
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 服务概览页（微应用通用模板）：所有业务微应用的默认页。
 * 服务标识来自基座下发的 props.base（如 /redis → redis），
 * 集群列表为静态演示数据；阶段二各服务替换为自己的接口实现
 * （经 @nemo/http 调用，失败可回退 clusterRowsOf 演示数据）。
 */
import { computed, ref } from 'vue'
import { clusterRowsOf, serviceTitle, type ClusterRow } from './serviceOverview'

const serviceKey = computed(() => {
  if (window.__POWERED_BY_WUJIE__) {
    const base = window.$wujie?.props?.base ?? ''
    return base.replace(/^\//, '')
  }
  // 独立运行：hash 查询参数指定服务，便于单独调试
  const q = new URLSearchParams(window.location.hash.split('?')[1] ?? '')
  return q.get('service') ?? ''
})

const title = computed(() => serviceTitle(serviceKey.value))
const rows = computed<ClusterRow[]>(() => clusterRowsOf(serviceKey.value))
const totalClusters = computed(() => rows.value.length)
const totalInstances = computed(() => rows.value.reduce((n, r) => n + r.instances, 0))
const regionCount = computed(() => new Set(rows.value.map((r) => r.region)).size)

// 阶段二接入真实接口的占位：当前静态数据无需加载，保留 loading 语义
const loading = ref(false)
</script>

<style scoped>
.service-overview { padding: 20px 24px; }
.head { display: flex; align-items: center; gap: 10px; }
h2 { margin: 0; font-size: 20px; }
.demo-tag {
  font-size: 11px; padding: 1px 6px; border-radius: 4px;
  color: var(--el-text-color-secondary); border: 1px dashed var(--el-border-color);
}
.sub { margin: 8px 0 16px; font-size: 13px; color: var(--el-text-color-secondary); }
.cluster-table { font-variant-numeric: tabular-nums; }
.mono { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 12px; }
.status { color: var(--el-color-success); font-size: 12px; }
.empty { padding: 40px 16px; text-align: center; font-size: 13px; color: var(--el-text-color-secondary); line-height: 2; }
.empty code { padding: 2px 6px; border-radius: 4px; background: var(--el-fill-color); }
</style>
