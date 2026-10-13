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
	"strings"

	"nemo/services/sample/internal/repo"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// UpdateItemLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type UpdateItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateItemLogic 构造业务规则层实例。
func NewUpdateItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateItemLogic {
	return &UpdateItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateItem 执行业务逻辑：先校验 name 必填（400 优先于存在性判断），再按 id
// 全量覆盖 Name/Desc；未命中（errors.Is 判定 repo.ErrNotFound）转 404
// SAMPLE_NOT_FOUND，存储异常先记原始 err 再归一化为 500 返回。
func (l *UpdateItemLogic) UpdateItem(req *types.UpdateItemReq) (*types.Item, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, kiterrors.BadRequest("SAMPLE_NAME_REQUIRED", "name 不能为空")
	}
	id, err := parseItemID(req.Id)
	if err != nil {
		return nil, kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")
	}
	it, err := l.svcCtx.ItemRepo.Update(l.ctx, repo.Item{ID: id, Name: req.Name, Desc: req.Desc})
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")
		}
		l.Logger.Errorf("update item %d: %v", id, err)
		return nil, kiterrors.From(err)
	}
	return &types.Item{Id: formatItemID(it.ID), Name: it.Name, Desc: it.Desc}, nil
}
