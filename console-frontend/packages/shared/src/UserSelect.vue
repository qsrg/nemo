<template>
  <!-- 用户选择器：跨微应用复用的业务组件（架构 §8.5 业务组件库职责）。
       数据来自 auth 服务的用户目录 GET /v1/auth/users（B 阶段改造 2.4：
       用户档案唯一事实源 = auth，选人不再依赖 user 服务 demo 用户），
       token 取 session——同一份组件在基座与任意微应用中行为一致。
       option value = 裸用户名（JWT sub 口径，即后端 userId），label = name（userId）。 -->
  <el-select
    :model-value="modelValue"
    filterable
    remote
    :remote-method="search"
    :loading="loading"
    :placeholder="placeholder"
    style="width: 100%"
    clearable
    @update:model-value="(v: string) => $emit('update:modelValue', v ?? '')"
    @change="search()"
  >
    <el-option v-for="u in users" :key="u.userId" :value="u.userId" :label="`${u.name}（${u.userId}）`" />
  </el-select>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createHttp, ApiError } from '@nemo/http'
import { session, sessionReady } from './micro'
import type { PagedResult } from './types'

/** 目录用户条目（GET /v1/auth/users 响应项；userId = 裸用户名 / JWT sub）。 */
interface DirectoryUser {
  userId: string
  name: string
  email: string
}

defineProps<{ modelValue?: string; placeholder?: string }>()
defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const users = ref<DirectoryUser[]>([])
const loading = ref(false)
const http = createHttp({
  baseURL: '/api/v1',
  getToken: () => session.token,
})

async function search(keyword = '') {
  loading.value = true
  try {
    const q = keyword ? `?keyword=${encodeURIComponent(keyword)}&pageSize=50` : '?pageSize=50'
    const result = await http.get<PagedResult<DirectoryUser>>(`/auth/users${q}`)
    users.value = result.items
  } catch (e) {
    ElMessage.error(e instanceof ApiError ? `加载用户失败：${e.message}` : '加载用户失败')
  } finally {
    loading.value = false
  }
}

// 初始加载挂 sessionReady 门控:iframe 形态下 postMessage 会话未到前
// session.token 为空,立刻请求会 401(与各页面同款握手竞态修复)
onMounted(async () => {
  await sessionReady()
  void search()
})
</script>
