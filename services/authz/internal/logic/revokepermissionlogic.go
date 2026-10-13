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

// RevokePermissionLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type RevokePermissionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRevokePermissionLogic 构造业务规则层实例。
func NewRevokePermissionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokePermissionLogic {
	return &RevokePermissionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// RevokePermission 回收权限点（与 GrantPermissions 对称）：
//   - 校验：操作人必在；targetType 仅 user/org；targetId / permission 必填且
//     permission 过 permissionCodePattern（400 AUTHZ_PERMISSION_INVALID）；
//     org 目标前置校验组织存在（404 AUTHZ_ORG_NOT_FOUND）；
//   - 写路径（审计先行）：CreatePending → 删
//     permission:{code}#can@user:{id} 或 @org:{id}#member → MarkCommitted；
//     引擎错误 → MarkFailed + 503。元组不存在时 DeleteTuples 读后删幂等跳过
//     （Revoked=0），审计照记（动作语义完整）。
func (l *RevokePermissionLogic) RevokePermission(req *types.RevokePermissionReq) (*types.RevokePermissionResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	targetID := trimOrgID(req.TargetId)
	targetType := strings.TrimSpace(req.TargetType)
	if targetType != "user" && targetType != "org" {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "targetType 仅支持 user 或 org")
	}
	if targetID == "" {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "目标 ID 不能为空")
	}
	code := strings.TrimSpace(req.Permission)
	if !validPermissionCode(code) {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "非法权限码: "+req.Permission)
	}
	if targetType == "org" {
		exists, err := orgExists(l.ctx, l.svcCtx, targetID)
		if err != nil {
			return nil, engineErrTo503(err)
		}
		if !exists {
			return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
		}
	}

	subject, err := grantSubject(targetType, targetID)
	if err != nil {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", err.Error())
	}
	tuple := engine.Tuple{User: subject, Relation: "can", Object: permObject(code)}

	// 回收前先读存在性：Revoked 语义 = 实际删除的元组数（不存在 = 0）。
	existed := true
	if grants, gerr := directGrantCodes(l.ctx, l.svcCtx, subject); gerr == nil {
		existed = false
		for _, c := range grants {
			if c == code {
				existed = true
				break
			}
		}
	} else {
		// 存在性前置读失败按引擎故障处理（fail-closed，不做静默删除）。
		return nil, engineErrTo503(gerr)
	}

	err = writeWithAudit(l.ctx, l.svcCtx, operator, "revoke_permission", grantTarget(targetType, targetID), req, func() error {
		return l.svcCtx.Engine.DeleteTuples(l.ctx, []engine.Tuple{tuple})
	})
	if err != nil {
		return nil, err
	}

	revoked := 0
	if existed {
		revoked = 1
	}
	return &types.RevokePermissionResp{Revoked: revoked}, nil
}
