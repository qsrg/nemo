// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 独立服务改造：授权名单删经 kit/authz relations 转发集中式 authz。

package logic

import (
	"context"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// RevokeTopicGrantLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type RevokeTopicGrantLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRevokeTopicGrantLogic 构造业务规则层实例。
func NewRevokeTopicGrantLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeTopicGrantLogic {
	return &RevokeTopicGrantLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// RevokeTopicGrant 撤销授予（资源级授权，模型 v6；名单管理权判定已上收
// 路由守卫 requireTopicManagePerm——owner 或管理码，**不含 editor**）：删
// 目标的 viewer/editor 元组（幂等：不存在同样成功——撤销对称语义）。Topic
// 不存在 404；审计先行（revoke_topic）。
func (l *RevokeTopicGrantLogic) RevokeTopicGrant(req *types.TopicGrantIdReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	subject, err := topicGrantSubject(req.TargetType, req.TargetId)
	if err != nil {
		return err
	}

	id, err := parseTopicID(req.Id)
	if err != nil {
		return err
	}

	if _, err := l.svcCtx.TopicRepo.Get(l.ctx, id); err != nil {
		return errors.NotFound("AUTHZ_TOPIC_NOT_FOUND", "Topic 不存在")
	}

	return writeWithAudit(l.ctx, l.svcCtx, operator, "revoke_topic", topicObject(id), req, func() error {
		// 读 viewer/editor 名单，命中目标则逐条经 relations 接口删除。
		for _, rel := range []string{"viewer", "editor"} {
			tuples, rerr := l.svcCtx.Authz.ListRelations(l.ctx, topicObject(id), rel)
			if rerr != nil {
				return rerr
			}
			for _, t := range tuples {
				if t.User == subject {
					if derr := l.svcCtx.Authz.DeleteRelation(l.ctx, t.Object, t.User, t.Relation); derr != nil {
						return derr
					}
				}
			}
		}
		return nil // 幂等：无授予可撤，语义成功
	})
}
