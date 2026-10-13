// reconcilezookeeperhandler.go：授权对账的 HTTP 入口（写权限已上收路由守卫
// zookeeper:manage——对账补写 owner 是运维操作）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/zookeeper/internal/logic"
	"nemo/services/zookeeper/internal/svc"
)

// ReconcileHandler 授权对账（补写缺失的 owner 元组）：POST /v1/zookeeper/reconcile。
func ReconcileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewReconcileLogic(r.Context(), svcCtx)
		resp, err := l.Reconcile()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
