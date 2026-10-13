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
	"context"
	"flag"
	"time"

	"net/http"
	"os"

	authzkit "nemo/kit/authz"
	"nemo/services/authz/internal/bootstrap"
	"nemo/services/authz/internal/casdir"
	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/handler"
	"nemo/services/authz/internal/orgsync"
	"nemo/services/authz/internal/reconcile"
	"nemo/services/authz/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
	kitlog "nemo/kit/logging"
)

var configFile = flag.String("f", "etc/authz-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// kit/logging 日志初始化：main 最先调用；服务名生成后如与部署名不符，按服务名修正。
	kitlog.Init("authz-api", false)
	// kit/errors 注册 go-zero 全局错误出口：httpx.ErrorCtx 返回的错误统一转为约定错误体。
	kiterrors.InstallErrorHandler()

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// kit/logging 中间件：RequestID 最外层生成/透传 request_id，AccessLog 依赖它打访问日志。
	server.Use(kitlog.RequestID)
	server.Use(kitlog.AccessLog)
	// 身份透传中间件：把标准 claims（sub/roles）以类型化方式注入 ctx，
	// 供 AdminRole 中间件与 logic 层取当前用户（背景见 kit/authz authn.go 包注释）。
	server.Use(authzkit.Identity(c.Auth.AccessSecret))

	ctx := svc.NewServiceContext(c)
	defer ctx.Stop()

	// BOOTSTRAP=1 内联钩子：服务启动前先执行幂等引导（建 Store/模型/根组织/超管，
	// 与 cmd/bootstrap 独立工具走同一 bootstrap.Run）。失败则退出（退出码 1；
	// 引导期依赖不可用必须快速失败）——os.Exit 不走 defer，进程即将退出可接受。
	if os.Getenv("BOOTSTRAP") == "1" {
		if err := bootstrap.Run(context.Background(), ctx.Engine, bootstrap.Options{
			AdminUsers: c.Bootstrap.AdminUsers,
			AdminOrgs:  c.Bootstrap.AdminOrgs,
		}); err != nil {
			logx.Errorf("BOOTSTRAP 内联引导失败: %v", err)
			os.Exit(1)
		}
	}

	// 启动模型重放（Phase C 改造②）：资源类型注册表有已注册类型时，以
	// "基础模型 + 全部注册块"重建完整模型并 EnsureModel——修复重启后引擎
	// 回落基础模型导致对注册类型的 check 503（Shares 页 bug）。必须在
	// bootstrap 之后执行（重放的合并基准含基础模型，且最终模型须含注册块）。
	if err := svc.ReplayRegisteredModel(context.Background(), ctx); err != nil {
		logx.Errorf("启动模型重放失败: %v", err)
		os.Exit(1)
	}

	// 审计对账定时任务：main 内 Ticker（间隔 Reconcile.Interval 默认 1m），
	// 每轮扫超时 pending（Reconcile.OlderThan 默认 5m）回查实态修正
	//（internal/reconcile.RunOnce，可测核心与挂载分离）。进程退出即随
	// 进程终止，无需显式停止。
	interval := c.Reconcile.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	olderThan := c.Reconcile.OlderThan
	if olderThan <= 0 {
		olderThan = 5 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			rctx, cancel := context.WithTimeout(context.Background(), interval)
			res, err := reconcile.RunOnce(rctx, ctx, olderThan)
			cancel()
			if err != nil {
				// 回查故障：本轮中止，条目保持 pending，下轮重试。
				logx.Errorf("审计对账轮中止: %v", err)
				continue
			}
			// 有修正动作时输出摘要（failed 明细已由 RunOnce 内告警）。
			if res.Committed+res.Failed+res.Skipped > 0 {
				logx.Infof("审计对账完成: committed=%d failed=%d skipped=%d", res.Committed, res.Failed, res.Skipped)
			}
		}
	}()

	// CAS 组织架构同步定时任务（orgsync 全量对账）：Enabled 时启动先立即
	// 执行一轮（快速收敛），随后按 CasSync.Interval 周期对账——撤权时效 =
	// 同步周期。每轮为独立 CAS 客户端（无状态），失败记日志下轮自动重试
	//（同步幂等，重试安全）。手动触发接口 POST /v1/admin/orgs/sync 同入口。
	if c.CasSync.Enabled && c.CasSync.BaseURL != "" {
		go func() {
			syncInterval := c.CasSync.Interval
			if syncInterval <= 0 {
				syncInterval = 5 * time.Minute
			}
			runSync := func() {
				sctx, cancel := context.WithTimeout(context.Background(), syncInterval)
				defer cancel()
				res, err := orgsync.RunOnce(sctx, ctx, casdir.NewClient(
					c.CasSync.BaseURL, c.CasSync.InsecureSkipVerify, c.CasSync.Timeout))
				if err != nil {
					logx.Errorf("CAS 组织同步失败（下轮重试）: %v", err)
					return
				}
				if res.OrgsUpserted+res.ParentsWritten+res.ParentsDeleted+res.MembersWritten+
					res.MembersDeleted+res.OrgsDeleted+res.OrgsSkipped+res.DanglingUsers > 0 {
					logx.Infof("CAS 组织同步完成: %+v", res)
				}
			}
			runSync()
			ticker := time.NewTicker(syncInterval)
			defer ticker.Stop()
			for range ticker.C {
				runSync()
			}
		}()
	} else {
		logx.Infof("CAS 组织同步未启用（Enabled=%v BaseURL=%q），组织数据仅支持管理面手动维护与手动同步",
			c.CasSync.Enabled, c.CasSync.BaseURL)
	}

	handler.RegisterHandlers(server, ctx)

	// 健康检查：认证之外独立注册（可运维性规范）。
	// /healthz 存活探针恒 200；/readyz 就绪探针由 svc 提供
	// （引擎 HTTP 端口 dial 成功才 200，失败 503）。
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
