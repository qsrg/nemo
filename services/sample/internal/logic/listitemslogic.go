// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// ListItemsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListItemsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListItemsLogic 构造业务规则层实例。
func NewListItemsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListItemsLogic {
	return &ListItemsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListItems 执行业务逻辑：返回全部 Item 与总数；空列表保证 Items 为空切片
// （JSON 序列化为 [] 而非 null），存储异常先记原始 err 再归一化为 500 返回。
func (l *ListItemsLogic) ListItems() (*types.ListItemsResp, error) {
	list, err := l.svcCtx.ItemRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("list items: %v", err)
		return nil, kiterrors.From(err)
	}
	items := make([]types.Item, 0, len(list))
	for _, it := range list {
		items = append(items, types.Item{Id: formatItemID(it.ID), Name: it.Name, Desc: it.Desc})
	}
	return &types.ListItemsResp{Items: items, Total: len(items)}, nil
}
