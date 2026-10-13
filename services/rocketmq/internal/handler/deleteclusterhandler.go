// deleteclusterhandler.go：删除集群的 HTTP 入口（写权限已上收路由守卫
// rocketmq-topics:manage）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"
)

// DeleteClusterHandler 删除集群：DELETE /v1/rocketmq/clusters/:id。
func DeleteClusterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteRocketmqClusterReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeleteClusterLogic(r.Context(), svcCtx)
		if err := l.DeleteCluster(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, nil)
	}
}
