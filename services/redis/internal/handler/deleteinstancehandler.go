// deleteinstancehandler.go：删除实例的 HTTP 入口（参数绑定 → logic）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"
)

// DeleteInstanceHandler 删除实例：DELETE /v1/redis/instances/:id。
func DeleteInstanceHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteInstanceReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeleteInstanceLogic(r.Context(), svcCtx)
		err := l.DeleteInstance(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, nil)
	}
}
