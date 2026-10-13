// updateclusterlogic.go 编辑集群（PUT /v1/kafka/clusters/:id）。
// 编辑权判定在路由守卫（requireClusterManageCode）；code 为对象身份不可改
// （与 Topic name 同规则），只更新 dc/address/remark；未命中返回 404。
package logic

import (
	"context"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// UpdateClusterLogic 承载集群编辑接口的业务规则。
type UpdateClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateClusterLogic 构造集群编辑业务规则层实例。
func NewUpdateClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateClusterLogic {
	return &UpdateClusterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateCluster 更新集群属性（code 不可改）；目标不存在返回 404。
func (l *UpdateClusterLogic) UpdateCluster(req *types.UpdateClusterReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	// 目标存在性预检（404 语义，与 Topic 编辑口径一致：判定先于存在性）。
	if _, gerr := l.svcCtx.ClusterRepo.GetById(l.ctx, req.Id); gerr != nil {
		if isNotFound(gerr) {
			return errors.NotFound("AUTHZ_CLUSTER_NOT_FOUND", "集群不存在: "+itoa(req.Id))
		}
		l.Logger.Errorf("集群存在性预检失败 id=%d: %v", req.Id, gerr)
		return errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	upd := repo.Cluster{
		ID:        req.Id,
		Dc:        strings.TrimSpace(req.Dc),
		Address:   strings.TrimSpace(req.Address),
		Remark:    strings.TrimSpace(req.Remark),
		UpdateBy:  operator,
		UpdatedAt: time.Now(),
	}
	return writeWithAudit(l.ctx, l.svcCtx, operator, "update_cluster", "cluster:id:"+itoa(req.Id), req, func() error {
		return l.svcCtx.ClusterRepo.Update(l.ctx, upd)
	})
}
