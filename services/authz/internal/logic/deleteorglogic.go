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
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteOrgLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteOrgLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteOrgLogic 构造业务规则层实例。
func NewDeleteOrgLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteOrgLogic {
	return &DeleteOrgLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteOrg 删除组织：不存在返回 404；写路径（审计先行）先读出该组织的全部
// 引擎元组（object 或 user 任一侧命中 org:{id}，即 parent/member 及其对子组织
// 的 parent 引用），逐条幂等删除，再移除注册表元数据，最后 MarkCommitted。
// 引擎错误 → MarkFailed + 503。
func (l *DeleteOrgLogic) DeleteOrg(req *types.OrgIdReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	orgID := trimOrgID(req.Id)
	if orgID == "" {
		return errors.BadRequest("AUTHZ_ORG_INVALID", "组织 ID 不能为空")
	}

	exists, err := orgExists(l.ctx, l.svcCtx, orgID)
	if err != nil {
		return engineErrTo503(err)
	}
	if !exists {
		return errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
	}

	// 预读该组织的全部元组（删前快照，两侧命中：作为 object 的关系、
	// 作为 user 的 parent 引用）。
	all, err := l.svcCtx.Engine.ReadTuples(l.ctx, "", "")
	if err != nil {
		return engineErrTo503(err)
	}
	victim := make([]engine.Tuple, 0)
	for _, t := range all {
		if t.Object == "org:"+orgID || t.User == "org:"+orgID {
			victim = append(victim, t)
		}
	}

	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_org", "org:"+orgID, req, func() error {
		if err := l.svcCtx.Engine.DeleteTuples(l.ctx, victim); err != nil {
			return err
		}
		// 注册表未命中视为已清理（幂等），不阻断删除。
		if derr := l.svcCtx.Orgs.Delete(l.ctx, orgID); derr != nil && !stderrors.Is(derr, repo.ErrNotFound) {
			return derr
		}
		return nil
	})
}
