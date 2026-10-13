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

	"nemo/services/sample/internal/repo"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// GetItemLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type GetItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGetItemLogic 构造业务规则层实例。
func NewGetItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetItemLogic {
	return &GetItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetItem 执行业务逻辑：按 id 查询；未命中（errors.Is 判定 repo.ErrNotFound）
// 转 404 SAMPLE_NOT_FOUND，存储异常先记原始 err 再归一化为 500 返回。
func (l *GetItemLogic) GetItem(req *types.ItemIdReq) (*types.Item, error) {
	id, err := parseItemID(req.Id)
	if err != nil {
		return nil, kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")
	}
	it, err := l.svcCtx.ItemRepo.Get(l.ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")
		}
		l.Logger.Errorf("get item %d: %v", id, err)
		return nil, kiterrors.From(err)
	}
	return &types.Item{Id: formatItemID(it.ID), Name: it.Name, Desc: it.Desc}, nil
}
