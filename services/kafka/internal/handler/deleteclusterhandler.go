// deleteclusterhandler.go 集群删除路由处理器（删除权由路由守卫
// requireClusterManageCode 门控）：仅做协议转换，业务规则在 logic 层。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// DeleteClusterHandler 返回集群删除路由处理器。
func DeleteClusterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ClusterIdReq
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
