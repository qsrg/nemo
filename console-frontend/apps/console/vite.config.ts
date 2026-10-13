import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const stripApi = (p: string) => p.replace(/^\/api/, '')

// 基座 dev 代理：生产由 Nginx/Traefik 按 path 路由（架构 §4.2），
// dev 用 vite proxy 等价模拟。约定：前端统一请求 /api/v1/*，
// 网关/代理剥掉 /api 前缀转发到各服务的 /v1/*。
// 认证/会话走 auth（:8083），权限码与管理面走 authz（:8084），用户资料走 user（:8081）。
export default defineConfig({
  plugins: [vue()],
  server: {
    // 端口可被 PORT 环境变量覆盖（多实例/自动化测试场景），默认 5273
    port: Number(process.env.PORT) || 5273,
    strictPort: true,
    proxy: {
      // —— authz（:8084）：授权管理面 API 与当前用户权限码（permission-design §5.2）
      '/api/v1/authz': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/admin': { target: 'http://localhost:8084', rewrite: stripApi },
      // me/permissions 由 authz 服务提供（permission-design §5.2）；
      // 长前缀须排在 /api/v1/me 之前（代理按声明序取首个命中前缀，避免被 /me 截走）
      // me/menus 与 me/permissions 同为 authz 服务提供(动态菜单,Phase C);
      // 两条长前缀都必须排在 /api/v1/me 之前,否则被 auth 的会话规则截走
      '/api/v1/me/permissions': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/me/menus': { target: 'http://localhost:8084', rewrite: stripApi },
      // —— auth 已并入 authz（服务合并）：登录/会话/api-key/me 全部收敛 :8084。/api/v1/me 从 user 服务（:8081）切到 auth
      // —— auth 接管会话恢复（me 返回 claims，roles 续签实时重取，Phase B Task 3 起）；
      // 联调回退：临时改回 http://localhost:8081 即可（user 服务旧实现仍在）
      '/api/v1/auth': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/api-keys': { target: 'http://localhost:8084', rewrite: stripApi },
      '/api/v1/me': { target: 'http://localhost:8084', rewrite: stripApi },
      // —— user（:8081）：用户资料等存量接口，保持不变
      '/api/v1/users': { target: 'http://localhost:8081', rewrite: stripApi },
    },
  },
})
