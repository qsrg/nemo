// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：handler 只做协议转换（参数绑定、调 logic、写响应），
// 业务规则一律在 logic 层实现（见 backend-design §2）。

package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// DeleteKafkaTopicHandler 返回路由处理器：仅做协议转换，业务规则在 logic 层。
func DeleteKafkaTopicHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteTopicReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewDeleteKafkaTopicLogic(r.Context(), svcCtx)
		err := l.DeleteKafkaTopic(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
