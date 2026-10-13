package handler

import (
	"net/http"

	"nemo/services/authz/internal/logic"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// MenuButtonsHandler 返回按钮声明查询处理器：仅做协议转换，业务规则在 logic 层。
func MenuButtonsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.MenuButtonsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewMenuButtonsLogic(r.Context(), svcCtx)
		resp, err := l.MenuButtons(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
