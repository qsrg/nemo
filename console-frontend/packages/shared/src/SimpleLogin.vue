<template>
  <!-- 独立运行模式的调试入口：微应用脱离基座单独调试时手动注入会话 token。
       CAS 登录流程走基座（Login.vue → /cas-ticket → acceptToken），本组件不参与；
       集成在基座（wujie/iframe）时由基座下发会话，本组件不会渲染（v-if="!session.token"）。 -->
  <div class="simple-login">
    <el-card class="card">
      <template #header>独立运行模式 · 手动注入会话</template>
      <el-form @submit.prevent="submit">
        <el-form-item label="Token">
          <el-input
            v-model="token"
            type="textarea"
            :rows="3"
            placeholder="粘贴会话 token（JWT）进行调试"
          />
        </el-form-item>
        <el-button type="primary" style="width: 100%" native-type="submit">
          注入会话
        </el-button>
      </el-form>
      <p class="tip">完整会话（用户/权限码）请从基座进入——基座将经 postMessage 自动下发</p>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { session, setSessionToken } from './micro'

const token = ref('')

// 手动注入 token：仅写 session.token（用户/权限码为空，相关视图降级），
// 便于本地直调子应用页面与 API；随后异步补拉当前用户权限码写入
// session.permissions（基座模式由基座下发，此处为独立调试增强，失败不阻断）。
// 口令登录端点已下线，不再走表单登录
function submit() {
  const value = token.value.trim()
  if (!value) {
    ElMessage.warning('请先粘贴 token')
    return
  }
  setSessionToken(value) // 唤醒 sessionReady 等待方(UserSelect 等)
  void fetchPermissions(value)
  ElMessage.success('已写入会话 token；从基座进入可获完整会话（用户/权限码）')
}

/** 独立调试补全权限码：拉 /me/permissions（跨服务判定面）写入 session。 */
async function fetchPermissions(value: string) {
  try {
    const resp = await fetch('/api/v1/me/permissions', {
      headers: { Authorization: `Bearer ${value}` },
    })
    if (!resp.ok) return
    const data = (await resp.json()) as { perms?: unknown }
    if (Array.isArray(data.perms)) {
      session.permissions = data.perms.filter((p): p is string => typeof p === 'string')
    }
  } catch {
    // 调试增强失败静默降级，不阻断页面
  }
}
</script>

<style scoped>
.simple-login {
  display: flex;
  justify-content: center;
  padding-top: 10vh;
}
.card {
  width: 360px;
}
.tip {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
