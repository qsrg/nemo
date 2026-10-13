// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"fmt"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CreateMenuLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateMenuLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateMenuLogic 构造业务规则层实例。
func NewCreateMenuLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMenuLogic {
	return &CreateMenuLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateMenu 新建菜单/按钮行（菜单模型简化，docs/menu-simplify-design.md）：
// 行身份 = 自增 id（落库生成），父子引用 = parentId（顶级/服务首页为 0），
// 权限码统一存 permissionCode 一列。校验规则：
//   - title 必填；type ∈ {menu（默认）, button}；
//   - 菜单行：path 必填（导航入口）；permissionCode 可选（空 = 全员可见，
//     非空须为合法权限码）；relation 禁填；可挂父菜单行——三级菜单约束
//     （顶级=1/一级=2/二级=3，沿 parent 链向上校验，父级为二级时拒绝）；
//   - 按钮行：parentId 必填且指向存在的菜单行（按钮不得再挂子节点）；
//     path 可空（不入侧边栏）；页面级按钮（relation 空）permissionCode
//     必填、resourceType 禁填；资源级按钮（relation 非空）permissionCode
//     同样必填（仅作前端动作分派标识——操作列 handler 按码映射，授权不读
//     此码，跟随资源关系三档）、resourceType 必填且 relation 命中引擎
//     出口白名单；
//   - 非空权限码全表唯一（先查后写 409 AUTHZ_MENU_EXISTS；并发窗口由
//     DB UNIQUE 索引兜底，空码 = NULL 不参与唯一约束）；
//   - enabled 固定 true（停用走 PUT）。审计先行（writeWithAudit，
//     action=create_menu；target 用 title——id 落库才生成，审计 pending 先行）。
//
// 落库后同步资源类型派生（资源驱动菜单/资源级按钮 → 类型注册表），失败
// fail-closed（菜单已生效，重试可归一）。
func (l *CreateMenuLogic) CreateMenu(req *types.CreateMenuReq) (*types.MenuResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "title 不能为空")
	}
	// 行类型：menu（默认）| button。按钮行不得作为导航入口。
	menuType := strings.TrimSpace(req.Type)
	if menuType == "" {
		menuType = "menu"
	}
	if menuType != "menu" && menuType != "button" {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "type 非法（须为 menu 或 button）")
	}

	code := strings.TrimSpace(req.PermissionCode)
	// 非空权限码形态校验（可空性按行类型/按钮级别在下方约束）。
	if code != "" && !validPermissionCode(code) {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "permissionCode 非法（须为 服务:动作 码形态，如 kafka-topics:create）")
	}
	// resourceType 可空；非空支持逗号分隔多值（一个页面含多种业务资源：
	// me/menus 对任一声明类型下有任一资源可见即解锁入口），逐项须匹配引擎
	// 资源类型名。落库保持声明原文（拆分解读见 splitResourceTypes）。
	resTypes, rerr := validateResourceTypes(req.ResourceType)
	if rerr != nil {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", rerr.Error())
	}
	resType := strings.Join(resTypes, ",")
	relation := strings.TrimSpace(req.Relation)

	switch menuType {
	case "menu":
		if relation != "" {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "菜单行不能声明资源关系（relation 仅资源级按钮使用）")
		}
		if strings.TrimSpace(req.Path) == "" {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "path 不能为空（菜单行为导航入口）")
		}
		// 三级菜单层级：挂父菜单行时，父级须存在且为菜单行，父链深度 + 1 ≤ 3
		//（父级为二级菜单时拒绝——二级不能再作父级）。
		if req.ParentId != 0 {
			p, perr := l.svcCtx.MenuRepo.Get(l.ctx, req.ParentId)
			if perr != nil {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", fmt.Sprintf("父菜单不存在: %d", req.ParentId))
			}
			if p.Type != "menu" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "父级须为菜单行")
			}
			all, lerr := l.svcCtx.MenuRepo.List(l.ctx)
			if lerr != nil {
				return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "菜单注册表读取失败")
			}
			pDepth, derr := menuChainDepth(all, req.ParentId)
			if derr != nil || pDepth+1 > 3 {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "菜单层级最多三级（顶级/一级/二级，二级菜单不能再作父级）")
			}
		}
	case "button":
		if req.ParentId == 0 {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "按钮行必须挂在菜单下（parentId 必填）")
		}
		p, perr := l.svcCtx.MenuRepo.Get(l.ctx, req.ParentId)
		if perr != nil {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", fmt.Sprintf("父菜单不存在: %d", req.ParentId))
		}
		if p.Type != "menu" {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "父级须为菜单行（按钮不得再挂子节点）")
		}
		if relation != "" {
			// 资源级按钮：授权跟随资源关系三档（码不参与判定）；权限码必填
			// 但仅作前端动作分派标识——操作列 handler 按码映射（如 TopicManage
			// 的 BUTTON_ACTIONS），键名删除后码是按钮行的稳定字符串身份。
			if code == "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "资源级按钮必须声明权限码（前端动作分派标识，如 kafka-topics:freeze）")
			}
			if resType == "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "资源级按钮必须关联资源类型（resourceType 必填）")
			}
			if !menuRelationPattern.MatchString(relation) {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "relation 非法（须为小写字母开头的小写字母/数字/下划线，最长 64）")
			}
			if verr := validateButtonRelation(relation); verr != nil {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", verr.Error())
			}
		} else {
			// 页面级按钮：功能权限码必填（授权界面勾选的实体），与具体资源无关。
			if code == "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "页面级按钮必须声明权限码（permissionCode 必填）")
			}
			if resType != "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "页面级按钮无需资源类型（资源类型仅资源级按钮/资源驱动菜单使用）")
			}
		}
	}

	// 非空权限码全表唯一（先查后写 409 语义；并发窗口由 DB UNIQUE 索引兜底）。
	if code != "" {
		all, lerr := l.svcCtx.MenuRepo.List(l.ctx)
		if lerr != nil {
			return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "菜单注册表读取失败")
		}
		for _, m := range all {
			if m.PermissionCode == code {
				return nil, errors.Conflict("AUTHZ_MENU_EXISTS", fmt.Sprintf("权限码已被菜单 %q 占用: %s", m.Title, code))
			}
		}
	}

	created := repo.Menu{
		ParentID:       req.ParentId,
		Type:           menuType,
		Title:          strings.TrimSpace(req.Title),
		Icon:           strings.TrimSpace(req.Icon),
		Path:           strings.TrimSpace(req.Path),
		Sort:           req.Sort,
		PermissionCode: code,
		ResourceType:   resType,
		Relation:       relation,
		Enabled:        true,
		CreateBy:       operator, // 审计字段（AGENTS.md 规范）：创建人 = 操作人。
	}
	var saved repo.Menu
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_menu", "menu:"+created.Title, req, func() error {
		var werr error
		saved, werr = l.svcCtx.MenuRepo.Create(l.ctx, created)
		return werr
	})
	if err != nil {
		return nil, err
	}

	// 资源类型派生同步：菜单即类型声明（类型由资源级按钮/资源驱动菜单派生），
	// 落库后重算；失败 fail-closed（菜单已生效，重试可归一）。
	if err := SyncResourceTypesFromMenus(l.ctx, l.svcCtx); err != nil {
		return nil, resourceTypeSyncErrTo503(err)
	}
	resp := menuResp(saved)
	return &resp, nil
}
