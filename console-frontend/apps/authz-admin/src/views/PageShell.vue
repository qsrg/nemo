<template>
  <div class="page">
    <h3 class="page-title">{{ title }}</h3>
    <el-empty description="骨架页：Task 8-11 逐页填实" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<style scoped>
.page {
  padding: 16px;
}
.page-title {
  margin: 0 0 12px;
  font-size: 16px;
  font-weight: 600;
}
</style>
