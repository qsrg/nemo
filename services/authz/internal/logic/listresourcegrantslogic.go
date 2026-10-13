// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器返回。

package logic

import (
	"context"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListResourceGrantsLogic 承载通用资源授权列表的业务规则。
type ListResourceGrantsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListResourceGrantsLogic 构造业务规则层实例。
func NewListResourceGrantsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListResourceGrantsLogic {
	return &ListResourceGrantsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// listViewableBy 校验调用者对资源的"查看名单"权限并返回对象 id
// （2026-10-11 收敛 kafka_topic 专用分支后统一注册策略）：
//   - 已注册类型：注册配置驱动的通用名单策略（grantpolicy.go）——名单
//     管理码 or can_view（仅当类型注册了 can_view 出口），零改码接入；
//   - 未注册类型（order 等基础模型类型）：403 fail-closed，不泄漏名单。
//
// 返回解析后的对象 id（topic 场景为 int64 字符串），供元组读取使用。
func (l *ListResourceGrantsLogic) listViewableBy(sub, object string) (string, error) {
	objType, objID, ok := strings.Cut(object, ":")
	if !ok || objType == "" || objID == "" {
		return "", errors.BadRequest("AUTHZ_RELATION_INVALID", "object 须为 type:id 形式")
	}
	policy, err := grantPolicyFor(l.ctx, l.svcCtx, objType)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("读资源类型名单策略失败 %s: %v", objType, err)
		return "", topicEngineErr()
	}
	if policy == nil {
		return "", errors.Forbidden("AUTHZ_DENIED", "该资源类型暂未开放名单查看")
	}
	allowed, err := grantsViewable(l.ctx, policy, l.svcCtx, sub, object)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("名单查看权判定失败 object=%s: %v", object, err)
		return "", topicEngineErr()
	}
	if !allowed {
		return "", errors.Forbidden("AUTHZ_DENIED", "无操作权限")
	}
	return objID, nil
}

// ListResourceGrants 通用资源授权列表：按 object（+可选 relation）读取元组。
// 查看校验见 listViewableBy（fail-closed，不泄漏名单给无权用户）。
func (l *ListResourceGrantsLogic) ListResourceGrants(req *types.ListResourceGrantsReq) (*types.ResourceGrantsResp, error) {
	sub, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	if _, err := l.listViewableBy(sub, req.Object); err != nil {
		return nil, err
	}

	tuples, err := l.svcCtx.Engine.ReadTuples(l.ctx, req.Object, strings.TrimSpace(req.Relation))
	if err != nil {
		return nil, errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
	}
	items := make([]types.ResourceGrantItem, 0, len(tuples))
	for _, t := range tuples {
		items = append(items, types.ResourceGrantItem{
			User:     t.User,
			Relation: t.Relation,
			Object:   t.Object,
		})
	}
	return &types.ResourceGrantsResp{Items: items, Total: len(items)}, nil
}
