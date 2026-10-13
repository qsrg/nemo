import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 约定：前端请求 /api/v1/*，代理/网关剥掉 /api 前缀转发到服务的 /v1/*。
const stripApi = (p: string) => p.replace(/^\/api/, '')

// 微应用 dev 配置：独立端口（供基座加载），代理指向所属后端服务。
// 微应用 JS 运行在 wujie 的 iframe 中，origin 是本端口——
// 所以 API 请求走本 dev server 的 proxy，等价于生产由 Nginx/Traefik 按 path 路由。
// 代理分段照搬基座 Task 6 的口径（apps/console/vite.config.ts）：
// 权限管理面与登录/会话均走 authz（:8084）——auth 已于 2026-09-21 并入 authz。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5276,
    strictPort: true,
    // wujie 沙箱 iframe 是 opaque origin（Origin: null），ES module 跨源加载必须放行
    cors: { origin: '*' },
    proxy: {
      // —— authz（:8084）：授权管理面 API 与当前用户权限码（permission-design §5.2）
      '/api/v1/authz': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/admin': { target: 'http://localhost:8084', rewrite: stripApi },
      // me/permissions 由 authz 服务提供；长前缀须排在 /api/v1/me 之前
      // （代理按声明序取首个命中前缀，避免被 /me 截走）
      '/api/v1/me/permissions': { target: 'http://localhost:8084', rewrite: stripApi },
      // —— auth 已并入 authz（服务合并）：登录/会话/api-key/me 全部收敛 :8084
      '/api/v1/auth': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/api-keys': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/me': { target: 'http://localhost:8084', rewrite: stripApi },
      // —— user（:8081）：成员管理抽屉的 UserSelect 拉用户列表（口径同基座 Task 6）
      '/api/v1/users': { target: 'http://localhost:8081', rewrite: stripApi },
    },
  },
})
