// listinstanceshandler.go：实例列表的 HTTP 入口（参数绑定 → logic）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"
)

// ListInstancesHandler 实例列表：GET /v1/redis/instances。
func ListInstancesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListInstancesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewListInstancesLogic(r.Context(), svcCtx)
		resp, err := l.ListInstances(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
