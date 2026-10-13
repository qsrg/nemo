// listnodeshandler.go：节点浏览的 HTTP 入口（集群可见性守卫在路由层）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/zookeeper/internal/logic"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"
)

// ListNodesHandler 节点树浏览：GET /v1/zookeeper/clusters/:id/nodes。
func ListNodesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListNodesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewListNodesLogic(r.Context(), svcCtx)
		resp, err := l.ListNodes(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
