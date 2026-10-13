// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 独立服务改造：授权名单写经 kit/authz relations 转发集中式 authz
//（authz 侧按对象类型校验名单管理权——kafka_topic 同 topicCanManage）。

package logic

import (
	"context"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// GrantTopicLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type GrantTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGrantTopicLogic 构造业务规则层实例。
func NewGrantTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GrantTopicLogic {
	return &GrantTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GrantTopic 授予查看/编辑权限（资源级授权，模型 v6；名单管理权判定已上收
// 路由守卫 requireTopicManagePerm——owner 或管理码，**不含 editor**）：
// 为目标（user/org）写 viewer 或 editor 元组——viewer 可查看，editor 可查看
// 并编辑；授予部门即覆盖其子树全部成员（v5 聚合语义）。同目标同关系重复
// 授予幂等（authz relations 接口读后写）。审计先行（grant_topic——审计表在
// kafka 侧；授权写经 authz relations 接口时 authz 侧另以 write_relations
// 记录，双写审计保留操作全貌）。
func (l *GrantTopicLogic) GrantTopic(req *types.TopicGrantReq) (*types.TopicGrantResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	relation := strings.TrimSpace(req.Relation)
	if relation != "viewer" && relation != "editor" {
		return nil, errors.BadRequest("AUTHZ_TOPIC_INVALID", "relation 仅支持 viewer 或 editor")
	}
	subject, err := topicGrantSubject(req.TargetType, req.TargetId)
	if err != nil {
		return nil, err
	}

	id, err := parseTopicID(req.Id)
	if err != nil {
		return nil, err
	}

	if _, err := l.svcCtx.TopicRepo.Get(l.ctx, id); err != nil {
		return nil, errors.NotFound("AUTHZ_TOPIC_NOT_FOUND", "Topic 不存在")
	}

	err = writeWithAudit(l.ctx, l.svcCtx, operator, "grant_topic", topicObject(id), req, func() error {
		_, werr := l.svcCtx.Authz.WriteRelations(l.ctx, []authzkit.Tuple{topicGrantTuple(id, subject, relation)})
		return werr
	})
	if err != nil {
		return nil, err
	}

	logx.WithContext(l.ctx).Infof("topic grant: topic=%d subject=%s relation=%s", id, subject, relation)
	return &types.TopicGrantResp{TargetType: req.TargetType, TargetId: req.TargetId, Relation: relation}, nil
}
