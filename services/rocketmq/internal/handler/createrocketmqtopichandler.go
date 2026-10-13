// createrocketmqtopichandler.go 新建 Topic：仅做协议转换，业务规则在 logic 层。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"
)

// CreateRocketmqTopicHandler 返回路由处理器。
func CreateRocketmqTopicHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateRocketmqTopicReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewCreateRocketmqTopicLogic(r.Context(), svcCtx)
		resp, err := l.CreateRocketmqTopic(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
