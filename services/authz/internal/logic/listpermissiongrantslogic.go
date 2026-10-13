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

// ListPermissionGrantsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListPermissionGrantsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListPermissionGrantsLogic 构造业务规则层实例。
func NewListPermissionGrantsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPermissionGrantsLogic {
	return &ListPermissionGrantsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListPermissionGrants 查询主体现有的权限点授予（授权向导回显）：
//   - 校验：targetType 仅 user/org、targetId 必填（400 AUTHZ_PERMISSION_INVALID）；
//   - 读路径：ReadTuples 全量 can 元组 → 按主体（user:{id} / org:{id}#member）
//     过滤 → 去前缀后升序去重的权限码列表（只含直接授予，不含部门继承展开
//     ——继承总览走 /permissions/overview）。引擎读失败 → 503 fail-closed；
//     无授予 → 200 空数组（非 null）。
func (l *ListPermissionGrantsLogic) ListPermissionGrants(req *types.ListPermissionGrantsReq) (*types.PermissionGrantListResp, error) {
	targetID := trimOrgID(req.TargetId)
	targetType := strings.TrimSpace(req.TargetType)
	if targetType != "user" && targetType != "org" {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "targetType 仅支持 user 或 org")
	}
	if targetID == "" {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "目标 ID 不能为空")
	}

	subject, err := grantSubject(targetType, targetID)
	if err != nil {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", err.Error())
	}
	codes, err := directGrantCodes(l.ctx, l.svcCtx, subject)
	if err != nil {
		l.Logger.Errorf("读权限授予元组失败 subject=%s: %v", subject, err)
		return nil, engineErrTo503(err)
	}

	return &types.PermissionGrantListResp{Codes: codes}, nil
}
