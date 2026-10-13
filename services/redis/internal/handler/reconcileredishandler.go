// reconcileredishandler.go：授权对账的 HTTP 入口（写权限已上收路由守卫
// redis:manage——对账补写 owner 是运维操作）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
)

// ReconcileRedisHandler 授权对账（补写缺失的 owner 元组）：POST /v1/redis/reconcile。
func ReconcileRedisHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewReconcileRedisLogic(r.Context(), svcCtx)
		resp, err := l.ReconcileRedis()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
