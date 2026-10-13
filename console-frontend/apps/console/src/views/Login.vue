<template>
  <div class="login-page">
    <div class="glow glow-a"></div>
    <div class="glow glow-b"></div>

    <div class="card">
      <div class="brand">
        <span class="mark">N</span>
      </div>
      <h1 class="title">nemo 控制台</h1>
      <p class="sub">统一登录，进入你的服务工作台</p>

      <!-- 口令表单已下线（POST /api/v1/auth/login 端点移除）：
           登录统一跳转统一身份认证（CAS），成功后回跳 /cas-ticket 接票换 token -->
      <el-button type="primary" size="large" class="submit" @click="goCas">
        统一身份认证登录
      </el-button>
      <p class="tip">点击后将跳转至统一身份认证（CAS）页面，认证成功自动返回控制台</p>
    </div>
  </div>
</template>

<script setup lang="ts">
// CAS 登录入口：整页跳转后端 CAS 接口（302 至 CAS 登录页）。
// service 参数指向基座接票页 /cas-ticket——CAS 认证成功后 GET 回跳该页处理票据，
// 由 CasTicket.vue 调 acceptToken 换取会话（token 不经 URL 暴露，仅一次性 ticket 进 query）
function goCas() {
  const service = encodeURIComponent(window.location.origin + '/cas-ticket')
  window.location.href = `/api/v1/auth/cas/login?service=${service}`
}
</script>

<style scoped>
.login-page {
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100%;
  overflow: hidden;
  /* 深海渐变：品牌身份的第一印象 */
  background: linear-gradient(160deg, #0a4a57 0%, #0e6e7e 45%, #11808f 100%);
}
/* 两团柔和光晕制造深度，不用图片 */
.glow {
  position: absolute;
  pointer-events: none;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.5;
}
.glow-a {
  width: 480px; height: 480px;
  background: #2ea7b8;
  top: -160px; right: -120px;
}
.glow-b {
  width: 380px; height: 380px;
  background: #06333d;
  bottom: -140px; left: -100px;
}

.card {
  position: relative;
  width: 400px;
  padding: 40px 36px 32px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 18px 50px rgba(4, 42, 51, 0.35);
  text-align: center;
}
.brand { display: flex; justify-content: center; }
.mark {
  width: 52px; height: 52px; border-radius: 14px;
  background: linear-gradient(145deg, #128494, #0a4a57);
  color: #fff; font-size: 26px; font-weight: 700;
  display: inline-flex; align-items: center; justify-content: center;
  box-shadow: 0 6px 16px rgba(10, 74, 87, 0.35);
}
.title {
  margin: 16px 0 4px;
  font-size: 22px; font-weight: 600; color: var(--el-text-color-primary);
}
.sub {
  margin: 0 0 28px;
  font-size: 13px; color: var(--el-text-color-secondary);
}
.submit { width: 100%; margin-top: 4px; }
.tip {
  margin: 16px 0 0;
  font-size: 12px; color: var(--el-text-color-placeholder);
}
</style>
