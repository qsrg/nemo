// reconcilerocketmqtopicshandler.go 对账 Topic 授权元组：仅做协议转换，
// 业务规则在 logic 层。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
)

// ReconcileRocketmqTopicsHandler 返回路由处理器。
func ReconcileRocketmqTopicsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewReconcileRocketmqTopicsLogic(r.Context(), svcCtx)
		resp, err := l.ReconcileRocketmqTopics()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
