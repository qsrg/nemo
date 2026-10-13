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

// AddMemberLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type AddMemberLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewAddMemberLogic 构造业务规则层实例。
func NewAddMemberLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddMemberLogic {
	return &AddMemberLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// AddMember 添加组织成员：
//   - 校验：操作人（JWT sub）必在；orgId/userId 必填（全 optional 契约下由
//     本层自校验）；组织必须已存在（404）；
//   - 写路径（审计先行）：CreatePending → 写 org:{id}#member@user:{uid} 元组
//     → MarkCommitted；引擎错误 → MarkFailed + 503。
//     member 语义（上级沿 parent 链继承）由授权模型承担，此处只写直接关系。
//     重复添加由 WriteTuples 读后写幂等保证。
func (l *AddMemberLogic) AddMember(req *types.MemberReq) (*types.MemberResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	orgID := trimOrgID(req.OrgId)
	userID := trimOrgID(req.UserId)
	if orgID == "" || userID == "" {
		return nil, errors.BadRequest("AUTHZ_MEMBER_INVALID", "组织 ID 与用户 ID 不能为空")
	}

	exists, err := orgExists(l.ctx, l.svcCtx, orgID)
	if err != nil {
		return nil, engineErrTo503(err)
	}
	if !exists {
		return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
	}

	target := "org:" + orgID
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "add_member", target, req, func() error {
		return l.svcCtx.Engine.WriteTuples(l.ctx, []engine.Tuple{
			{User: "user:" + userID, Relation: "member", Object: target},
		})
	})
	if err != nil {
		return nil, err
	}

	return &types.MemberResp{OrgId: orgID, UserId: userID}, nil
}
