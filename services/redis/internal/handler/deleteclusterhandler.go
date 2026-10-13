// deleteclusterhandler.go：删除集群的 HTTP 入口（写权限已上收路由守卫
// redis:manage；非空集群由 logic 层 409 拒绝）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"
)

// DeleteClusterHandler 删除集群：DELETE /v1/redis/clusters/:id。
func DeleteClusterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteClusterReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeleteClusterLogic(r.Context(), svcCtx)
		err := l.DeleteCluster(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"ok": "true"})
	}
}
