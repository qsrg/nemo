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

// AssignRoleLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type AssignRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewAssignRoleLogic 构造业务规则层实例。
func NewAssignRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignRoleLogic {
	return &AssignRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// AssignRole 授予平台角色：
//   - 校验：操作人（JWT sub）必在；roleKey/userId 必填（400 AUTHZ_ROLE_INVALID）；
//     RoleKey 白名单（内置 operator/readonly，未注册 → 400 AUTHZ_ROLE_UNKNOWN；
//     super_admin 已移除——超管鉴权统一走 permission:*.* 权限码）；targetType 仅 user/org（缺省 user 向后兼容）；
//     org 目标时组织必须已存在（404 AUTHZ_ORG_NOT_FOUND，orgExists 复用
//     listorgmemberslogic 的判定模式——注册表或引擎元组任一命中即存在）；
//   - 写路径（审计先行）：CreatePending → 写
//     platform_role:{key}#assignee@user:{uid}（user 目标，现状不变）或
//     platform_role:{key}#assignee@org:{uid}#member（org 目标，模型 v2 起
//     组织成员沿 org.member 的 parent 传递获得角色）→ MarkCommitted；
//     引擎错误 → MarkFailed + 503。重复授予由 WriteTuples 读后写幂等保证。
func (l *AssignRoleLogic) AssignRole(req *types.AssignRoleReq) (*types.RoleResp, error) {
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
	// targetType 归一化：缺省 user（向后兼容既有调用方）；org 目标前置校验
	// 存在性（404），参数校验类错误必须先于审计写路径完成。
	targetType := strings.TrimSpace(req.TargetType)
	if targetType == "" {
		targetType = "user"
	}
	if targetType != "user" && targetType != "org" {
		return nil, errors.BadRequest("AUTHZ_ROLE_INVALID", "targetType 仅支持 user 或 org")
	}
	assignee := "user:" + targetID
	if targetType == "org" {
		exists, err := orgExists(l.ctx, l.svcCtx, targetID)
		if err != nil {
			return nil, engineErrTo503(err)
		}
		if !exists {
			return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
		}
		// org 目标写 userset 元组：org:{uid}#member 作为 assignee 的用户集，
		// 成员判定（含 parent 链继承）交由授权模型承担。
		assignee = "org:" + targetID + "#member"
	}

	target := "platform_role:" + roleKey
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "assign_role", target, req, func() error {
		return l.svcCtx.Engine.WriteTuples(l.ctx, []engine.Tuple{
			{User: assignee, Relation: "assignee", Object: target},
		})
	})
	if err != nil {
		return nil, err
	}

	return &types.RoleResp{RoleKey: roleKey, Name: name}, nil
}
