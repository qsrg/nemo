<template>
  <!-- CAS 接票页：CAS 认证成功后 GET 回跳 /cas-ticket?ticket=ST-xxx。
       只做一件事——拿一次性 ticket 换 token 建会话，随即进基座（成功）或回登录页（失败）。
       页面本身不落任何 UI 状态，处理中仅展示 loading。 -->
  <div class="cas-ticket">
    <div class="card">
      <el-icon class="is-loading spin" :size="28"><Loading /></el-icon>
      <p class="text">正在验证登录凭证…</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)

onMounted(async () => {
  // ticket 只进一次 query（CAS 一次性票据，消费后即作废），token 不经 URL 暴露
  const ticket = typeof route.query.ticket === 'string' ? route.query.ticket : ''
  if (!ticket) {
    ElMessage.warning('缺少登录票据，请重新登录')
    router.replace('/login')
    return
  }
  loading.value = true
  try {
    // POST /api/v1/auth/cas/callback 换 TokenResp（acceptToken 内部完成）：
    // 设 token → /me 恢复用户 → 拉权限码；失败即 logout 并抛错
    await auth.acceptToken(ticket)
    // 支持 ?redirect= 回原页（登录前被守卫拦下的目标）；只接受站内路径防开放重定向
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
    const safe = redirect.startsWith('/') && !redirect.startsWith('//') ? redirect : '/'
    router.replace(safe)
  } catch {
    ElMessage.error('登录失败，请重新登录')
    router.replace('/login')
  }
})
</script>

<style scoped>
.cas-ticket {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100%;
}
.card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: var(--el-text-color-secondary);
}
.spin {
  color: var(--el-color-primary);
}
.text {
  margin: 0;
  font-size: 14px;
}
</style>
