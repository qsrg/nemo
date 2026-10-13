// routes.go 注册 rocketmq 独立服务的全部路由：
//   - /v1/rocketmq/clusters* 集群 CRUD（列表无守卫——页面入口菜单码过滤、
//     选择器数据源；新建/删除挂管理码）；
//   - /v1/rocketmq/topics* Topic CRUD（列表按集群 + 集群可见性，行级操作
//     按 topic 归属集群做集群级 can_view 判定；新建用 create 码或管理码）；
//   - /healthz、/readyz 健康检查（认证之外独立注册）。
//
// 身份校验：rest.WithJwt（与 authz 共享 AccessSecret）+ authn.Identity
// 中间件注入会话（claims.sub 供 logic operatorFrom 取操作人）。
// 集群级判定（2026-10-12 定稿）：行级接口挂 requireClusterPerm
// （guard.go，编辑/删除 → 集群 can_view + 管理码回退）；新建挂
// requireCodeOrManage（create 动作码或管理码）；集群 CRUD 挂管理码。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
)

// RegisterHandlers 注册全部路由。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 受保护路由：jwt 校验 + 会话注入。
	server.AddRoutes(
		[]rest.Route{
			// ---- 集群管理（基础配置数据 + 授权锚点）----
			// 列表读取无守卫（页面入口由菜单码 rocketmq-clusters:read 过滤，
			// 选择器数据源）；集群权限入口由行内 canManageGrants 徽标控制。
			{
				Method:  http.MethodGet,
				Path:    "/clusters",
				Handler: ListClustersHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/clusters",
				Handler: requireClusterManageCode(serverCtx)(CreateClusterHandler(serverCtx)),
			},
			{
				Method:  http.MethodDelete,
				Path:    "/clusters/:id",
				Handler: requireClusterManageCode(serverCtx)(DeleteClusterHandler(serverCtx)),
			},
			// ---- Topic（集群级授权）----
			{
				Method:  http.MethodGet,
				Path:    "/topics",
				Handler: ListRocketmqTopicsHandler(serverCtx),
			},
			{
				Method: http.MethodPost,
				Path:   "/topics",
				// 新建入口判定上收路由守卫：create 动作码或管理码（管理档兜底全权）；
				// logic 再校验目标集群可见（防建到不可见集群）。
				Handler: requireCodeOrManage(serverCtx, logic.RocketMQTopicsCreateCode)(CreateRocketmqTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodPut,
				Path:   "/topics/:id",
				// 编辑权判定上收路由守卫：topic 归属集群 rmq_cluster:can_view 主判定
				//（集群可见即可操作）+ 管理码回退。
				Handler: requireClusterPerm(serverCtx, "can_view")(UpdateRocketmqTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodDelete,
				Path:   "/topics/:id",
				// 删除权判定上收路由守卫：topic 归属集群 rmq_cluster:can_view 主判定
				// + 管理码回退。
				Handler: requireClusterPerm(serverCtx, "can_view")(DeleteRocketmqTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodPost,
				Path:   "/reconcile",
				// 对账为运维操作：仅管理码持有者可触发（无实例全局动作，创建码不适用）。
				Handler: requireCodeOrManage(serverCtx, logic.RocketMQTopicsManageCode)(ReconcileRocketmqTopicsHandler(serverCtx)),
			},
		},
		rest.WithPrefix("/v1/rocketmq"),
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
	)

	// 健康检查：认证之外独立注册。
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/healthz",
				Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			},
			{
				Method:  http.MethodGet,
				Path:    "/readyz",
				Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			},
		},
	)
}
