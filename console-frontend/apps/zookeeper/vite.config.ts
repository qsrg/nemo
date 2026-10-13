import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 约定：前端请求 /api/v1/*，代理/网关剥掉 /api 前缀转发到服务的 /v1/*。
const stripApi = (p: string) => p.replace(/^\/api/, '')

// zookeeper 微应用后端分流（与 redis/kafka 同模式）：
//   /api/v1/zookeeper → zookeeper 独立服务（:8087，集群与节点浏览）
//   /api/v1/admin、/api/v1/authz、/api/v1/me、/api/v1/auth → authz（:8084，
//     授权管理/判定/通用 relations——ResourceGrantDialog 数据源）
//   /api/v1/users → user 服务（:8081，授权对话框的用户目录数据源）
// 按 path 精确分流——生产等价物是网关（Nginx/Traefik）的路由规则。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5278,
    strictPort: true,
    // wujie 沙箱 iframe 是 opaque origin（Origin: null），ES module 跨源加载必须放行
    cors: { origin: '*' },
    proxy: {
      // authz（:8084）：管理面 + authz 判定面——长前缀排在通配 /api 之前
      '/api/v1/admin': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/authz': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/me': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/auth': { target: 'http://localhost:8084', rewrite: stripApi },
      // zookeeper 独立服务（:8087）：集群与节点浏览
      '/api/v1/zookeeper': { target: 'http://localhost:8087', rewrite: stripApi },
      // user 服务（:8081）：UserSelect 的用户目录数据源（shared 包复用）
      '/api/v1/users': { target: 'http://localhost:8081', rewrite: stripApi },
      '/api': { target: 'http://localhost:8082', rewrite: stripApi },
    },
  },
})
