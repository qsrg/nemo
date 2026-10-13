// deleteclusterhandler.go：删除集群的 HTTP 入口（写权限已上收路由守卫
// zookeeper:manage；删除集群由 logic 层守卫）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/zookeeper/internal/logic"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"
)

// DeleteClusterHandler 删除集群：DELETE /v1/zookeeper/clusters/:id。
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
