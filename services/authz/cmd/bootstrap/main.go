// cmd/bootstrap 是独立的引导工具入口：按配置启动引擎进程内实例，
// 执行 bootstrap.Run（幂等建 Store/模型/根组织/超管）后退出。
// 薄入口——全部逻辑在 internal/bootstrap（可测核心）。
//
// 运行约定：工作目录为 services/authz（模型文件 model.fga 与配置
// etc/authz-api.yaml 均相对该目录解析，与服务主入口同约定）。
package main

import (
	"context"
	"flag"
	"os"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"

	kitlog "nemo/kit/logging"

	"nemo/services/authz/internal/bootstrap"
	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
)

// main 读配置（优先级：-f flag > 环境变量 AUTHZ_API_CONFIG > 默认 etc/authz-api.yaml），
// 启动引擎执行幂等引导；失败快速退出（退出码 1）。
func main() {
	// 默认配置路径：环境变量优先，其次与服务主入口相同的 etc 相对路径。
	defPath := os.Getenv("AUTHZ_API_CONFIG")
	if defPath == "" {
		defPath = "etc/authz-api.yaml"
	}
	configFile := flag.String("f", defPath, "the config file")
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// kit/logging 日志初始化（AGENTS.md 禁 print 系输出，统一走 logx）。
	kitlog.Init("authz-bootstrap", false)

	h, err := engine.Start(context.Background(), engine.Options{
		HTTPAddr:  c.Engine.HTTPAddr,
		StoreName: c.Engine.StoreName,
		Datastore: c.Engine.Datastore,
		MySQLDSN:  c.Engine.MySQLDSN,
	})
	if err != nil {
		logx.Errorf("启动引擎失败: %v", err)
		os.Exit(1)
	}
	defer h.Stop()

	// 幂等引导：重复执行结果一致（EnsureModel 按名建 Store、WriteTuples 读后写）。
	// 初始超管由配置文件 Bootstrap 段声明（用户/组织编码，可多个；未声明默认 admin）。
	if err := bootstrap.Run(context.Background(), h, bootstrap.Options{
		AdminUsers: c.Bootstrap.AdminUsers,
		AdminOrgs:  c.Bootstrap.AdminOrgs,
	}); err != nil {
		logx.Errorf("bootstrap 失败: %v", err)
		os.Exit(1)
	}
}
