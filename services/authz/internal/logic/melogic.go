// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	authzkit "nemo/kit/authz"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// MeLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type MeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMeLogic 构造业务规则层实例。
func NewMeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MeLogic {
	return &MeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Me 执行业务逻辑：从 ctx 会话（authzkit.Identity 中间件自正式平台 token 的
// claims 注入）直取返回，不查任何下游——token 未过期即代表会话有效，
// roles/name/tenant 以签发时 claims 为准（roles 的实时性由 refresh 续签
// 重取保证）。无会话（未经中间件/token 无 sub）按未认证 401。
func (l *MeLogic) Me() (resp *types.MeResp, err error) {
	s, err := authzkit.IdentityFrom(l.ctx)
	if err != nil {
		return nil, kiterrors.Unauthorized("AUTH_UNAUTHORIZED", "未认证")
	}
	roles := s.Roles
	if roles == nil {
		// 无角色用户也输出空数组（非 null），与前端约定一致。
		roles = []string{}
	}
	return &types.MeResp{
		Sub:    s.Sub,
		Name:   s.Name,
		Tenant: s.Tenant,
		Roles:  roles,
	}, nil
}
