// listrocketmqtopicslogic.go 实现 Topic 列表：先选集群（clusterId 必传），
// 集群级可见性过滤（rmq_cluster can_view / 管理码全量），集群可见即该
// 集群下全部 topic 可见——不再做逐行能力判定，无行级徽标（前端操作按钮
// 随集群可见恒显示，后端集群级守卫兜底）。
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

// ListRocketmqTopicsLogic 承载列表接口业务规则。
type ListRocketmqTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListRocketmqTopicsLogic 构造列表业务规则实例。
func NewListRocketmqTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRocketmqTopicsLogic {
	return &ListRocketmqTopicsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// ListRocketmqTopics 返回指定集群（clusterId）下可见 Topic 列表（当前用户
// 视角）：
//   - manage 码持有者：该集群全部 topic；
//   - 普通用户：集群可见（rmq_cluster can_view——含 owner 派生）才出列表，
//     否则空列表（集群不可见 = 该集群 topic 不可见，无需逐行过滤）。
//
// 集群不存在按 400 拒绝（集群是列表的必选上下文，防悬空查询）。
func (l *ListRocketmqTopicsLogic) ListRocketmqTopics(req *types.ListRocketmqTopicsReq) (*types.RocketmqTopicListResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	if req.ClusterId <= 0 {
		return nil, errors.New(http.StatusBadRequest, "AUTHZ_OBJECT_INVALID", "clusterId 必须为正整数")
	}
	// 集群必须真实存在（先选集群再出列表；悬空集群按 400 拒绝）。
	if _, err := l.svcCtx.ClusterRepo.GetById(l.ctx, req.ClusterId); err != nil {
		return nil, errors.New(http.StatusBadRequest, "CLUSTER_NOT_FOUND", "集群不存在")
	}

	manageOK, err := codeGranted(l.ctx, l.svcCtx, RocketMQTopicsManageCode)
	if err != nil {
		l.Logger.Errorf("manage 码判定失败 sub=%s: %v", sub, err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
	}

	if !manageOK {
		// 集群级可见性：rmq_cluster can_view 集合（owner 派生 viewer，经引擎
		// ListObjects 展开），集群不可见直接返回空列表。
		viewSet, err := clusterRelationSet(l.ctx, l.svcCtx, "can_view")
		if err != nil {
			l.Logger.Errorf("集群 can_view 集合加载失败 sub=%s: %v", sub, err)
			return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
		}
		if !viewSet[rmqClusterObject(req.ClusterId)] {
			return &types.RocketmqTopicListResp{Items: []types.RocketmqTopicResp{}, Total: 0}, nil
		}
	}

	topics, err := l.svcCtx.TopicRepo.ListByCluster(l.ctx, req.ClusterId)
	if err != nil {
		l.Logger.Errorf("Topic 列表读取失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 数据读取失败")
	}

	items := make([]types.RocketmqTopicResp, 0, len(topics))
	for _, t := range topics {
		items = append(items, types.RocketmqTopicResp{
			Id:          strconv.FormatInt(t.Id, 10),
			Name:        t.Name,
			Env:         t.Env,
			Description: t.Description,
			ClusterId:   strconv.FormatInt(t.ClusterId, 10),
			OrgId:       t.OrgId,
			CreatedBy:   t.CreatedBy,
		})
	}
	return &types.RocketmqTopicListResp{Items: items, Total: len(items)}, nil
}
