// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 独立服务改造：判定经 kit/authz 转发集中式 authz。

package logic

import (
	"context"
	"strconv"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateKafkaTopicLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type UpdateKafkaTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateKafkaTopicLogic 构造业务规则层实例。
func NewUpdateKafkaTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateKafkaTopicLogic {
	return &UpdateKafkaTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateKafkaTopic 更新 Topic（编辑权判定已上收路由守卫，业务代码只关注业务）。
//
// 执行顺序：
//  1. 编辑权判定在路由层完成（routes.go 挂 requireTopicPerm("can_edit")：
//     kafka_topic:can_edit 主判定——模型 v6 = owner or editor，authz 侧内置
//     ABAC 叠加，终端用户 JWT 经 Identity 中间件透传——无权 403 / 判定服务
//     故障 503 fail-closed / 路径 ID 非法 400，logic 不再内联）；
//  2. 取操作人 operatorFrom（本地 ctx 取 JWT claims.sub，无远程调用；缺失
//     401 fail-closed）；入口 parseTopicID 把字符串 Id 转 int64；
//  3. 资源存在性（TopicRepo.Get，未命中 404——判定已前置，不再泄露
//     "无权与不存在"之外的存在性信息）；
//  4. 审计先行写闭包（writeWithAudit，action=update_topic）：**只改库，不写
//     任何授权元组**——编辑业务属性不改变权限拓扑（owner/viewer/editor 元组
//     原样保留），authz 在编辑路径是纯只读的（与 create/delete 需写/清元组
//     的差异点）。name 不可改：业务唯一键（Create 按 name 重名预检），
//     引擎对象身份是 kafka_topic:{id} 与 name 无关，但改名会破坏前端与
//     审计快照按 name 的既有引用。
//
// 主键改造：API 入参 Id 为字符串，入口 parseTopicID 转 int64；resp.Id 回转字符串。
// resp 徽标：能走到写库即已通过 can_edit 判定，CanEdit/CanDelete 恒 true
// （owner 仅按 sub == CreatedBy 作创建人展示，不参与判定）。
func (l *UpdateKafkaTopicLogic) UpdateKafkaTopic(req *types.UpdateKafkaTopicReq) (*types.KafkaTopicResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseTopicID(req.Id)
	if err != nil {
		return nil, err
	}

	prev, err := l.svcCtx.TopicRepo.Get(l.ctx, id)
	if err != nil {
		return nil, errors.NotFound("AUTHZ_TOPIC_NOT_FOUND", "Topic 不存在")
	}

	updated := repo.Topic{
		ID:          prev.ID,
		Name:        prev.Name,
		Env:         req.Env,
		Description: req.Description,
		CreatedBy:   prev.CreatedBy,
		UpdateBy:    sub, // 审计字段（AGENTS.md 规范）：更新人 = 当前操作人。
	}
	err = writeWithAudit(l.ctx, l.svcCtx, sub, "update_topic", topicObject(prev.ID), req, func() error {
		_, werr := l.svcCtx.TopicRepo.Update(l.ctx, updated)
		return werr
	})
	if err != nil {
		return nil, err
	}

	saved, err := l.svcCtx.TopicRepo.Get(l.ctx, prev.ID)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("回读更新 Topic 失败 %d: %v", prev.ID, err)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 落库后回读失败")
	}
	return &types.KafkaTopicResp{
		Id:          strconv.FormatInt(saved.ID, 10),
		Name:        saved.Name,
		Env:         saved.Env,
		Description: saved.Description,
		CreatedBy:   saved.CreatedBy,
		// 菜单级授权：能通过 can_edit 判定即可编辑/删除（owner 仅作创建人展示）。
		CanEdit:   true,
		CanDelete: true,
		Owner:     sub == saved.CreatedBy,
	}, nil
}
