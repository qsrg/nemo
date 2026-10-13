// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"net/http"

	"nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// MePermissionsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type MePermissionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMePermissionsLogic 构造业务规则层实例。
func NewMePermissionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MePermissionsLogic {
	return &MePermissionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MePermissions 当前用户权限码集合（去角色化改造）：不再读 claims.roles 与
// 规则表，改为对授权引擎实时查询（effectivePermCodes：ListObjects
// user:{sub} can permission → 权限码数组）——权限点对象 id = 权限码（如
// redis:read，通配 *:read），授权 = 人/组织 × 权限点一条 can 元组，组织继承
// 由 org#member userset 原生承担，授予/撤权即时生效（无需重新登录）。
//   - 身份缺失 → 401 fail-closed；
//   - 引擎查询失败 → 503 fail-closed（宁可拒绝下发也不放行猜测；
//     与判定面"存储/引擎故障 503"一致）；
//   - 无权限 → 200 空数组（非 null）。
func (l *MePermissionsLogic) MePermissions() (*types.MePermissionsResp, error) {
	sub, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	perms, err := effectivePermCodes(l.ctx, l.svcCtx.Engine, sub)
	if err != nil {
		l.Logger.Errorf("权限引擎实时查询失败 sub=%s: %v", sub, err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用，权限集合不可用")
	}

	return &types.MePermissionsResp{Perms: perms}, nil
}
