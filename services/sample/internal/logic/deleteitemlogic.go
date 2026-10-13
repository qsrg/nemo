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

// DeleteItemLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteItemLogic 构造业务规则层实例。
func NewDeleteItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteItemLogic {
	return &DeleteItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteItem 执行业务逻辑：按 id 删除；未命中（errors.Is 判定 repo.ErrNotFound）
// 转 404 SAMPLE_NOT_FOUND，存储异常先记原始 err 再归一化为 500 返回。
func (l *DeleteItemLogic) DeleteItem(req *types.ItemIdReq) error {
	id, err := parseItemID(req.Id)
	if err != nil {
		return kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")
	}
	err = l.svcCtx.ItemRepo.Delete(l.ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")
		}
		l.Logger.Errorf("delete item %d: %v", id, err)
		return kiterrors.From(err)
	}
	return nil
}
