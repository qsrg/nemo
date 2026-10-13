// deleterocketmqtopichandler.go 删除 Topic：仅做协议转换，业务规则在 logic 层。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"
)

// DeleteRocketmqTopicHandler 返回路由处理器。
func DeleteRocketmqTopicHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteRocketmqTopicReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeleteRocketmqTopicLogic(r.Context(), svcCtx)
		if err := l.DeleteRocketmqTopic(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, nil)
		}
	}
}
