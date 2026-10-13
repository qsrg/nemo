// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	kiterrors "nemo/kit/errors"
	"nemo/services/authz/internal/casdir"
	"nemo/services/authz/internal/orgsync"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// SyncOrgsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type SyncOrgsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewSyncOrgsLogic 构造业务规则层实例。
func NewSyncOrgsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncOrgsLogic {
	return &SyncOrgsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SyncOrgs 手动触发 CAS 组织架构同步（与定时任务同入口）：按配置构造
// CAS 目录客户端，执行一轮全量对账（拉 /orgs、/users → diff → 落 orgs
// 注册表与引擎 parent/member 元组），返回写入动作摘要。
// 与定时任务的区别仅在触发方式——对账幂等，两者可任意并行触发（引擎
// 写幂等保证收敛，不产生重复元组）。
func (l *SyncOrgsLogic) SyncOrgs() (*types.SyncOrgsResp, error) {
	conf := l.svcCtx.Config.CasSync
	if conf.BaseURL == "" {
		return nil, kiterrors.Internal("AUTHZ_CAS_NOT_CONFIGURED", "未配置 CAS 服务地址")
	}
	cas := casdir.NewClient(conf.BaseURL, conf.InsecureSkipVerify, conf.Timeout)

	res, err := orgsync.RunOnce(l.ctx, l.svcCtx, cas)
	if err != nil {
		// 同步失败已在审计与 orgsync 内告警日志留痕；管理面返回语义 503
		//（可能是 CAS 不可用，也可能是引擎写故障，原因见审计 reason）。
		logx.Errorf("[orgsync] 手动同步失败: %v", err)
		return nil, kiterrors.New(503, "AUTHZ_CAS_SYNC_FAILED", "组织同步失败，请检查 CAS 与授权引擎可用性")
	}
	l.Infof("[orgsync] 手动同步完成: %+v", res)
	return &types.SyncOrgsResp{
		OrgsUpserted:   res.OrgsUpserted,
		ParentsWritten: res.ParentsWritten,
		ParentsDeleted: res.ParentsDeleted,
		MembersWritten: res.MembersWritten,
		MembersDeleted: res.MembersDeleted,
		OrgsDeleted:    res.OrgsDeleted,
		OrgsSkipped:    res.OrgsSkipped,
		DanglingUsers:  res.DanglingUsers,
	}, nil
}
