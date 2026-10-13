// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	stderrors "errors"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// PermissionOverviewLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type PermissionOverviewLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewPermissionOverviewLogic 构造业务规则层实例。
func NewPermissionOverviewLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PermissionOverviewLogic {
	return &PermissionOverviewLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// PermissionOverview 某用户的有效权限总览（管理面"人员授权"页回显）：
//   - direct：直接授予该用户的 can 元组权限码（user:{userId} 主体）；
//   - viaOrgs：用户直接所在组织（org.member 元组反查）逐组织的 can 授权码
//     （org:{id}#member 主体；沿 parent 链上级组织的授予由引擎判定面原生
//     继承，本总览不展开，避免双份展示）；orgName 取组织注册表，未收录
//     （如 bootstrap 直写的 org:root）回退组织 ID；
//   - roles：过渡期展示——用户被授予的平台角色（ListObjects assignee
//     platform_role，去前缀），去角色化后角色不参与权限派生，仅存量展示；
//   - 引擎读失败 → 503 fail-closed；userId 必填（400 AUTHZ_PERMISSION_INVALID）。
func (l *PermissionOverviewLogic) PermissionOverview(req *types.PermissionOverviewReq) (*types.PermissionOverviewResp, error) {
	userID := trimOrgID(req.UserId)
	if userID == "" {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "userId 不能为空")
	}

	// 直接授予。
	direct, err := directGrantCodes(l.ctx, l.svcCtx, "user:"+userID)
	if err != nil {
		l.Logger.Errorf("读直接授予元组失败 userId=%s: %v", userID, err)
		return nil, engineErrTo503(err)
	}

	// 经部门：用户所在组织 → 逐组织读 can 授权码。
	orgIDs, err := userOrgIDs(l.ctx, l.svcCtx, userID)
	if err != nil {
		l.Logger.Errorf("读组织成员元组失败 userId=%s: %v", userID, err)
		return nil, engineErrTo503(err)
	}
	viaOrgs := make([]types.PermissionViaOrg, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		codes, err := directGrantCodes(l.ctx, l.svcCtx, "org:"+orgID+"#member")
		if err != nil {
			l.Logger.Errorf("读组织授权元组失败 orgId=%s: %v", orgID, err)
			return nil, engineErrTo503(err)
		}
		name := orgID
		if org, gerr := l.svcCtx.Orgs.Get(l.ctx, orgID); gerr == nil && org.Name != "" {
			name = org.Name
		} else if gerr != nil && !stderrors.Is(gerr, repo.ErrNotFound) {
			// 注册表读失败不阻断总览（展示字段），回退组织 ID 并记日志。
			logx.WithContext(l.ctx).Errorf("查组织注册表失败 orgId=%s: %v", orgID, gerr)
		}
		viaOrgs = append(viaOrgs, types.PermissionViaOrg{
			OrgId:   orgID,
			OrgName: name,
			Codes:   codes,
		})
	}

	// 过渡期角色展示（去 platform_role: 前缀）。
	roleObjects, err := l.svcCtx.Engine.ListObjects(l.ctx, "user:"+userID, "assignee", "platform_role")
	if err != nil {
		l.Logger.Errorf("列平台角色失败 userId=%s: %v", userID, err)
		return nil, engineErrTo503(err)
	}
	roles := make([]string, 0, len(roleObjects))
	for _, o := range roleObjects {
		roles = append(roles, trimPrefixStrict(o, "platform_role:"))
	}

	return &types.PermissionOverviewResp{
		Direct:  direct,
		ViaOrgs: viaOrgs,
		Roles:   roles,
	}, nil
}

// trimPrefixStrict 去指定前缀（无前缀原样返回）。
func trimPrefixStrict(s, prefix string) string {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
