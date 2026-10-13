<template>
  <div class="settings">
    <div class="page-head">
      <h2>系统设置</h2>
      <p class="sub">维护你的使用偏好，保存在本机浏览器，下次登录自动生效</p>
    </div>

    <section class="sec">
      <h3 class="sec-title">菜单偏好</h3>
      <p class="sec-desc">控制左侧「菜单」面板中各服务的菜单默认展开还是收起</p>
      <div class="rows">
        <div v-for="s in SERVICES" :key="s.key" class="row">
          <span
            class="row-icon"
            :style="{ color: domainColor(s.domain).fg, background: domainColor(s.domain).bg }"
          >
            <el-icon><component :is="serviceIcons[s.icon ?? ''] ?? serviceIcons.Menu" /></el-icon>
          </span>
          <div class="row-body">
            <div class="row-title">{{ s.title }}</div>
            <div class="row-desc">{{ s.menus.map((m) => m.title).join('、') }}</div>
          </div>
          <el-switch
            :model-value="!prefs.menuCollapsed.includes(s.key)"
            active-text="展开"
            inactive-text="收起"
            @change="(v: string | number | boolean) => prefs.setMenuCollapsed(s.key, !v)"
          />
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
// 系统设置：基座本地页面（不走微应用），维护用户偏好。
// 当前提供「菜单偏好」：各服务在左侧菜单面板中的默认展开/收起状态，
// 经 prefs store 持久化到 localStorage，基座启动时生效。
import { SERVICES } from '../menus'
import { serviceIcons, domainColor } from '../serviceIcons'
import { usePrefsStore } from '../stores/prefs'

const prefs = usePrefsStore()
</script>

<style scoped>
.settings { padding: 24px 28px; max-width: 720px; }
.page-head { margin-bottom: 20px; }
h2 { margin: 0 0 4px; font-size: 20px; }
.sub { margin: 0; color: var(--el-text-color-secondary); font-size: 13px; }
.sec { margin-bottom: 28px; }
.sec-title { margin: 0 0 4px; font-size: 14px; font-weight: 600; }
.sec-desc { margin: 0 0 12px; font-size: 12px; color: var(--el-text-color-secondary); }
.rows {
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-light);
  border-radius: 8px; overflow: hidden;
}
.row {
  display: flex; align-items: center; gap: 12px; padding: 12px 16px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.row:last-child { border-bottom: none; }
.row-icon {
  width: 32px; height: 32px; border-radius: 7px; flex: none;
  display: inline-flex; align-items: center; justify-content: center; font-size: 15px;
}
.row-body { flex: 1; min-width: 0; }
.row-title { font-size: 14px; font-weight: 500; }
.row-desc {
  font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
</style>
