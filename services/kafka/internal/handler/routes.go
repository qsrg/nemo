// routes.go 注册 kafka 独立服务的全部路由：
//   - /v1/kafka/* Topic CRUD + 名单三件套（与原内嵌于 authz 的 kafka 路由
//     契约逐字一致，前端 apps/kafka 无需改动）；
//   - /healthz、/readyz 健康检查（可运维性规范，认证之外独立注册）。
//
// 身份校验：rest.WithJwt（与 authz 共享 AccessSecret）+ authn.Identity
// 中间件注入会话（claims.sub 供 logic operatorFrom 取操作人）。
// 资源级判定：带实例 ID 的行级接口挂声明式守卫 requireTopicPerm
// （guard.go，编辑 can_edit / 删除 can_delete / 查看消费者 can_view +
// 管理码回退）；列表过滤、创建管理码判定等无法中间件化的判定仍在
// logic 内（见各 logic 注释）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
)

// RegisterHandlers 注册全部路由。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 受保护路由：jwt 校验 + 会话注入。
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/topics",
				Handler: ListKafkaTopicsHandler(serverCtx),
			},
			{
				Method: http.MethodPost,
				Path:   "/topics",
				// 新建入口判定上收路由守卫（guard.go）：create 动作码
				// kafka-topics:create 或管理码 kafka-topics:manage（管理档
				// 兜底全权）；logic 不再内联 menuManageGranted。
				Handler: requireCodeOrManage(serverCtx, logic.KafkaTopicsCreateCode)(CreateKafkaTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodPut,
				Path:   "/topics/:id",
				// 编辑权判定上收路由守卫（guard.go）：kafka_topic:can_edit 主判定
				// + 管理码回退；logic 不再内联 topicCanEdit。
				Handler: requireTopicPerm(serverCtx, "can_edit")(UpdateKafkaTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodDelete,
				Path:   "/topics/:id",
				// 删除判定上收路由守卫（can_delete + 管理码回退）。can_delete
				// 2026-09-30 起为独立授权边界（owner），不再复用 can_edit——
				// 可编辑者（owner or editor）不再自动可删。幂等分支保留在
				// logic（资源不存在返成功——守卫前置后仅管理码持有者可达该分支；
				// 普通 owner 重试已删资源将 403，与 PUT 的判定先于存在性同口径，
				// 同时关闭原实现"403=存在 / 200=不存在"的存在性枚举通道）。
				Handler: requireTopicPerm(serverCtx, "can_delete")(DeleteKafkaTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodPost,
				Path:   "/reconcile",
				// 对账为运维操作：管理码守卫（无实例 ID）。
				Handler: requireTopicManageCode(serverCtx)(ReconcileKafkaTopicsHandler(serverCtx)),
			},
			{
				// 导出清单（工具栏按钮，无实例全局操作）：export 动作码或管理码守卫
				// ——与新建/编辑/删除等操作对齐，管理档天然包含导出权（导出范围
				// = 调用者可见行，logic 复用列表可见性口径）。
				Method:  http.MethodPost,
				Path:    "/topics/export",
				Handler: requireCodeOrManage(serverCtx, logic.KafkaTopicsExportCode)(ExportKafkaTopicsHandler(serverCtx)),
			},
			{
				// 重置位点（行内按钮，资源级操作）：can_edit 资源关系守卫
				//（能编辑该 topic 才能重置其位点，与编辑/删除同类，不建码）；
				// 管理码回退由 requireTopicPerm 内置。
				Method:  http.MethodPost,
				Path:    "/topics/:id/reset-offset",
				Handler: requireTopicPerm(serverCtx, "can_edit")(ResetKafkaTopicOffsetHandler(serverCtx)),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/v1/kafka"),
	)

	// Topic 名单接口：不挂 AdminRole——逻辑层已按 manage 码 / can_view 判定
	//（与前端 manage 码按钮显隐一致）。
	server.AddRoutes(
		[]rest.Route{
			{
				Method: http.MethodGet,
				Path:   "/topics/:id/grants",
				// 名单可见性判定上收路由守卫（can_view + 管理码回退，
				// 原 logic 内 manageOK/can_view 两级同语义）。
				Handler: requireTopicPerm(serverCtx, "can_view")(ListTopicGrantsHandler(serverCtx)),
			},
			{
				Method: http.MethodPost,
				Path:   "/topics/:id/grants",
				// 名单管理权判定上收路由守卫（owner + 管理码回退，不含 editor）。
				Handler: requireTopicManagePerm(serverCtx)(GrantTopicHandler(serverCtx)),
			},
			{
				Method: http.MethodDelete,
				Path:   "/topics/:id/grants/:targetType/:targetId",
				// 名单管理权判定上收路由守卫（同 grant）。
				Handler: requireTopicManagePerm(serverCtx)(RevokeTopicGrantHandler(serverCtx)),
			},
			{
				Method: http.MethodGet,
				Path:   "/topics/:id/consumers",
				// 可见权判定上收路由守卫（guard.go）：kafka_topic:can_view 主判定
				// + 管理码回退；logic 不再内联 topicCanView。
				Handler: requireTopicPerm(serverCtx, "can_view")(GetTopicConsumersHandler(serverCtx)),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/v1/kafka"),
	)

	// 集群管理（Cluster CRUD，菜单 kafka:clusters）：列表读取无守卫（页面
	// 入口由菜单码 kafka-clusters:read 过滤）；新建/编辑/删除挂管理码守卫
	// requireClusterManageCode（kafka-clusters:manage，管理档全权）。
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/clusters",
				Handler: ListClusterHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/clusters",
				Handler: requireClusterManageCode(serverCtx)(CreateClusterHandler(serverCtx)),
			},
			{
				Method:  http.MethodPut,
				Path:    "/clusters/:id",
				Handler: requireClusterManageCode(serverCtx)(UpdateClusterHandler(serverCtx)),
			},
			{
				Method:  http.MethodDelete,
				Path:    "/clusters/:id",
				Handler: requireClusterManageCode(serverCtx)(DeleteClusterHandler(serverCtx)),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/v1/kafka"),
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
