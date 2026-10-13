// kafka 独立微服务入口：Topic 业务数据自管（topics/audits 表），权限判定
// 与授权名单管理经 kit/authz 远程调用集中式 authz（8084），不内嵌引擎。
//
// kit 接线约定（kit/README.md，与 sample/authz 一致）：
//  1. kitlog.Init —— main 最先调用；
//  2. InstallErrorHandler —— 注册 go-zero 全局错误出口；
//  3. server.Use —— RequestID 最外层，AccessLog 其后。
package main

import (
	"flag"

	authzkit "nemo/kit/authz"
	"nemo/services/kafka/internal/config"
	"nemo/services/kafka/internal/handler"
	"nemo/services/kafka/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"

	kiterrors "nemo/kit/errors"
	kitlog "nemo/kit/logging"
)

var configFile = flag.String("f", "etc/kafka-api.yaml", "the config file")

// main 装配并启动 kafka 服务：加载配置 → kit 日志/全局错误出口 → rest
// server（RequestID/AccessLog/Identity 中间件）→ ServiceContext 与路由注册。
func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// kit/logging 日志初始化：main 最先调用。
	kitlog.Init("kafka-api", false)
	// kit/errors 注册 go-zero 全局错误出口。
	kiterrors.InstallErrorHandler()

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// kit/logging 中间件：RequestID 最外层生成/透传 request_id，AccessLog 依赖它。
	server.Use(kitlog.RequestID)
	server.Use(kitlog.AccessLog)

	// 身份中间件：解析 Bearer token 把 claims.sub 等注入 ctx（operatorFrom
	// 取操作人）。JWT 校验仍由路由组 rest.WithJwt 负责，本中间件只做身份透传。
	server.Use(authzkit.Identity(c.Auth.AccessSecret))

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	logx.Infof("Starting server at %s:%d...", c.Host, c.Port)
	server.Start()
}
