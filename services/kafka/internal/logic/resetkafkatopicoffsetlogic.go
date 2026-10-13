// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 动作码演示（按钮级功能权限）：重置位点 = kafka-topics:reset-offset 码
//（路由守卫）+ 资源级 can_edit（本层判定）——功能码管"能不能做这个
// 动作"，资源级管"对哪个对象"，两层 AND。demo 无 Kafka 集群，效果为
// 记录操作并返回成功（真实实现替换为 AdminClient 调用）。

package logic

import (
	"context"
	"strings"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ResetKafkaTopicOffsetLogic 承载「重置位点」的业务规则。
type ResetKafkaTopicOffsetLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewResetKafkaTopicOffsetLogic 构造业务规则层实例。
func NewResetKafkaTopicOffsetLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ResetKafkaTopicOffsetLogic {
	return &ResetKafkaTopicOffsetLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ResetKafkaTopicOffset 重置某 Topic 的消费者位点（演示语义）：
//   - 功能码 kafka-topics:reset-offset 已在路由守卫判定（无码 403）；
//   - 本层叠加资源级 can_edit：对该 topic 无编辑权 → 403（动作权 × 对象权）；
//   - 引擎判定故障 → 503 fail-closed。
func (l *ResetKafkaTopicOffsetLogic) ResetKafkaTopicOffset(req *types.TopicIdReq) (*types.ResetOffsetResp, error) {
	id, err := parseTopicID(strings.TrimSpace(req.Id))
	if err != nil {
		return nil, err
	}
	// 资源级把关：can_edit（owner/editor/manage 码，引擎真源 + ABAC 叠加）。
	allowed, cerr := l.svcCtx.Authz.Check(l.ctx, topicObject(id), "can_edit")
	if cerr != nil {
		l.Logger.Errorf("can_edit 判定失败 topic=%d: %v", id, cerr)
		return nil, topicEngineErr()
	}
	if !allowed {
		return nil, errors.Forbidden("AUTHZ_DENIED", "无操作权限")
	}
	// demo 无集群：记录操作即可（真实实现替换为 Kafka AdminClient 重置）。
	op, _ := operatorFrom(l.ctx)
	l.Logger.Infof("重置位点（demo）topic=%d by=%s", id, op)
	return &types.ResetOffsetResp{Id: req.Id, Reset: true}, nil
}
