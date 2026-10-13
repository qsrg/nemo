// createclusterhandler.go：新建集群的 HTTP 入口（写权限已上收路由守卫
// zookeeper:manage——集群管理档维护）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/zookeeper/internal/logic"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"
)

// CreateClusterHandler 新建集群：POST /v1/zookeeper/clusters。
func CreateClusterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateClusterReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewCreateClusterLogic(r.Context(), svcCtx)
		resp, err := l.CreateCluster(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
