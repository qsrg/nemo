// listrocketmqtopicshandler.go 列表 Topic：仅做协议转换（clusterId 查询
// 参数解析），业务规则在 logic 层。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"
)

// ListRocketmqTopicsHandler 返回路由处理器。
func ListRocketmqTopicsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListRocketmqTopicsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewListRocketmqTopicsLogic(r.Context(), svcCtx)
		resp, err := l.ListRocketmqTopics(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
