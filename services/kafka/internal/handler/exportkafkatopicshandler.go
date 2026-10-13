// Code scaffolded by goctl. Safe to edit.
//
// 约定：handler 只做协议转换（参数绑定、调 logic、写响应），
// 业务规则一律在 logic 层实现（见 backend-design §2）。
// 导出是 CSV 文本响应（非 JSON），Content-Disposition 触发浏览器下载。

package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// ExportKafkaTopicsHandler 返回路由处理器：仅做协议转换，业务规则在 logic 层。
func ExportKafkaTopicsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ExportTopicsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewExportKafkaTopicsLogic(r.Context(), svcCtx)
		csv, err := l.ExportKafkaTopics(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=topics.csv")
		_, _ = w.Write([]byte(csv))
	}
}
