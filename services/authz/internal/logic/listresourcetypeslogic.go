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

	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListResourceTypesLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListResourceTypesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListResourceTypesLogic 构造业务规则层实例。
func NewListResourceTypesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListResourceTypesLogic {
	return &ListResourceTypesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListResourceTypes 列出全部已注册资源类型：Actions 从注册表 DslBlock
// 反解（注册配置即源头，动作码不另存列），按类型名升序输出。
func (l *ListResourceTypesLogic) ListResourceTypes() (*types.ResourceTypeListResp, error) {
	rts, err := l.svcCtx.ResourceTypeRepo.List(l.ctx)
	if err != nil {
		return nil, errors.Internal("AUTHZ_INTERNAL", "查询资源类型失败")
	}
	items := make([]types.ResourceTypeResp, 0, len(rts))
	for _, rt := range rts {
		items = append(items, types.ResourceTypeResp{
			Name:       rt.Name,
			Actions:    engine.ExtractActions(rt.DslBlock),
			ManageCode: rt.ManageCode,
		})
	}
	return &types.ResourceTypeListResp{Items: items, Total: len(items)}, nil
}
