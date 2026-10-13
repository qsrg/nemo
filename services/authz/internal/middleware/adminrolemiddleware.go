// adminrolemiddleware.go 管理面鉴权中间件（去角色化后，权限码是唯一真源）：
//
//	JWT 已由 go-zero jwt 中间件校验，这里改为对授权引擎实时判定——
//	Check(user:{sub}, can, permission:*.*)：admin 引导时已写通配权限元组，
//	持有该权限点即天然全权（PermGranted/hasPermission 的 *:* 分支匹配任意
//	<服务>:<动作>）。不再读 claims.roles，也不再查 platform_role 类型：
//	  - 引擎判定 true → 放行；
//	  - 明确 false（含无身份的异常路径）→ 403 AUTHZ_FORBIDDEN（fail-closed）；
//	  - 引擎不可用 → 503 AUTHZ_SERVICE_UNAVAILABLE（fail-closed：管理面操作
//	    宁可拒绝也不在引擎失联时放行——撤权即时生效，不再缓存角色）。
package middleware

import (
	"net/http"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
)

// AdminRoleMiddleware 是管理面鉴权中间件（去角色化改造后保留原名，语义已
//
//	从"查超管角色"变为"查全通配权限"——admin 引导时写 permission:*.*#can）。
type AdminRoleMiddleware struct {
	// engine 是内嵌授权引擎句柄（EnsureModel 绑定后的 Check 判定）。
	engine *engine.Handle
}

// adminAllPerm 是超管全通配权限点对象（与 bootstrap AdminAllPerm 常量
// 同源）：Check(user:{sub}, can, permission:*.*) 判定超管全权。
const adminAllPerm = "permission:*.*"

// NewAdminRoleMiddleware 构造管理面鉴权中间件，engine 为授权引擎句柄。
func NewAdminRoleMiddleware(engine *engine.Handle) *AdminRoleMiddleware {
	return &AdminRoleMiddleware{engine: engine}
}

// Handle 实现中间件：引擎实时判定 user:{sub} 是否持有 permission:*.* 的
// can——false/无身份 403，引擎错误 503（均 fail-closed）。
func (m *AdminRoleMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := authzkit.IdentityFrom(r.Context())
		sub := s.Sub
		if err != nil || strings.TrimSpace(sub) == "" {
			// 能进到这里说明 jwt 已通过但 ctx 无 sub——异常路径，按无权限拒绝（fail-closed）。
			httpx.ErrorCtx(r.Context(), w, errors.Forbidden("AUTHZ_FORBIDDEN", "需要平台管理员权限"))
			return
		}
		allowed, err := m.engine.Check(r.Context(), "user:"+sub, "can", adminAllPerm)
		if err != nil {
			// 引擎不可用：管理面操作 fail-closed 拒绝（503），不做本地缓存兜底。
			logx.WithContext(r.Context()).Errorf("管理面鉴权引擎判定失败 sub=%s: %v", sub, err)
			httpx.ErrorCtx(r.Context(), w, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "授权引擎不可用，操作被拒绝（fail-closed）"))
			return
		}
		if !allowed {
			httpx.ErrorCtx(r.Context(), w, errors.Forbidden("AUTHZ_FORBIDDEN", "需要平台管理员权限"))
			return
		}
		next(w, r)
	}
}
