// Code scaffolded by goctl. Safe to edit.
//
// 约定：handler 只做协议转换（参数绑定、调 logic、写响应），
// 业务规则一律在 logic 层实现。

package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
)

// ReconcileKafkaTopicsHandler 返回路由处理器：触发 Topic 授权对账（补缺失
// owner），业务规则在 logic 层（管理码鉴权）。
func ReconcileKafkaTopicsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewReconcileKafkaTopicsLogic(r.Context(), svcCtx)
		resp, err := l.ReconcileKafkaTopics()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
