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
	"nemo/services/authz/internal/types"
)

// RevokePermissionHandler 返回路由处理器：仅做协议转换，业务规则在 logic 层。
func RevokePermissionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RevokePermissionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		// go-zero 的 httpx.Parse 对 DELETE 不把 URL query 读进 r.Form(Go ParseForm
		// 行为),form 标签绑不上——契约约定 revoke 走 query,此处显式兜底读取。
		// 与 @nemo/http 的 delete(无 body)对齐。
		q := r.URL.Query()
		if req.TargetType == "" {
			req.TargetType = q.Get("targetType")
		}
		if req.TargetId == "" {
			req.TargetId = q.Get("targetId")
		}
		if req.Permission == "" {
			req.Permission = q.Get("permission")
		}

		l := logic.NewRevokePermissionLogic(r.Context(), svcCtx)
		resp, err := l.RevokePermission(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
