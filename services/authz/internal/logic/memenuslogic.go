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

	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// MeMenusLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type MeMenusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMeMenusLogic 构造业务规则层实例。
func NewMeMenusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MeMenusLogic {
	return &MeMenusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MeMenus 当前用户可见菜单（菜单模型简化，docs/menu-simplify-design.md）：
//   - 身份缺失 → 401 fail-closed；
//   - 权限来源与 /me/permissions 同源（去角色化改造）：授权引擎实时查询
//     （effectivePermCodes：type permission 的 can 元组，ListObjects 求值），
//     引擎失败 → 503 fail-closed；
//   - 过滤语义：permissionCode 空 = 全员可见（不过滤）；非空 = permGranted
//     （有效码集合判定，与前端 hasPermission 同口径）——仅 enabled 且可见的
//     行下发（fail-closed：不可见即不下发）；
//   - 资源驱动通道（resource_type + ListObjects can_view）正交保留；
//   - 注册表读失败 → 503 fail-closed；
//   - 结果按 (sort, id) 升序（repo List 契约），前端按 path 首段归组到服务、
//     按 parentId 组装树。无可见菜单 → 200 空数组（非 null）。
func (l *MeMenusLogic) MeMenus() (*types.MeMenusResp, error) {
	sub, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	perms, err := effectivePermCodes(l.ctx, l.svcCtx.Engine, sub)
	if err != nil {
		l.Logger.Errorf("权限引擎实时查询失败 sub=%s: %v", sub, err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用，菜单不可用")
	}

	menus, err := l.svcCtx.MenuRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("菜单注册表读取失败: %v", err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "菜单注册表读取失败")
	}

	items := make([]types.MeMenuItem, 0, len(menus))
	// 方案 B（资源驱动菜单）：resource_type 非空（可逗号分隔多值）的行，用户
	// 对任一声明类型下任一资源有 can_view 级关系（viewer/editor/owner）即解锁
	// 入口，无需再授 read 码。同类型多菜单共享一次 ListObjects 查询（cache map）。
	resVisible := make(map[string]bool, 2)
	resourceVisible := func(resType string) (bool, error) {
		if v, ok := resVisible[resType]; ok {
			return v, nil
		}
		objs, err := l.svcCtx.Engine.ListObjects(l.ctx, "user:"+sub, "can_view", resType)
		if err != nil {
			return false, err
		}
		v := len(objs) > 0
		resVisible[resType] = v
		return v, nil
	}
	// 第一遍：按码/资源驱动判定各菜单行可见性（按钮行与 disabled 行不入集）。
	visible := make(map[int64]bool, len(menus))
	for _, m := range menus {
		// 按钮行不入侧边栏：仅授权目录实体（me/menus 只返回可导航菜单）。
		if m.Type == "button" || !m.Enabled {
			continue
		}
		// 空码 = 全员可见（不过滤）；非空码 = 准入门槛（与前端 hasPermission
		// 同口径），未持有则看资源驱动通道。
		ok := m.PermissionCode == "" || permGranted(m.PermissionCode, perms)
		if !ok {
			// 资源驱动菜单：任一类型下有可见资源即解锁（多类型 = 页面含多种
			// 业务资源，入口对其中任一资源的可见用户开放）。
			for _, rt := range splitResourceTypes(m.ResourceType) {
				v, verr := resourceVisible(rt)
				if verr != nil {
					l.Logger.Errorf("资源级菜单可见性查询失败 sub=%s type=%s: %v", sub, rt, verr)
					return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用，菜单不可用")
				}
				if v {
					ok = true
					break
				}
			}
		}
		if ok {
			visible[m.ID] = true
		}
	}
	// 第二遍：按父子关系递归组装下发行（树序遍历，父先子后）。
	// 下发规则（菜单是稳定导航结构，页面内数据由资源授权过滤）：
	//   - 根行（parentId 为 0）：独立可见（码命中/空码/资源驱动）才下发；
	//   - 子行：父行"因准入可见"（父持服务级码/资源驱动解锁，guarded=true）
	//     → 跟随下发（页面入口继承服务级入口，不随资源授权增删变动——如
	//     zhangsan 持 rocketmq:read 时，即使取消 rmq_cluster 资源授权，
	//     集群管理/RMQ Topic 管理入口仍可见，避免"菜单随授权消失"的导航
	//     抖动）；父行仅因空码可见（guarded=false，如 NuJob 根）→ 子行按
	//     独立可见性判定（有码未持不下发，保持页面级准入语义）；
	//   - 子行父行未下发但独立可见 → 仍下发（前端孤儿提升保持"有权限即
	//     有入口"，如 zhangsan 持 kafka-topics:read 而无 kafka:read）。
	children := make(map[int64][]repo.Menu, len(menus))
	for _, m := range menus {
		children[m.ParentID] = append(children[m.ParentID], m)
	}
	var walk func(pid int64, parentEmitted bool, parentGuarded bool)
	walk = func(pid int64, parentEmitted bool, parentGuarded bool) {
		for _, m := range children[pid] {
			if m.Type == "button" || !m.Enabled {
				continue
			}
			if !visible[m.ID] && !(parentEmitted && parentGuarded) {
				continue // 父未准入下发且自身不可见 → 不下发
			}
			items = append(items, types.MeMenuItem{
				Id:             m.ID,
				ParentId:       m.ParentID,
				Title:          m.Title,
				Icon:           m.Icon,
				Path:           m.Path,
				Sort:           m.Sort,
				PermissionCode: m.PermissionCode,
			})
			// 准入性向下传递：父有码/资源驱动 → 子仍随准入可见下发。
			walk(m.ID, true, parentGuarded || m.PermissionCode != "" || m.ResourceType != "")
		}
	}
	walk(0, false, false)
	// 后处理：移除「仅剩根行」的纯锚点行（parentId=0 且标题≈path 首段且无任何
	// 下发子项）。根行本身不挂页面（多为 redirect 锚点，如 NuJob /nujob 在目标
	// 微应用无对应路由），子菜单全被过滤时仅剩根行 = 无页面可访问，服务对该
	// 用户不可见（fail-closed，避免前端渲染点不进的假入口）。标题≠服务名的
	// 根行（如 kafka「集群概览」）是真实默认页，即使无子项也保留。
	childIds := make(map[int64]bool, len(items))
	for _, it := range items {
		childIds[it.ParentId] = true
	}
	filtered := items[:0]
	for _, it := range items {
		if it.ParentId == 0 {
			seg := strings.TrimPrefix(it.Path, "/")
			seg = strings.Split(seg, "/")[0]
			if seg != "" && strings.EqualFold(it.Title, seg) && !childIds[it.Id] {
				continue
			}
		}
		filtered = append(filtered, it)
	}
	items = filtered
	return &types.MeMenusResp{Items: items}, nil
}
