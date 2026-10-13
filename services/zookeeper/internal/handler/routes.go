// routes.go 注册 zookeeper 独立服务的全部路由：
//   - /v1/zookeeper/* 集群与节点浏览（前端 apps/zookeeper 经 vite proxy 接入）；
//   - /healthz、/readyz 健康检查（可运维性规范，认证之外独立注册）。
//
// 身份校验：rest.WithJwt（与 authz 共享 AccessSecret）+ authzkit.Identity
// 中间件注入会话（claims.sub 供 logic operatorFrom 取操作人）。
// 集群可见性：GET /clusters 无守卫（logic 内按引擎 can_view 过滤——manage
// 码全量，普通用户只见被授权集群）；**节点浏览挂集群 can_view 守卫**
// （"看到集群才能浏览节点"，不可见集群 403）；写操作挂管理码。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"nemo/services/zookeeper/internal/svc"
)

// RegisterHandlers 注册全部路由。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 受保护路由：jwt 校验 + 会话注入。
	server.AddRoutes(
		[]rest.Route{
			{
				// 集群列表：读取无守卫（页面入口由菜单码 zookeeper:read 过滤，
				// me/menus 逐行过滤），可见性在 logic 内按引擎 can_view 过滤
				//（manage 码持有者全量）——选择器数据源。
				Method:  http.MethodGet,
				Path:    "/clusters",
				Handler: ListClustersHandler(serverCtx),
			},
			{
				// 节点浏览：**集群 can_view 主判定 + 管理码回退**——看到集群
				// 才能浏览节点（与列表同口径，不可见集群 403 fail-closed）。
				Method:  http.MethodGet,
				Path:    "/clusters/:id/nodes",
				Handler: requireClusterView(serverCtx)(ListNodesHandler(serverCtx)),
			},
			{
				// 新建集群：管理码守卫（管理档维护，无按钮动作码）。
				Method:  http.MethodPost,
				Path:    "/clusters",
				Handler: requireClusterManageCode(serverCtx)(CreateClusterHandler(serverCtx)),
			},
			{
				// 删除集群：管理码守卫；zk 集群无业务子实体，无非空校验。
				Method:  http.MethodDelete,
				Path:    "/clusters/:id",
				Handler: requireClusterManageCode(serverCtx)(DeleteClusterHandler(serverCtx)),
			},
			{
				// 授权对账（补偿）：扫描业务表补写缺失的 owner 元组。
				Method:  http.MethodPost,
				Path:    "/reconcile",
				Handler: requireClusterManageCode(serverCtx)(ReconcileHandler(serverCtx)),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/v1/zookeeper"),
	)

	// 健康检查：认证之外独立注册。
	server.AddRoute(rest.Route{
		Method: http.MethodGet,
		Path:   "/healthz",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		},
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/readyz",
		Handler: serverCtx.ReadyHandler,
	})
}
