// listclusterslogic.go 实现集群列表：管理档（管理码持有者）全量；普通
// 用户 = 引擎授权（rmq_cluster can_view，owner 派生 viewer，空集群也能
// 授权——授权对象与集群行一一对应，先授权后建 topic 也成立）。行级
// canManageGrants 徽标（管理码或该集群 owner）批量取回，前端控制
// 「集群权限」配置入口。
package logic

import (
	"context"
	"net/http"
	"strconv"

	"nemo/kit/errors"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListClustersLogic 承载集群列表接口的业务规则。
type ListClustersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListClustersLogic 构造集群列表业务规则实例。
func NewListClustersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListClustersLogic {
	return &ListClustersLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// ListClusters 集群列表（管理码全量；普通用户引擎 can_view 授权集合）。
func (l *ListClustersLogic) ListClusters() (*types.RocketmqClusterListResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	clusters, err := l.svcCtx.ClusterRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("集群列表读取失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	manageOK, err := codeGranted(l.ctx, l.svcCtx, RocketMQTopicsManageCode)
	if err != nil {
		l.Logger.Errorf("manage 判定失败 sub=%s: %v", sub, err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
	}

	// owner 徽标集合（manage/普通视角都需要：owner = 集群名单管理权，
	// 「集群权限」入口直接消费，不重复判定）。
	ownerSet, err := clusterRelationSet(l.ctx, l.svcCtx, "owner")
	if err != nil {
		l.Logger.Errorf("owner 集合加载失败 sub=%s: %v", sub, err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
	}

	visible := map[string]bool{}
	if !manageOK {
		viewSet, err := clusterRelationSet(l.ctx, l.svcCtx, "can_view")
		if err != nil {
			l.Logger.Errorf("集群 can_view 集合加载失败 sub=%s: %v", sub, err)
			return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
		}
		visible = viewSet
	}

	items := make([]types.RocketmqClusterResp, 0, len(clusters))
	for _, c := range clusters {
		obj := rmqClusterObject(c.ID)
		if !manageOK && !visible[obj] {
			continue
		}
		items = append(items, types.RocketmqClusterResp{
			Id:              strconv.FormatInt(c.ID, 10),
			Code:            c.Code,
			Remark:          c.Remark,
			CreatedBy:       c.CreatedBy,
			UpdatedAt:       c.UpdatedAt.Format("2006-01-02 15:04:05"),
			CanManageGrants: manageOK || ownerSet[obj],
		})
	}
	return &types.RocketmqClusterListResp{Items: items}, nil
}
