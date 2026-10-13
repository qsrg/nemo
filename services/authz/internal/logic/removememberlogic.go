// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// RemoveMemberLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type RemoveMemberLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRemoveMemberLogic 构造业务规则层实例。
func NewRemoveMemberLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveMemberLogic {
	return &RemoveMemberLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// RemoveMember 移除组织成员：
//   - 校验：操作人必在；orgId/userId 必填；组织必须已存在（404）；
//   - 写路径（审计先行）：CreatePending → 删 org:{id}#member@user:{uid} 元组
//     → MarkCommitted；引擎错误 → MarkFailed + 503。
//     元组不存在时 DeleteTuples 读后删跳过，接口幂等不报错。
func (l *RemoveMemberLogic) RemoveMember(req *types.MemberIdReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	orgID := trimOrgID(req.OrgId)
	userID := trimOrgID(req.UserId)
	if orgID == "" || userID == "" {
		return errors.BadRequest("AUTHZ_MEMBER_INVALID", "组织 ID 与用户 ID 不能为空")
	}

	exists, err := orgExists(l.ctx, l.svcCtx, orgID)
	if err != nil {
		return engineErrTo503(err)
	}
	if !exists {
		return errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
	}

	return writeWithAudit(l.ctx, l.svcCtx, operator, "remove_member", "org:"+orgID, req, func() error {
		return l.svcCtx.Engine.DeleteTuples(l.ctx, []engine.Tuple{
			{User: "user:" + userID, Relation: "member", Object: "org:" + orgID},
		})
	})
}
