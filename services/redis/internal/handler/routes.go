// routes.go 注册 redis 独立服务的全部路由：
//   - /v1/redis/* 实例 CRUD（前端 apps/redis 经 vite proxy 接入）；
//   - /healthz、/readyz 健康检查（可运维性规范，认证之外独立注册）。
//
// 身份校验：rest.WithJwt（与 authz 共享 AccessSecret）+ authzkit.Identity
// 中间件注入会话（claims.sub 供 logic operatorFrom 取操作人）。
// 资源级判定：带实例 ID 的行级接口挂声明式守卫 requireInstancePerm
// （guard.go，编辑 can_edit / 删除 can_delete + 管理码回退）；列表过滤、
// 新建管理码判定等无法中间件化的判定仍在 logic 内（见各 logic 注释）。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"

	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
)

// RegisterHandlers 注册全部路由。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 受保护路由：jwt 校验 + 会话注入。
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/instances",
				Handler: ListInstancesHandler(serverCtx),
			},
			{
				Method: http.MethodPost,
				Path:   "/instances",
				// 新建入口判定上收路由守卫（guard.go）：create 动作码
				// redis:create 或管理码 redis:manage（管理档兜底全权）；
				// logic 不再内联 manage 判定。
				Handler: requireCodeOrManage(serverCtx, logic.RedisCreateCode)(CreateInstanceHandler(serverCtx)),
			},
			{
				Method: http.MethodPut,
				Path:   "/instances/:id",
				// 编辑权判定上收路由守卫（guard.go）：redis_instance:can_edit
				// 主判定 + 管理码回退；logic 不再内联 can_edit。
				Handler: requireInstancePerm(serverCtx, "can_edit")(UpdateInstanceHandler(serverCtx)),
			},
			{
				Method: http.MethodDelete,
				Path:   "/instances/:id",
				// 删除判定上收路由守卫（can_delete + 管理码回退）。can_delete
				// 为独立授权边界（owner），可编辑者不自动可删。幂等分支保留在
				// logic（资源不存在返成功——守卫前置后仅管理码持有者可达该
				// 分支，与 PUT 的判定先于存在性同口径，同时关闭存在性枚举通道）。
				Handler: requireInstancePerm(serverCtx, "can_delete")(DeleteInstanceHandler(serverCtx)),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/v1/redis"),
	)

	// 集群管理（分组维度，基础配置数据）：列表读取无守卫（页面入口由菜单码
	// redis:read 过滤，选择器数据源）；新建/删除挂管理码守卫（管理档维护，
	// 无按钮动作码——与 kafka 集群同口径）。集群下仍有实例时由 logic 层 409
	// 拒绝删除（归属完整性应用层保证，禁外键规范）。
	server.AddRoutes(
		[]rest.Route{
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
			{
				// 授权对账（补偿）：扫描业务表补写缺失的 owner 元组（创建路径
				// 元组写入重试耗尽的最终兜底）。manage 码守卫——运维操作。
				Method:  http.MethodPost,
				Path:    "/reconcile",
				Handler: requireClusterManageCode(serverCtx)(ReconcileRedisHandler(serverCtx)),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/v1/redis"),
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
