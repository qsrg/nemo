// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"sort"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListOrgMembersLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListOrgMembersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListOrgMembersLogic 构造业务规则层实例。
func NewListOrgMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListOrgMembersLogic {
	return &ListOrgMembersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListOrgMembers 列出组织直接成员：
//   - 校验：orgId 必填（契约 path 参数仍归一化自校验）；组织必须已存在（404）；
//   - 读路径：ReadTuples 按 object=org:{id} + relation=member 过滤读
//     （引擎内部按 continuation token 翻页取全），从元组 User 提取
//     "user:" 后缀为成员 UserId；只含直接成员——上级组织沿 parent 链的
//     继承成员由授权模型在判定时承担，不在本列表展开；
//   - 引擎读失败 → 503 fail-closed（engineErrTo503）；
//   - 无成员 = 200 空数组（非 null），不作为错误。
func (l *ListOrgMembersLogic) ListOrgMembers(req *types.OrgIdReq) (*types.MemberListResp, error) {
	orgID := trimOrgID(req.Id)
	if orgID == "" {
		return nil, errors.BadRequest("AUTHZ_MEMBER_INVALID", "组织 ID 不能为空")
	}

	exists, err := orgExists(l.ctx, l.svcCtx, orgID)
	if err != nil {
		return nil, engineErrTo503(err)
	}
	if !exists {
		return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
	}

	tuples, err := l.svcCtx.Engine.ReadTuples(l.ctx, "org:"+orgID, "member")
	if err != nil {
		return nil, engineErrTo503(err)
	}

	userIDs := make([]string, 0, len(tuples))
	for _, t := range tuples {
		if !strings.HasPrefix(t.User, "user:") {
			continue
		}
		userIDs = append(userIDs, strings.TrimPrefix(t.User, "user:"))
	}
	// 按 UserId 升序输出，列表顺序确定（引擎返回顺序无保证）。
	sort.Strings(userIDs)

	members := make([]types.MemberResp, 0, len(userIDs))
	for _, uid := range userIDs {
		members = append(members, types.MemberResp{
			OrgId:       orgID,
			UserId:      uid,
			DisplayName: l.svcCtx.UserDisplayName(uid),
		})
	}
	return &types.MemberListResp{Members: members}, nil
}
