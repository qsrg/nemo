// listclustershandler.go：集群列表的 HTTP 入口（读取无守卫，页面可见即可读）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/zookeeper/internal/logic"
	"nemo/services/zookeeper/internal/svc"
)

// ListClustersHandler 全量集群列表：GET /v1/zookeeper/clusters。
func ListClustersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewListClustersLogic(r.Context(), svcCtx)
		resp, err := l.ListClusters()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
