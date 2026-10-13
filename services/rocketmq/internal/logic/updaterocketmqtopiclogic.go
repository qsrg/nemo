// updaterocketmqtopiclogic.go 实现编辑 Topic：路径 id 解析 + 存在性校验 +
// 字段覆盖。权限判定在路由守卫（rmq_cluster:can_view——集群可见即可操作，
// 或管理码回退；2026-10-12 集群级授权模型）。
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

// UpdateRocketmqTopicLogic 承载编辑接口业务规则。
type UpdateRocketmqTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateRocketmqTopicLogic 构造编辑业务规则实例。
func NewUpdateRocketmqTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateRocketmqTopicLogic {
	return &UpdateRocketmqTopicLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// UpdateRocketmqTopic 编辑 Topic：id 非法 400；未命中 404；成功返回覆盖后记录。
func (l *UpdateRocketmqTopicLogic) UpdateRocketmqTopic(req *types.UpdateRocketmqTopicReq) (*types.RocketmqTopicResp, error) {
	id, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New(http.StatusBadRequest, "AUTHZ_OBJECT_INVALID", "Topic ID 非法")
	}
	old, err := l.svcCtx.TopicRepo.Get(l.ctx, id)
	if err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return nil, errors.New(http.StatusNotFound, "AUTHZ_NOT_FOUND", "Topic 不存在")
		}
		l.Logger.Errorf("Topic 查询失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 读取失败")
	}
	updated, err := l.svcCtx.TopicRepo.Update(l.ctx, types.RocketmqTopic{
		Id:          id,
		Name:        old.Name,
		Env:         req.Env,
		Description: req.Description,
		ClusterId:   old.ClusterId,
		OrgId:       old.OrgId,
		CreatedBy:   old.CreatedBy,
	})
	if err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return nil, errors.New(http.StatusNotFound, "AUTHZ_NOT_FOUND", "Topic 不存在")
		}
		l.Logger.Errorf("Topic 更新失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 更新失败")
	}
	return &types.RocketmqTopicResp{
		Id:          strconv.FormatInt(updated.Id, 10),
		Name:        updated.Name,
		Env:         updated.Env,
		Description: updated.Description,
		ClusterId:   strconv.FormatInt(updated.ClusterId, 10),
		OrgId:       updated.OrgId,
		CreatedBy:   updated.CreatedBy,
	}, nil
}
