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
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// GrantPermissionsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type GrantPermissionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGrantPermissionsLogic 构造业务规则层实例。
func NewGrantPermissionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GrantPermissionsLogic {
	return &GrantPermissionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GrantPermissions 批量授予权限点（去角色化改造）：
//   - 校验：操作人（JWT sub）必在；targetType 仅 user/org（400
//     AUTHZ_PERMISSION_INVALID）；targetId 必填；permissions 非空且逐码过
//     permissionCodePattern（`服务:动作` 或 `*:动作` 通配）；org 目标前置校验
//     组织存在（404 AUTHZ_ORG_NOT_FOUND，orgExists 复用既有判定模式）；
//   - 写路径（审计先行）：CreatePending → 逐码写
//     permission:{code}#can@user:{id}（user 目标）或
//     permission:{code}#can@org:{id}#member（org 目标，成员含 parent 链继承）
//     → MarkCommitted；引擎错误 → MarkFailed + 503。重复授予由 WriteTuples
//     读后写幂等保证。参数校验类错误全部先于审计写路径完成。
func (l *GrantPermissionsLogic) GrantPermissions(req *types.GrantPermissionsReq) (*types.GrantPermissionsResp, error) {
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
	if len(req.Permissions) == 0 {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "权限码列表不能为空")
	}
	codes := make([]string, 0, len(req.Permissions))
	seen := make(map[string]bool, len(req.Permissions))
	for _, raw := range req.Permissions {
		code := strings.TrimSpace(raw)
		if !validPermissionCode(code) {
			return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "非法权限码: "+raw)
		}
		if !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
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
	tuples := grantTuples(subject, codes)

	err = writeWithAudit(l.ctx, l.svcCtx, operator, "grant_permission", grantTarget(targetType, targetID), req, func() error {
		return l.svcCtx.Engine.WriteTuples(l.ctx, tuples)
	})
	if err != nil {
		return nil, err
	}

	return &types.GrantPermissionsResp{Granted: len(tuples)}, nil
}
