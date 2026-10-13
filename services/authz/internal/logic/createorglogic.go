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

// CreateOrgLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateOrgLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateOrgLogic 构造业务规则层实例。
func NewCreateOrgLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOrgLogic {
	return &CreateOrgLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateOrg 创建组织：
//   - 校验：操作人（JWT sub）必在；name 必填（全 optional 契约下由本层自校验）；
//     parentId 非空时父组织必须已存在（注册表或引擎元组）；
//   - 防环：沿 parent 元组向上遍历见自身即 Conflict（新建组织理论上无子孙，
//     此处与 UpdateOrg 共用同一防线，语义见 ensureNoCycle）；
//   - 写路径（审计先行）：CreatePending → 写 parent 元组（根组织 parent 留空，
//     不写元组）+ 注册表落库 → MarkCommitted；引擎错误 → MarkFailed + 503。
func (l *CreateOrgLogic) CreateOrg(req *types.CreateOrgReq) (*types.OrgResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.BadRequest("AUTHZ_ORG_INVALID", "组织名称不能为空")
	}
	parentID := trimOrgID(req.ParentId)

	// 父组织存在性：仅在指定 parent 时校验（根组织 parent 留空）。
	if parentID != "" {
		exists, err := orgExists(l.ctx, l.svcCtx, parentID)
		if err != nil {
			return nil, engineErrTo503(err)
		}
		if !exists {
			return nil, errors.NotFound("AUTHZ_ORG_NOT_FOUND", "父组织不存在")
		}
	}

	// 生成组织 ID 并做防环预检（新建组织无既有元组，防线与 UpdateOrg 统一）。
	orgID, err := newResourceID("org")
	if err != nil {
		return nil, errors.Internal("AUTHZ_INTERNAL", "生成组织 ID 失败")
	}
	cycled, err := ensureNoCycle(l.ctx, l.svcCtx.Engine, orgID, parentID)
	if err != nil {
		return nil, engineErrTo503(err)
	}
	if cycled {
		return nil, errors.Conflict("AUTHZ_ORG_CYCLE", "组织环")
	}

	// 审计先行写路径：parent/child 互逆元组（如有）+ 注册表元数据。
	// parent（org:父#parent@org:子）供防环向上遍历；child（org:子#child@org:父）
	// 供判定面 member from child 聚合子树成员（模型 v5 继承方向）。
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_org", "org:"+orgID, req, func() error {
		if parentID != "" {
			if werr := l.svcCtx.Engine.WriteTuples(l.ctx, []engine.Tuple{
				{User: "org:" + parentID, Relation: "parent", Object: "org:" + orgID},
				{User: "org:" + orgID, Relation: "child", Object: "org:" + parentID},
			}); werr != nil {
				return werr
			}
		}
		if _, uerr := l.svcCtx.Orgs.Upsert(l.ctx, repo.OrgInfo{
			ID:       orgID,
			Name:     name,
			ParentID: parentID,
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

	return &types.OrgResp{Id: orgID, Name: name, ParentId: parentID}, nil
}
