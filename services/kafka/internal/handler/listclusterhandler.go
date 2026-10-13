// listclusterhandler.go 集群列表路由处理器：仅做协议转换，业务规则在 logic 层。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// ListClusterHandler 返回集群列表路由处理器。
func ListClusterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListClustersReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewListClusterLogic(r.Context(), svcCtx)
		resp, err := l.ListCluster(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
