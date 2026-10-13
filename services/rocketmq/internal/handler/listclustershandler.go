// listclustershandler.go 集群列表 HTTP 入口（列表读取无守卫：页面入口由
// 菜单码过滤，选择器数据源；可见性过滤在 logic 层）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
)

// ListClustersHandler 集群列表：GET /v1/rocketmq/clusters。
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
