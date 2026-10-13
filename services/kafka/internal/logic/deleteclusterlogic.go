// deleteclusterlogic.go 删除集群（DELETE /v1/kafka/clusters/:id）。
// 删除权判定在路由守卫（requireClusterManageCode）；未命中返回 404。
package logic

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// DeleteClusterLogic 承载集群删除接口的业务规则。
type DeleteClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteClusterLogic 构造集群删除业务规则层实例。
func NewDeleteClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteClusterLogic {
	return &DeleteClusterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteCluster 删除集群；目标不存在返回 404。
func (l *DeleteClusterLogic) DeleteCluster(req *types.ClusterIdReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_cluster", "cluster:id:"+itoa(req.Id), req, func() error {
		derr := l.svcCtx.ClusterRepo.Delete(l.ctx, req.Id)
		if isNotFound(derr) {
			return errors.NotFound("AUTHZ_CLUSTER_NOT_FOUND", "集群不存在: "+itoa(req.Id))
		}
		return derr
	})
}
