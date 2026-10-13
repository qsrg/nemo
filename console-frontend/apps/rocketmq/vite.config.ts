import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 约定：前端请求 /api/v1/*，代理/网关剥掉 /api 前缀转发到服务的 /v1/*。
const stripApi = (p: string) => p.replace(/^\/api/, '')

// rocketmq 微应用（按钮常量试点）：只需要本服务与 authz（身份/权限）：
//   /api/v1/rocketmq → rocketmq 独立服务（本微应用所属服务，8088）
//   /api/v1/admin、/api/v1/authz、/api/v1/me → authz（:8084，授权管理/判定）
// 试点核心：不再请求 /api/v1/authz/menu-buttons——按钮定义写死在前端常量。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5279,
    strictPort: true,
    // wujie 沙箱 iframe 是 opaque origin（Origin: null），ES module 跨源加载必须放行
    cors: { origin: '*' },
    proxy: {
      // authz（:8084）：身份/权限码判定
      '/api/v1/admin': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/authz': { target: 'http://localhost:8084', rewrite: stripApi },
      // rocketmq 独立服务（:8088）：Topic CRUD（行级权限后端判定）
      '/api/v1/rocketmq': { target: 'http://localhost:8088', rewrite: stripApi },
      '/api/v1/me': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api': { target: 'http://localhost:8082', rewrite: stripApi },
    },
  },
})
