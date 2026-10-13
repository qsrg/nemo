<template>
  <div class="svc-home">
    <h3 class="page-title">权限管理</h3>
    <p class="page-desc">选择一个功能进入</p>
    <div v-if="loading" class="loading">加载中…</div>
    <el-empty v-else-if="!items.length" description="当前没有可见菜单，请联系管理员" />
    <div v-else class="cards">
      <div
        v-for="m in items"
        :key="m.id"
        class="card"
        role="button"
        tabindex="0"
        @click="go(m)"
        @keydown.enter="go(m)"
      >
        <div class="card-title">{{ m.title }}</div>
        <div class="card-sub">{{ m.path }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 服务首页（服务根行入口）：列出当前服务下可见子菜单，点击跳转对应功能。
 * 数据源 me/menus（服务端按调用者权限过滤）——authz-admin 经基座挂载，
 * me/menus 返回即 authz 服务下可见菜单；独立运行（hash 模式）同样可用。
 * 背景：服务根行此前 redirect 到 /grants 造成「权限管理/人员授权」同页双入口，
 * 现改为根行渲染本页（子菜单导航），与具体功能页区分开（见 router.ts）。
 */
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { http, showApiError } from '../api/client'
import type { MeMenuItem } from '../api/menus'

const router = useRouter()
const items = ref<MeMenuItem[]>([])
const loading = ref(true)

async function load() {
  try {
    const d = await http.get<{ items: MeMenuItem[] }>('/me/menus')
    // 服务端已按调用者权限过滤（仅 enabled 且可见的菜单行），按 (sort, id) 升序
    items.value = [...d.items].sort((a, b) => a.sort - b.sort || a.id - b.id)
  } catch (e) {
    showApiError(e)
  } finally {
    loading.value = false
  }
}

function go(m: MeMenuItem) {
  if (m.path) router.push(m.path)
}

onMounted(load)
</script>

<style scoped>
.svc-home {
  padding: 16px;
}
.page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.page-desc {
  margin: 4px 0 16px;
  font-size: 13px;
  color: #6b7280;
}
.loading {
  color: #6b7280;
  font-size: 13px;
}
.cards {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.card {
  flex: 1 1 160px;
  max-width: 240px;
  padding: 14px;
  border: 1px solid #e4e3dd;
  border-radius: 10px;
  cursor: pointer;
  background: #fff;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.card:hover,
.card:focus-visible {
  border-color: #9eacea;
  box-shadow: 0 0 0 3px rgba(158, 172, 234, 0.2);
}
.card-title {
  font-size: 14px;
  font-weight: 600;
  color: #1a1b1c;
}
.card-sub {
  margin-top: 4px;
  font-size: 12px;
  color: #9ca3af;
  word-break: break-all;
}
</style>
