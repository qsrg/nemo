// deleterocketmqtopiclogic.go 实现删除 Topic：路径 id 解析 + 存在性校验 +
// 删除业务行 + 清引擎元组。权限判定在路由守卫（rmq_topic:can_delete 或
// 管理码）。
//
// 元组清理经集中式 authz 接口（DeleteRelation 带重试）逐条删除——与创建
// 双写对称；**严禁绕过控制台/接口直接改引擎库**。清理失败仅记日志：
// 引擎侧残留孤儿元组由 reconcile 兜底（对象已不存在，不影响新对象）。
package logic

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"

	"nemo/kit/errors"
	"nemo/services/rocketmq/internal/repo"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteRocketmqTopicLogic 承载删除接口业务规则。
type DeleteRocketmqTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteRocketmqTopicLogic 构造删除业务规则实例。
func NewDeleteRocketmqTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteRocketmqTopicLogic {
	return &DeleteRocketmqTopicLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// DeleteRocketmqTopic 删除 Topic：id 非法 400；未命中 404；成功后清理引擎
// 侧该对象的全部元组（owner/org/editor/viewer 等）。
func (l *DeleteRocketmqTopicLogic) DeleteRocketmqTopic(req *types.DeleteRocketmqTopicReq) error {
	id, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil || id <= 0 {
		return errors.New(http.StatusBadRequest, "AUTHZ_OBJECT_INVALID", "Topic ID 非法")
	}
	if err := l.svcCtx.TopicRepo.Delete(l.ctx, id); err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return errors.New(http.StatusNotFound, "AUTHZ_NOT_FOUND", "Topic 不存在")
		}
		l.Logger.Errorf("Topic 删除失败: %v", err)
		return errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 删除失败")
	}

	// 清理引擎元组（失败不阻断返回：孤儿元组由 reconcile 兜底）。
	obj := rmqTopicObject(id)
	tuples, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, obj, "")
	if rerr != nil {
		l.Logger.Errorf("Topic 元组列取失败（孤儿残留）object=%s: %v", obj, rerr)
		return nil
	}
	for _, t := range tuples {
		if derr := deleteRelationWithRetry(l.ctx, l.svcCtx.Authz, t.Object, t.User, t.Relation); derr != nil {
			l.Logger.Errorf("Topic 元组清理失败 object=%s rel=%s user=%s: %v", obj, t.Relation, t.User, derr)
		}
	}
	return nil
}
