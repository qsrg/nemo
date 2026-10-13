// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// kit 接线约定（kit/README.md）：
//   1. kitlog.Init        —— main 最先调用，服务名取 .api 的 service 名；
//                            生产环境部署时第二个参数改 true。
//   2. InstallErrorHandler —— 注册 go-zero 全局错误出口，main 调用一次；
//                            logic 返回 kit/errors 语义错误时统一转为约定 JSON 错误体。
//   3. server.Use         —— RequestID 必须挂最外层，AccessLog 挂其后，顺序不可颠倒。

package main

import (
	"flag"
	"net/http"

	"nemo/services/sample/internal/config"
	"nemo/services/sample/internal/handler"
	"nemo/services/sample/internal/svc"

	authzkit "nemo/kit/authz"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
	kitlog "nemo/kit/logging"
)

var configFile = flag.String("f", "etc/sample-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// kit/logging 日志初始化：main 最先调用；服务名生成后如与部署名不符，按服务名修正。
	kitlog.Init("sample-api", false)
	// kit/errors 注册 go-zero 全局错误出口：httpx.ErrorCtx 返回的错误统一转为约定错误体。
	kiterrors.InstallErrorHandler()

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// kit/logging 中间件：RequestID 最外层生成/透传 request_id，AccessLog 依赖它打访问日志。
	server.Use(kitlog.RequestID)
	server.Use(kitlog.AccessLog)
	// 统一身份中间件（kit/authz，与 authz/kafka 同一套）：解析 Bearer token 把
	// claims.sub 等注入 ctx（logic 经 authzkit.IdentityFrom 取操作人），并把
	// 原始 token 写入 authz ctx 键供 Remote 透传。JWT 校验仍由路由组
	// rest.WithJwt 负责（需要鉴权的路由挂上即可），本中间件只做身份透传。
	server.Use(authzkit.Identity(c.Auth.AccessSecret))

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// 权限冒烟路由（Task 8）：kit/authz Require 挂一行中间件保护一条独立路由
	// （不侵入 .api 契约的既有 Item 路由）。perm "order:can_edit" 语义：
	// 对象类型 order、出口关系 can_edit，对象实例 ID 取自路径末段；身份（JWT）
	// 由 Require 提取进 ctx，经 Remote 透传给 authz 判定——无权 403
	// AUTHZ_DENIED，判定服务不可用 503 AUTHZ_SERVICE_UNAVAILABLE（fail-closed）。
	// 说明：计划/spec 示例写 "order:manage"，但授权模型（model.fga + 资源类型
	// 注册模板）无 manage 关系、出口固定为 can_* 白名单，冒烟取真实出口 can_edit。
	authzChecker := authzkit.New(authzkit.Config{BaseURL: c.Authz.BaseURL, Timeout: c.Authz.Timeout})
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/v1/managed-items/:id",
		Handler: authzkit.Require(authzChecker, "order:can_edit")(handler.GetItemHandler(ctx)),
	})

	// 健康检查：认证之外独立注册（可运维性规范）。
	// /healthz 存活探针恒 200；/readyz 就绪探针由 svc 提供判定
	// （内存模式恒 200，MySQL 模式探活成功才 200，失败 503）。
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
		Handler: ctx.ReadyHandler,
	})

	// 启动输出走 logx（AGENTS.md 禁 print 系输出），与统一日志体系一致。
	logx.Infof("Starting server at %s:%d...", c.Host, c.Port)
	server.Start()
}
