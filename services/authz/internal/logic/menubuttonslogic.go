// MenuButtonsLogic 承载按钮声明查询：按父菜单路由（path）返回其下 enabled
// 的按钮行（type=button）。按钮声明是授权模型的 UI 表现层数据——前端按
// 声明渲染操作列（按钮文案/出口/归属类型），显隐由调用方按当前用户在该
// 资源上的关系（can_* 出口）判定；因此登录即可读，不要求管理员身份。
// 与 admin/menus（管理面全量）不同：本接口只下发按钮定义，不含授权配置。
package logic

import (
	"context"
	"net/http"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// MenuButtonsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type MenuButtonsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMenuButtonsLogic 构造业务规则层实例。
func NewMenuButtonsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MenuButtonsLogic {
	return &MenuButtonsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MenuButtons 查询父菜单下的按钮声明（enabled 才下发，禁用 = 前端不渲染）。
// 父菜单定位锚 = 路由 path（键名删除后微应用已知自身页面路由，
// /{service}/{page} 即父菜单完整路由）；多行同 path 时合并下发其按钮
// （path 非唯一索引，声明侧自洽即可）。读失败 503 fail-closed；
// 非按钮行或父行不匹配一律跳过。
func (l *MenuButtonsLogic) MenuButtons(req *types.MenuButtonsReq) (*types.MenuButtonsResp, error) {
	// 登录校验（按钮定义本身非敏感，授权判定在调用方按关系进行）。
	if _, _, err := identityFrom(l.ctx); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "缺少父菜单路由（path 参数，如 /kafka/topics）")
	}

	menus, err := l.svcCtx.MenuRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("菜单注册表读取失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "菜单注册表读取失败")
	}

	// 先定位父菜单行集合（按 path 匹配菜单行），再收集其下按钮行。
	parentIDs := make(map[int64]bool, 2)
	for _, m := range menus {
		if m.Type != "button" && m.Path == path {
			parentIDs[m.ID] = true
		}
	}
	items := make([]types.MenuButtonItem, 0, 4)
	for _, m := range menus {
		if m.Type != "button" || !m.Enabled || !parentIDs[m.ParentID] {
			continue
		}
		items = append(items, types.MenuButtonItem{
			PermissionCode: m.PermissionCode,
			Title:          m.Title,
			ResourceType:   m.ResourceType,
			Relation:       m.Relation,
		})
	}
	return &types.MenuButtonsResp{Items: items}, nil
}
