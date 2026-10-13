// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器返回。

package logic

import (
	"context"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteResourceGrantLogic 承载通用资源授权撤销的业务规则。
type DeleteResourceGrantLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteResourceGrantLogic 构造业务规则层实例。
func NewDeleteResourceGrantLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteResourceGrantLogic {
	return &DeleteResourceGrantLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// grantManageableBy 校验调用者对资源的"名单管理"权限（写/撤销共用，
// 2026-10-11 收敛 kafka_topic 专用分支后统一注册策略）：
//   - 已注册类型：注册配置驱动的通用名单策略（grantpolicy.go）——
//     名单管理码 or owner（不含 editor，编辑属性不放大为名单管理权）
//     + ABAC 叠加；新资源类型接入零改码；
//   - 未注册类型（order 等基础模型类型）：403 fail-closed。
func (l *DeleteResourceGrantLogic) grantManageableBy(operator, object string) error {
	objType, _, ok := strings.Cut(object, ":")
	if !ok || objType == "" {
		return errors.BadRequest("AUTHZ_RELATION_INVALID", "object 须为 type:id 形式")
	}
	policy, err := grantPolicyFor(l.ctx, l.svcCtx, objType)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("读资源类型名单策略失败 %s: %v", objType, err)
		return topicEngineErr()
	}
	if policy == nil {
		return errors.Forbidden("AUTHZ_DENIED", "该资源类型暂未开放名单管理")
	}
	if ok, err := grantsManageable(l.ctx, policy, l.svcCtx, operator, object); err != nil {
		logx.WithContext(l.ctx).Errorf("名单管理权判定失败 object=%s: %v", object, err)
		return topicEngineErr()
	} else if !ok {
		return topicDenied()
	}
	return nil
}

// DeleteResourceGrant 通用资源授权撤销：按 object/user/relation 精确定位并
// 删除单条元组（幂等：不存在同成功，与 WriteTuples 对称）。管理权校验见
// grantManageableBy；审计先行（delete_relation）。
func (l *DeleteResourceGrantLogic) DeleteResourceGrant(req *types.DeleteResourceGrantReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	object := strings.TrimSpace(req.Object)
	user := strings.TrimSpace(req.User)
	relation := strings.TrimSpace(req.Relation)
	if object == "" || user == "" || relation == "" {
		return errors.BadRequest("AUTHZ_RELATION_INVALID", "object/user/relation 不能为空")
	}
	if !writableRelations[relation] {
		return errors.Forbidden("AUTHZ_RELATION_FORBIDDEN", "不允许写该关系")
	}
	if err := l.grantManageableBy(operator, object); err != nil {
		return err
	}

	snapshot := map[string]any{"object": object, "user": user, "relation": relation}
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "delete_relation", object, snapshot, func() error {
		return l.svcCtx.Engine.DeleteTuples(l.ctx, []engine.Tuple{{
			User:     user,
			Relation: relation,
			Object:   object,
		}})
	})
	if err != nil {
		return err
	}

	logx.WithContext(l.ctx).Infof("resource grant delete: object=%s user=%s relation=%s", object, user, relation)
	return nil
}
