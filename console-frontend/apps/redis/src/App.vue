<template>
  <div class="micro-app">
    <!-- 独立调试（无基座、无 service 标识）时自带简易登录与提示条；
         基座 iframe 直连模式经 URL query 携带服务标识，同样按嵌入形态渲染 -->
    <template v-if="standalone">
      <div class="standalone-bar">独立运行模式：未接入基座，登录后可单独调试</div>
      <SimpleLogin v-if="!session.token" />
    </template>
    <router-view v-if="!standalone || session.token" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { SimpleLogin, session } from '@nemo/shared'

// 嵌入形态判定：wujie 沙箱注入，或基座 iframe 直连（hash query 携带 service）
const standalone = computed(() => {
  if (window.__POWERED_BY_WUJIE__) return false
  return !new URLSearchParams(window.location.hash.split('?')[1] ?? '').has('service')
})
</script>

<style scoped>
/* 独立调试提示条：语义变量着色（接入规范 §视觉：不写裸色值） */
.standalone-bar {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning);
  padding: 8px 16px;
  font-size: 13px;
}
</style>

<style>
/* Nemo 视觉令牌（与基座一致）：shadow DOM 不继承基座 :root 变量，
   微应用必须自带同一组主色覆盖，保证跨应用视觉一致；
   根节点显式设背景色与文字色，避免系统深色模式下透黑（接入规范 §视觉） */
.micro-app {
  --el-color-primary: #0e6e7e;
  --el-color-primary-light-3: color-mix(in srgb, #0e6e7e 70%, white);
  --el-color-primary-light-5: color-mix(in srgb, #0e6e7e 50%, white);
  --el-color-primary-light-7: color-mix(in srgb, #0e6e7e 30%, white);
  --el-color-primary-light-8: color-mix(in srgb, #0e6e7e 20%, white);
  --el-color-primary-light-9: color-mix(in srgb, #0e6e7e 10%, white);
  --el-color-primary-dark-2: color-mix(in srgb, #0e6e7e 80%, black);
  background: var(--el-bg-color);
  color: var(--el-text-color-primary);
  min-height: 100%;
}
</style>
