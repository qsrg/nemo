// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：handler 只做协议转换（参数绑定、调 logic、写响应），
// 业务规则一律在 logic 层实现（见 backend-design §2）。
//
// 手写重写说明（契约上 CasLogin 无返回体）：goctl 生成的统一响应出口不适用，
// 本 handler 以 302 重写把 logic 拼装的 CAS 登录页 URL 下发（Phase B Task 2
// 计划允许重写该一个 handler，其余生成 handler 不动）。

package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"nemo/services/authz/internal/logic"
	"nemo/services/authz/internal/svc"
)

// CasLoginHandler 返回路由处理器：调 logic 拼 CAS 登录页 URL 并 302 重定向。
func CasLoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewCasLoginLogic(r.Context(), svcCtx)
		// service 由前端指定（接票页地址，多前端入口/dev 端口场景各异），
		// 空/非法时 logic 回退配置默认值
		loginURL, err := l.CasLogin(r.URL.Query().Get("service"))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
	}
}
