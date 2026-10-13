/**
 * useApiError：微应用视图层统一 API 错误提示（基础能力层）。
 *
 * 收编各微应用手写的 showApiError 样板（order / authz-admin 各一份）：
 * 401 静默（http 层已上报基座统一登出，页面不再重复提示）、503 服务不可用、
 * 403 按 kit/errors 语义码给精确文案、其余透出后端 message。
 * 需要定制文案的页面通过 opts 覆盖对应分支。
 */
import { ElMessage } from 'element-plus'
import { ApiError } from '@nemo/http'

export interface UseApiErrorOptions {
  /** 401 分支；默认静默（onUnauthorized 已上报基座/清会话，不重复提示） */
  on401?: (err: ApiError) => void
  /** 403 分支；缺省走 default403（按语义码精确文案） */
  on403?: (err: ApiError) => void
  /** 503 分支；缺省提示「服务暂不可用」 */
  on503?: (err: ApiError) => void
  /** 其他错误码分支；缺省透出后端 message */
  onOther?: (err: ApiError) => void
}

/** 403 语义码精确文案（kit/errors 约定，与 authz-admin 历史口径对齐）：
 *  AUTHZ_DENIED 通用无权限；AUTHZ_RELATION_FORBIDDEN 关系白名单校验失败；
 *  AUTHZ_ROLE_UNKNOWN 平台角色/规则表未定义；其余透出后端 message。 */
function default403(err: ApiError) {
  if (err.code === 'AUTHZ_DENIED') {
    ElMessage.warning('无权限')
    return
  }
  if (err.code === 'AUTHZ_RELATION_FORBIDDEN') {
    ElMessage.warning('不允许写该关系')
    return
  }
  if (err.code === 'AUTHZ_ROLE_UNKNOWN') {
    ElMessage.error('角色未定义')
    return
  }
  ElMessage.error(err.message || '请求失败')
}

export function useApiError(options: UseApiErrorOptions = {}) {
  return function handleApiError(err: unknown) {
    if (err instanceof ApiError) {
      if (err.status === 401) {
        options.on401?.(err)
        return
      }
      if (err.status === 403) {
        if (options.on403) options.on403(err)
        else default403(err)
        return
      }
      if (err.status === 503) {
        if (options.on503) options.on503(err)
        else ElMessage.error('服务暂不可用')
        return
      }
      if (options.onOther) {
        options.onOther(err)
        return
      }
      ElMessage.error(err.message || '请求失败')
      return
    }
    ElMessage.error('请求失败，请稍后重试')
  }
}
