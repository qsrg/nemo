<template>
  <router-view />
</template>

<script setup lang="ts"></script>

<style>
/* Nemo 视觉身份：深海青主色（替代 Element 默认蓝）。
   经 Element Plus 语义变量覆盖，全站组件自动继承；
   微应用侧在各自 App.vue 注入同一组令牌（shadow DOM 不继承基座变量）。 */
:root {
  --el-color-primary: #0e6e7e;
  --el-color-primary-light-3: color-mix(in srgb, #0e6e7e 70%, white);
  --el-color-primary-light-5: color-mix(in srgb, #0e6e7e 50%, white);
  --el-color-primary-light-7: color-mix(in srgb, #0e6e7e 30%, white);
  --el-color-primary-light-8: color-mix(in srgb, #0e6e7e 20%, white);
  --el-color-primary-light-9: color-mix(in srgb, #0e6e7e 10%, white);
  --el-color-primary-dark-2: color-mix(in srgb, #0e6e7e 80%, black);
}

html,
body,
#app {
  margin: 0;
  height: 100%;
  /* 显式背景：避免系统深色模式下 canvas 透出黑底 */
  background: var(--el-bg-color);
  font-family: 'Helvetica Neue', Helvetica, 'PingFang SC', 'Microsoft YaHei', Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
}

/* 数字列等宽，金额/计数不跳动 */
.el-table {
  font-variant-numeric: tabular-nums;
}
</style>
