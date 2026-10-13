// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// GetTopicConsumers：行级只读动作——查看某 Topic 的消费者组，可见权判定已
// 上收路由守卫（routes.go 挂 requireTopicPerm("can_view")，管理码回退），
// logic 只关注业务；不新增关系。

package logic

import (
	"context"

	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// GetTopicConsumersLogic 承载查看消费者接口的业务规则。
type GetTopicConsumersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGetTopicConsumersLogic 构造业务规则层实例。
func NewGetTopicConsumersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTopicConsumersLogic {
	return &GetTopicConsumersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetTopicConsumers 返回目标 Topic 的消费者组列表：
// 生产环境由 Kafka AdminClient 实时查询（DescribeConsumerGroups 等）；
// demo 环境无 Kafka 集群，返回空列表（接口契约先行，数据源后续接入）。
// 可见权判定在路由层（requireTopicPerm can_view，含管理码回退），本方法
// 只做 ID 解析（非法 400，与守卫的 400 契约同口径）与数据返回。
func (l *GetTopicConsumersLogic) GetTopicConsumers(req *types.TopicIdReq) (*types.GetTopicConsumersResp, error) {
	if _, err := parseTopicID(req.Id); err != nil {
		return nil, err
	}
	// TODO(prod)：接入 Kafka AdminClient（DescribeConsumerGroups/ListConsumerGroupOffsets）
	// 实时查询消费者组；demo 无集群时返回空列表。
	return &types.GetTopicConsumersResp{TopicId: req.Id, Items: []types.TopicConsumerGroupResp{}}, nil
}
