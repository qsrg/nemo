// deleteclusterlogic.go 实现删除集群：集群下仍有 topic 时 409 拒绝
//（归属完整性应用层保证，禁外键规范）；未命中 404。守卫在路由层
//（管理码），logic 只做业务与存储。
package logic

import (
	"context"
	stdErrors "errors"
	"net/http"

	kiterrors "nemo/kit/errors"
	"nemo/services/rocketmq/internal/repo"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteClusterLogic 承载删除集群接口的业务规则。
type DeleteClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteClusterLogic 构造删除集群业务规则实例。
func NewDeleteClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteClusterLogic {
	return &DeleteClusterLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// DeleteCluster 删除集群：集群下仍有 topic 按 409 拒绝（防孤儿数据）；
// 未命中按 404。
func (l *DeleteClusterLogic) DeleteCluster(req *types.DeleteRocketmqClusterReq) error {
	if req.Id <= 0 {
		return kiterrors.New(http.StatusBadRequest, "AUTHZ_OBJECT_INVALID", "集群 ID 非法")
	}
	topics, err := l.svcCtx.TopicRepo.ListByCluster(l.ctx, req.Id)
	if err != nil {
		l.Logger.Errorf("集群 topic 计数失败 id=%d: %v", req.Id, err)
		return kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}
	if len(topics) > 0 {
		return kiterrors.New(http.StatusConflict, "CLUSTER_NOT_EMPTY", "集群下仍有 topic，无法删除")
	}
	if err := l.svcCtx.ClusterRepo.Delete(l.ctx, req.Id); err != nil {
		if stdErrors.Is(err, repo.ErrClusterNotFound) {
			return kiterrors.New(http.StatusNotFound, "CLUSTER_NOT_FOUND", "集群不存在")
		}
		l.Logger.Errorf("删除集群失败 id=%d: %v", req.Id, err)
		return kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "集群删除失败")
	}
	return nil
}
