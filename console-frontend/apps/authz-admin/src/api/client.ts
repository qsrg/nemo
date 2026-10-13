/**
 * 权限管理微应用 http 兼容层（基础能力层收编后）：
 * 原 createHttp 实例 / showApiError / baseProps 逻辑已统一收敛到 @nemo/shared
 * （createMicroHttp / useApiError / sessionProps），本文件保留原导出名与
 * 语义（7 个 api 模块与 3 个视图的 import 路径不变，零改动接入）：
 * - http：微应用统一 http 工厂（token 实时读 session、401 竞态区分上报基座登出）
 * - showApiError：视图层统一错误提示（401 静默 / 503 / 403 按语义码 / 透出 message）
 * - baseProps：基座会话只读视图（wujie props 优先，独立运行组装）
 */
import { createMicroHttp, sessionProps, useApiError } from '@nemo/shared'

export const http = createMicroHttp()

/** 视图层统一错误提示：401 已由 http 层上报基座/静默，页面不重复提示（原语义不变） */
export const showApiError = useApiError()

/** 基座下发的会话与权限信息：wujie props 优先；iframe 直连形态回退
 *  shared sessionProps（postMessage nemo:session 写入的 reactive session）。 */
export const baseProps = sessionProps
