// Code scaffolded by goctl. Safe to edit.
// goctl {{.version}}
//
// kit 接线约定（kit/README.md）：
//   1. kitlog.Init        —— main 最先调用，服务名取 .api 的 service 名；
//                            生产环境部署时第二个参数改 true。
//   2. InstallErrorHandler —— 注册 go-zero 全局错误出口，main 调用一次；
//                            logic 返回 kit/errors 语义错误时统一转为约定 JSON 错误体。
//   3. server.Use         —— RequestID 必须挂最外层，AccessLog 挂其后，顺序不可颠倒。
//   4. authzkit.Identity   —— 统一身份中间件（平台鉴权接入，见 kit/README §统一鉴权）；
//                            依赖 .api 声明 jwt（config 自动生成 Auth 块，AccessSecret
//                            须与 authz 一致）；需要鉴权的路由再挂 rest.WithJwt +
//                            authzkit.Require。

package main

import (
	"flag"

	{{.importPackages}}

	"github.com/zeromicro/go-zero/core/logx"

	authzkit "nemo/kit/authz"
	kiterrors "nemo/kit/errors"
	kitlog "nemo/kit/logging"
)

var configFile = flag.String("f", "etc/{{.serviceName}}.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// kit/logging 日志初始化：main 最先调用；服务名生成后如与部署名不符，按服务名修正。
	kitlog.Init("{{.serviceName}}", false)
	// kit/errors 注册 go-zero 全局错误出口：httpx.ErrorCtx 返回的错误统一转为约定错误体。
	kiterrors.InstallErrorHandler()

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// kit/logging 中间件：RequestID 最外层生成/透传 request_id，AccessLog 依赖它打访问日志。
	server.Use(kitlog.RequestID)
	server.Use(kitlog.AccessLog)
	// 统一身份中间件（kit/authz，全平台同一套）：解析 Bearer token 把 claims.sub
	// 等注入 ctx（logic 经 authzkit.IdentityFrom 取操作人），并把原始 token 写入
	// authz ctx 键供 Remote 透传。JWT 校验由路由组 rest.WithJwt 负责（需要鉴权的
	// 路由挂上即可），本中间件只做身份透传——公开路由不挂 WithJwt 即可匿名访问。
	server.Use(authzkit.Identity(c.Auth.AccessSecret))

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// 启动输出走 logx（AGENTS.md 禁 print 系输出），与统一日志体系一致。
	logx.Infof("Starting server at %s:%d...", c.Host, c.Port)
	server.Start()
}
