// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

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

// RevokeRoleLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type RevokeRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRevokeRoleLogic 构造业务规则层实例。
func NewRevokeRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeRoleLogic {
	return &RevokeRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// RevokeRole 回收平台角色：校验与 AssignRole 一致（必填/白名单/targetType），
// 与授予对称——org 目标删 platform_role:{key}#assignee@org:{uid}#member 元组，
// user 目标删 @user:{uid}（现状不变）。org 目标不做存在性校验：回收是幂等
// 清理动作，元组不存在时 DeleteTuples 读后删跳过，接口幂等不报错（组织已删
// 的残留授予也允许回收）。
// 写路径（审计先行）：CreatePending → 删对应元组 → MarkCommitted；
// 引擎错误 → MarkFailed + 503。
func (l *RevokeRoleLogic) RevokeRole(req *types.AssignRoleReq) (*types.RoleResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	roleKey := strings.TrimSpace(req.RoleKey)
	targetID := trimOrgID(req.UserId)
	if roleKey == "" || targetID == "" {
		return nil, errors.BadRequest("AUTHZ_ROLE_INVALID", "角色 key 与用户 ID 不能为空")
	}
	name := roleName(roleKey)
	if name == "" {
		return nil, errors.BadRequest("AUTHZ_ROLE_UNKNOWN", "角色未定义")
	}
	// targetType 归一化与 AssignRole 对称（缺省 user；非法值 400）。
	targetType := strings.TrimSpace(req.TargetType)
	if targetType == "" {
		targetType = "user"
	}
	if targetType != "user" && targetType != "org" {
		return nil, errors.BadRequest("AUTHZ_ROLE_INVALID", "targetType 仅支持 user 或 org")
	}
	assignee := "user:" + targetID
	if targetType == "org" {
		assignee = "org:" + targetID + "#member"
	}

	target := "platform_role:" + roleKey
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "revoke_role", target, req, func() error {
		return l.svcCtx.Engine.DeleteTuples(l.ctx, []engine.Tuple{
			{User: assignee, Relation: "assignee", Object: target},
		})
	})
	if err != nil {
		return nil, err
	}

	return &types.RoleResp{RoleKey: roleKey, Name: name}, nil
}
