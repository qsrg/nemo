import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 约定：前端请求 /api/v1/*，代理/网关剥掉 /api 前缀转发到服务的 /v1/*。
const stripApi = (p: string) => p.replace(/^\/api/, '')

// kafka 微应用同时需要两个后端服务：
//   /api/v1/users → user 服务（UserSelect 业务组件的数据源，shared 包复用）
//   /api/v1/kafka → kafka 独立服务（本微应用所属服务，拆服务后 8085）
//   /api/v1/admin、/api/v1/authz、/api/v1/me → authz（:8084，授权管理/判定）
// 按 path 精确分流——生产等价物是网关（Nginx/Traefik）的路由规则。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5275,
    strictPort: true,
    // wujie 沙箱 iframe 是 opaque origin（Origin: null），ES module 跨源加载必须放行
    cors: { origin: '*' },
    proxy: {
      // authz（:8084）：管理面（菜单 seed/CRUD 等）、authz 判定面——
      // 长前缀必须排在通配 /api 之前（vite 按声明序取首个命中前缀）
      '/api/v1/admin': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/authz': { target: 'http://localhost:8084', rewrite: stripApi },
      // kafka 独立服务（:8085）：Topic CRUD + 名单管理（原内嵌 authz 路由迁移）
      '/api/v1/kafka': { target: 'http://localhost:8085', rewrite: stripApi },
      '/api/v1/me': { target: 'http://localhost:8084', rewrite: stripApi },
      // auth 已并入 authz（服务合并）：UserSelect 的用户目录数据源收敛 :8084
      '/api/v1/auth': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/users': { target: 'http://localhost:8081', rewrite: stripApi },
      '/api': { target: 'http://localhost:8082', rewrite: stripApi },
    },
  },
})
