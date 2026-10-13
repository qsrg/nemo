// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"net/http"
	"time"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListMenusLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListMenusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListMenusLogic 构造业务规则层实例。
func NewListMenusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMenusLogic {
	return &ListMenusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListMenus 菜单注册表全量列表（含 disabled，按 (sort, id) 升序）。
// 读失败 503 fail-closed。
func (l *ListMenusLogic) ListMenus() (*types.MenuListResp, error) {
	menus, err := l.svcCtx.MenuRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("菜单注册表读取失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "菜单注册表读取失败")
	}
	items := make([]types.MenuResp, 0, len(menus))
	for _, m := range menus {
		items = append(items, menuResp(m))
	}
	return &types.MenuListResp{Items: items, Total: len(items)}, nil
}

// menuResp 注册行 → 管理面响应。
func menuResp(m repo.Menu) types.MenuResp {
	return types.MenuResp{
		Id:             m.ID,
		ParentId:       m.ParentID,
		Type:           m.Type,
		Title:          m.Title,
		Icon:           m.Icon,
		Path:           m.Path,
		Sort:           m.Sort,
		PermissionCode: m.PermissionCode,
		ResourceType:   m.ResourceType,
		Relation:       m.Relation,
		Enabled:        m.Enabled,
		CreatedAt:      m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      m.UpdatedAt.Format(time.RFC3339),
	}
}
