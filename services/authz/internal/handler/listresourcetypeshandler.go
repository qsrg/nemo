// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：handler 只做协议转换（参数绑定、调 logic、写响应），
// 业务规则一律在 logic 层实现（见 backend-design §2）。

package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"nemo/services/authz/internal/logic"
	"nemo/services/authz/internal/svc"
)

// ListResourceTypesHandler 返回路由处理器：仅做协议转换，业务规则在 logic 层。
func ListResourceTypesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewListResourceTypesLogic(r.Context(), svcCtx)
		resp, err := l.ListResourceTypes()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
