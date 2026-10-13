// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 独立服务改造：名单读取经 kit/authz relations 转发集中式 authz。

package logic

import (
	"context"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListTopicGrantsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListTopicGrantsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListTopicGrantsLogic 构造业务规则层实例。
func NewListTopicGrantsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListTopicGrantsLogic {
	return &ListTopicGrantsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListTopicGrants 授予列表（资源级授权，模型 v6；名单可见性判定已上收
// 路由守卫 requireTopicPerm("can_view")——can_view 或管理码全量可见）：
// 读该 topic 的全部 viewer/editor 元组（经 authz relations 接口），解析为
// 目标+关系三元组，按（targetType, targetId, relation）升序。Topic 不存在
// 404；授权服务故障 503 fail-closed。
func (l *ListTopicGrantsLogic) ListTopicGrants(req *types.TopicIdReq) (*types.TopicGrantsResp, error) {
	id, err := parseTopicID(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.TopicRepo.Get(l.ctx, id); err != nil {
		return nil, errors.NotFound("AUTHZ_TOPIC_NOT_FOUND", "Topic 不存在")
	}

	tuples, err := l.svcCtx.Authz.ListRelations(l.ctx, topicObject(id), "viewer")
	if err != nil {
		return nil, topicEngineErr()
	}
	editorTuples, err := l.svcCtx.Authz.ListRelations(l.ctx, topicObject(id), "editor")
	if err != nil {
		return nil, topicEngineErr()
	}
	tuples = append(tuples, editorTuples...)

	items := listGrantsOf(tuples)
	return &types.TopicGrantsResp{Items: items, Total: len(items)}, nil
}
