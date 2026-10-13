// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"strings"

	"nemo/services/sample/internal/repo"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// CreateItemLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateItemLogic 构造业务规则层实例。
func NewCreateItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateItemLogic {
	return &CreateItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateItem 执行业务逻辑：校验 name 必填（空串与纯空白均视为空）后创建 Item，
// types↔repo 模型转换在本层完成（repo.Item 存储模型不外泄）；存储异常先记
// 原始 err 再经 From 归一化为 500 返回，响应体不携带内部错误细节。
func (l *CreateItemLogic) CreateItem(req *types.CreateItemReq) (*types.Item, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, kiterrors.BadRequest("SAMPLE_NAME_REQUIRED", "name 不能为空")
	}
	it, err := l.svcCtx.ItemRepo.Create(l.ctx, repo.Item{Name: req.Name, Desc: req.Desc})
	if err != nil {
		l.Logger.Errorf("create item: %v", err)
		return nil, kiterrors.From(err)
	}
	return &types.Item{Id: formatItemID(it.ID), Name: it.Name, Desc: it.Desc}, nil
}
