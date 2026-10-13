// Code scaffolded by goctl. Safe to edit.
//
// 约定：handler 只做协议转换（参数绑定、调 logic、写响应），业务规则在 logic 层。

package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"nemo/services/authz/internal/logic"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"
)

// MeGrantRelationsHandler 返回路由处理器：仅做协议转换，业务规则在 logic 层。
func MeGrantRelationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GrantRelationsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewGrantRelationsLogic(r.Context(), svcCtx)
		resp, err := l.MeGrantRelations(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
