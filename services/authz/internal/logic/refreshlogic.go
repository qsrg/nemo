// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"errors"

	authzkit "nemo/kit/authz"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// RefreshLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type RefreshLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRefreshLogic 构造业务规则层实例。
func NewRefreshLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshLogic {
	return &RefreshLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Refresh 换发平台 token：旧 token 未过期（到 logic 时 go-zero jwt 中间件
// 已验签名与有效期）即续签，流程 fail-closed：
//  1. 从 ctx 会话取身份（无会话 401；typ=temp 的 token 在 authzkit.Identity
//     中间件已被 401 拒绝，到不了这里）；
//  2. repo 按 (cas, sub) 实时刷新用户资料（name 以 repo 为准；用户不存在
//     ——如删库后——401 拒续签，防已删用户凭旧 token 无限续命）；
//  3. roles 实时重取：以【当前旧 token】透传调 authz ListPlatformRoles
//     （角色可能在旧 token 签发后被授予/回收，续签即收敛到实态）；
//     失败 503 AUTH_ROLE_FETCH_FAILED 拒续签（fail-closed，与登录链路同语义）；
//  4. 签发新 token（typ=access，复用 IssueToken）。
func (l *RefreshLogic) Refresh() (resp *types.TokenResp, err error) {
	s, err := authzkit.IdentityFrom(l.ctx)
	if err != nil {
		return nil, kiterrors.Unauthorized("AUTH_UNAUTHORIZED", "未认证")
	}

	u, err := l.svcCtx.Users.GetByIdp(l.ctx, idpTypeCAS, s.Sub)
	if errors.Is(err, repo.ErrNotFound) {
		// 用户不存在于 repo（如删库后旧 token 仍在有效期）：拒续签。
		return nil, kiterrors.Unauthorized("AUTH_USER_NOT_FOUND", "用户不存在")
	}
	if err != nil {
		logx.Errorf("刷新时查询用户失败 sub=%s: %v", s.Sub, err)
		return nil, kiterrors.Internal("AUTH_USER_QUERY_FAILED", "用户查询失败")
	}

	// 不再派生 super_admin 角色：去角色化后权限码是唯一真源，me/permissions
	// 实时查引擎 permission.can，不再从 claims.roles 取任何权限。
	roles := []string{}

	tr, err := IssueToken(l.ctx, l.svcCtx, u, roles)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}
