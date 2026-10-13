/**
 * 权限变更联动通知（Task18/B10：自 Task17/B9 的 Roles.vue/Rules.vue 各自实现
 * 收敛为共享 util，两页重复的同段逻辑不再各自维护）。
 *
 * 两路约定（Task 9 落定，注释随代码迁移）：
 * 1) wujie 集成且 bus 可达（__POWERED_BY_WUJIE__ && $wujie.bus）→
 *    emitToBase('refresh-permission')，基座 main.ts 监听该事件重拉
 *    /me/permissions，侧边栏菜单实时反映；
 * 2) bus 不可达（独立运行 / wujie bus 未注入）→ 降级提示用户手动刷新生效。
 */
import { ElMessage } from 'element-plus'
import { emitToBase } from '@nemo/shared'

export function notifyPermissionChanged() {
  if (window.__POWERED_BY_WUJIE__ && window.$wujie?.bus) {
    emitToBase('refresh-permission')
  } else {
    ElMessage.info('权限已更新，刷新页面生效')
  }
}
