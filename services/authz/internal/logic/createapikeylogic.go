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

// CreateApiKeyLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateApiKeyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateApiKeyLogic 构造业务规则层实例。
func NewCreateApiKeyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateApiKeyLogic {
	return &CreateApiKeyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateApiKey 执行业务逻辑：签发编排见 createapikey.go（薄委托）。
func (l *CreateApiKeyLogic) CreateApiKey(req *types.CreateApiKeyReq) (resp *types.ApiKeyResp, err error) {
	return createApiKey(l.ctx, l.svcCtx, req)
}
