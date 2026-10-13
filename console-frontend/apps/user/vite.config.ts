import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 约定：前端请求 /api/v1/*，代理/网关剥掉 /api 前缀转发到服务的 /v1/*。
const stripApi = (p: string) => p.replace(/^\/api/, '')

// 微应用 dev 配置：独立端口（供基座加载），代理指向所属后端服务。
// 微应用 JS 运行在 wujie 的 iframe 中，origin 是本端口——
// 所以 API 请求走本 dev server 的 proxy，等价于生产由 Nginx/Traefik 按 path 路由。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5274,
    strictPort: true,
    // wujie 沙箱 iframe 是 opaque origin（Origin: null），ES module 跨源加载必须放行
    cors: { origin: '*' },
    proxy: {
      '/api': { target: 'http://localhost:8081', rewrite: stripApi },
    },
  },
})
