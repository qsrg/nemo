// createinstancehandler.go：新建实例的 HTTP 入口（参数绑定 → logic）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"
)

// CreateInstanceHandler 新建实例：POST /v1/redis/instances。
func CreateInstanceHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateInstanceReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewCreateInstanceLogic(r.Context(), svcCtx)
		resp, err := l.CreateInstance(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
