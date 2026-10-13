// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CasCallbackLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CasCallbackLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCasCallbackLogic 构造业务规则层实例。
func NewCasCallbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CasCallbackLogic {
	return &CasCallbackLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CasCallback 执行业务逻辑：ticket 换用户信息 → 用户映射 → 取角色
// （fail-closed）→ 签发平台 JWT；编排见 cascallback.go。
func (l *CasCallbackLogic) CasCallback(req *types.CasCallbackReq) (resp *types.TokenResp, err error) {
	return casCallback(l.ctx, l.svcCtx, req)
}
