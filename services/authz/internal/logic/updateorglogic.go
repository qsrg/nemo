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
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateOrgLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type UpdateOrgLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateOrgLogic 构造业务规则层实例。
func NewUpdateOrgLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateOrgLogic {
	return &UpdateOrgLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateOrg 更新组织（name/parentId 均可选更新，空 = 不变）：
//   - 校验：操作人必在；Id 必填且组织必须已存在（404）；
//     parentId 非空时父组织必须存在（404）、且不得成环（409 AUTHZ_ORG_CYCLE）；
//   - 写路径（审计先行）：CreatePending → 按引擎现值删除全部旧 parent 元组
//     （注册表可能落后，见下）+ 写新 parent 元组 + 注册表更新 →
//     MarkCommitted；引擎错误 → MarkFailed + 503。
//
// 环拒绝发生在审计 pending 之前，不产生审计条目，引擎状态不被破坏。
func (l *UpdateOrgLogic) UpdateOrg(req *types.UpdateOrgReq) (*types.OrgResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	orgID := trimOrgID(req.Id)
	if orgID == "" {
		return nil, errors.BadRequest("AUTHZ_ORG_INVALID", "组织 ID 不能为空")
	}

	// 组织必须已存在（注册表或引擎元组）。
	exists, err := orgExists(l.ctx, l.svcCtx, orgID)
	if err != nil {
		return nil, engineErrTo503(err)
	}
	if !exists {
		return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "组织不存在")
	}

	// 现值：注册表元数据为准（引擎可见未注册的组织取不到现名，用 ID 兜底）。
	info, gerr := l.svcCtx.Orgs.Get(l.ctx, orgID)
	if gerr != nil {
		info = repo.OrgInfo{ID: orgID, Name: orgID, ParentID: ""}
	}

	newName := info.Name
	if n := strings.TrimSpace(req.Name); n != "" {
		newName = n
	}
	newParent := info.ParentID
	if p := trimOrgID(req.ParentId); p != "" {
		// 换父：父组织必须存在且不得成环（沿 parent 链上溯见自身即环）。
		pexists, err := orgExists(l.ctx, l.svcCtx, p)
		if err != nil {
			return nil, engineErrTo503(err)
		}
		if !pexists {
			return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "父组织不存在")
		}
		cycled, err := ensureNoCycle(l.ctx, l.svcCtx.Engine, orgID, p)
		if err != nil {
			return nil, engineErrTo503(err)
		}
		if cycled {
			return nil, errors.Conflict("AUTHZ_ORG_CYCLE", "组织环")
		}
		newParent = p
	}

	// 换父删除依据取引擎现值：模型 parent: [org] 为集合语义，注册表可能落后
	// 或丢失（内存实现重启即空）而引擎元组仍在，以注册表为"旧父"会漏删、
	// 制造多父状态。删前读出该 org 全部 parent 元组。
	oldParents, err := l.svcCtx.Engine.ReadTuples(l.ctx, "org:"+orgID, "parent")
	if err != nil {
		return nil, engineErrTo503(err)
	}
	oldParentIDs := make(map[string]bool, len(oldParents))
	for _, t := range oldParents {
		oldParentIDs[strings.TrimPrefix(t.User, "org:")] = true
	}

	// 审计先行写路径：旧 parent/child 互逆元组（引擎现值全量）逐条删除、新对
	// 写入；新父与唯一既有父相同则跳过元组改写。child（org:子#child@org:父）
	// 与 parent 互逆，判定面 member from child 聚合用（模型 v5）。
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "update_org", "org:"+orgID, req, func() error {
		soleOld := len(oldParentIDs) == 1 && oldParentIDs[newParent]
		if !soleOld {
			victims := make([]engine.Tuple, 0, 2*len(oldParentIDs))
			for id := range oldParentIDs {
				victims = append(victims,
					engine.Tuple{User: "org:" + id, Relation: "parent", Object: "org:" + orgID},
					engine.Tuple{User: "org:" + orgID, Relation: "child", Object: "org:" + id},
				)
			}
			if err := l.svcCtx.Engine.DeleteTuples(l.ctx, victims); err != nil {
				return err
			}
			if newParent != "" {
				if err := l.svcCtx.Engine.WriteTuples(l.ctx, []engine.Tuple{
					{User: "org:" + newParent, Relation: "parent", Object: "org:" + orgID},
					{User: "org:" + orgID, Relation: "child", Object: "org:" + newParent},
				}); err != nil {
					return err
				}
			}
		}
		if _, uerr := l.svcCtx.Orgs.Upsert(l.ctx, repo.OrgInfo{
			ID:       orgID,
			Name:     newName,
			ParentID: newParent,
			CreateBy: operator,
			UpdateBy: operator,
		}); uerr != nil {
			return uerr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &types.OrgResp{Id: orgID, Name: newName, ParentId: newParent}, nil
}
