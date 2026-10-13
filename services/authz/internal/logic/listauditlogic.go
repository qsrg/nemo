// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"strconv"
	"time"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"nemo/kit/errors"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListAuditLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListAuditLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListAuditLogic 构造业务规则层实例。
func NewListAuditLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAuditLogic {
	return &ListAuditLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListAudit 分页查询审计条目（管理面，GET /v1/admin/audit）：
//   - UserId 非空按操作人过滤（映射 AuditFilter.Operator）；
//   - 分页规范化由 repo 层保证（Page<1→1、PageSize<1→20），
//     排序按（CreatedAt,ID）倒序（新→旧）；
//   - 空结果返回 [] 非 null（前端约定，与其他列表接口一致）；
//   - 存储故障 → 500 AUTHZ_INTERNAL fail-closed（与 ListRules 的 repo
//     故障语义一致；403/503 双语义约束针对 kit/authz 判定链路，不适用管理面）。
func (l *ListAuditLogic) ListAudit(req *types.ListAuditReq) (resp *types.AuditListResp, err error) {
	if req == nil {
		req = &types.ListAuditReq{}
	}
	entries, total, err := l.svcCtx.AuditRepo.Query(l.ctx, repo.AuditFilter{
		Operator:      req.UserId,
		ExcludeSystem: req.ExcludeSystem,
		Page:          req.Page,
		PageSize:      req.PageSize,
	})
	if err != nil {
		logx.WithContext(l.ctx).Errorf("查询审计失败: %v", err)
		return nil, errors.Internal("AUTHZ_INTERNAL", "查询审计失败")
	}
	items := make([]types.AuditResp, 0, len(entries))
	for _, e := range entries {
		items = append(items, types.AuditResp{
			Id:        strconv.FormatInt(e.ID, 10),
			Operator:  e.Operator,
			Action:    e.Action,
			Target:    e.Target,
			Snapshot:  e.Snapshot,
			Status:    string(e.Status),
			CreatedAt: e.CreatedAt.Format(time.RFC3339),
		})
	}
	return &types.AuditListResp{Items: items, Total: total}, nil
}
