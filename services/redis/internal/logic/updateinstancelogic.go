// updateinstancelogic.go 承载更新实例的业务规则：addr/description 全量覆盖
// （name 为对象身份不可改），编辑权判定已上收路由守卫（can_edit + 管理码
// 回退），业务代码只关注业务。
package logic

import (
	"context"
	stderrors "errors"

	"nemo/kit/errors"
	"nemo/services/redis/internal/repo"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateInstanceLogic 承载更新实例接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type UpdateInstanceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateInstanceLogic 构造业务规则层实例。
func NewUpdateInstanceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateInstanceLogic {
	return &UpdateInstanceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateInstance 更新实例（编辑权判定已上收路由守卫 requireInstancePerm
// ("can_edit")：can_edit 主判定 + redis:manage 回退，无权 403 / 引擎故障
// 503 fail-closed）。判定先于存在性（与删除同口径，不暴露存在性枚举通道）。
func (l *UpdateInstanceLogic) UpdateInstance(req *types.UpdateInstanceReq, id string) (*types.InstanceResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	instID, err := parseInstanceID(id)
	if err != nil {
		return nil, err
	}

	// 先取存量（校验存在性与审计快照；判权在路由守卫已完成）。
	prev, err := l.svcCtx.InstanceRepo.Get(l.ctx, instID)
	if err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return nil, errors.NotFound("REDIS_INSTANCE_NOT_FOUND", "实例不存在")
		}
		l.Logger.Errorf("实例读取失败 id=%d: %v", instID, err)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "实例数据读取失败")
	}

	prev.UpdateBy = operator
	prev.Addr = req.Addr
	prev.Description = req.Description
	var saved repo.Instance
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "update_instance", instanceObject(instID), prev, func() error {
		updated, werr := l.svcCtx.InstanceRepo.Update(l.ctx, prev)
		if werr != nil {
			return werr
		}
		saved = updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &types.InstanceResp{
		Id:          id,
		Name:        saved.Name,
		Addr:        saved.Addr,
		Description: saved.Description,
		CreatedBy:   saved.CreatedBy,
	}, nil
}
