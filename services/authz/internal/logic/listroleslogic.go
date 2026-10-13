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

	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListRolesLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListRolesLogic 构造业务规则层实例。
func NewListRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRolesLogic {
	return &ListRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListRoles 平台角色列表：返回内置白名单全部角色（key + 展示名），按 key
// 升序稳定输出。只读接口，不产生审计。
func (l *ListRolesLogic) ListRoles() (*types.RoleListResp, error) {
	items := make([]types.RoleResp, 0, len(builtinRoles))
	for key, name := range builtinRoles {
		items = append(items, types.RoleResp{RoleKey: key, Name: name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RoleKey < items[j].RoleKey })
	return &types.RoleListResp{Items: items, Total: len(items)}, nil
}
