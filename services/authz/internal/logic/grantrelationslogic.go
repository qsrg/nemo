// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层，单测重点；错误一律用 kit/errors 语义构造器返回。

package logic

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// GrantRelationsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type GrantRelationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGrantRelationsLogic 构造业务规则层实例。
func NewGrantRelationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GrantRelationsLogic {
	return &GrantRelationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// grantRelationOrder 是可授基础关系档位的固定渲染顺序（继承阶梯：viewer <
// editor < owner，sharer 为正交共享档）。新档位接入在此登记。
var grantRelationOrder = []string{"viewer", "editor", "owner", "sharer"}

// exportToRelations 把注册类型的 can_* 出口集合反解为可授基础关系档位：
//   - can_view       → viewer（可见档）；
//   - can_edit       → editor（编辑档，隐含查看）；
//   - can_delete / can_manage_grants → owner（管理档：删除、名单管理，二者均
//     收口 owner，与 editor 拆分）；
//   - can_share      → sharer（共享档，与 editor 正交：可共享 ≠ 可编辑）。
//
// 菜单/按钮声明驱动注册（resourcetypesync），业务页不再硬编码关系集合。
func exportToRelations(actions []string) []string {
	has := make(map[string]bool, len(actions))
	for _, a := range actions {
		has[a] = true
	}
	out := make([]string, 0, len(grantRelationOrder))
	if has["can_view"] {
		out = append(out, "viewer")
	}
	if has["can_edit"] {
		out = append(out, "editor")
	}
	if has["can_delete"] || has["can_manage_grants"] {
		out = append(out, "owner")
	}
	if has["can_share"] {
		out = append(out, "sharer")
	}
	return out
}

// buildActionTitles 按菜单声明反查某资源类型下每个 can_* 出口对应的真实
// 按钮名清单（管理员在授权弹窗看到的就是这些名字，而不是 can_edit 这类
// 抽象词）。规则与 resourcetypesync 派生保持一致：
//   - type=menu 且 resourceType 命中：页面入口计入 can_view（"列表可见"）；
//   - type=button 且 relation 非空：按钮名计入该 relation；
//
// 同名按钮去重，顺序跟随 MenuRepo.List 的 (sort,id) 升序。查不到菜单时
// 返回 nil——前端回退到内置抽象词映射，不阻断授权弹窗打开。
func buildActionTitles(menus []repo.Menu, resourceType string) map[string][]string {
	out := map[string][]string{}
	seen := map[string]map[string]bool{}
	add := func(relation, title string) {
		if relation == "" || strings.TrimSpace(title) == "" {
			return
		}
		if seen[relation] == nil {
			seen[relation] = map[string]bool{}
		}
		if seen[relation][title] {
			return
		}
		seen[relation][title] = true
		out[relation] = append(out[relation], title)
	}
	for _, m := range menus {
		hit := false
		for _, rt := range splitResourceTypes(m.ResourceType) {
			if rt == resourceType {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		switch m.Type {
		case "menu":
			add("can_view", m.Title)
		case "button":
			add(m.Relation, m.Title)
		}
	}
	return out
}

// exportToLevels 与 exportToRelations 同序，把每个档位展开为它实际包含的
// can_* 出口列表（继承语义：高档隐含低档操作）。前端据此展示"该档位能干什么"：
//   - viewer：can_view；
//   - editor：can_view + can_edit；
//   - owner：can_view/can_edit（若注册）+ can_delete/can_manage_grants（实际注册项）；
//   - sharer：can_share（正交，不含编辑/删除）。
//
// titles 由 buildActionTitles 提供：每个 can_* 出口对应的真实按钮名清单，
// 用于 ActionDetails；titles 为 nil 时 ActionDetails 留空，前端回退抽象词。
func exportToLevels(actions []string, titles map[string][]string) []types.GrantLevel {
	has := make(map[string]bool, len(actions))
	for _, a := range actions {
		has[a] = true
	}
	with := func(list ...string) []string {
		out := make([]string, 0, len(list))
		for _, a := range list {
			if has[a] {
				out = append(out, a)
			}
		}
		return out
	}
	details := func(list ...string) []types.GrantAction {
		if titles == nil {
			return nil // 菜单反查不可用：不留空壳，前端整体回退抽象词
		}
		out := make([]types.GrantAction, 0, len(list))
		for _, a := range list {
			if !has[a] {
				continue
			}
			out = append(out, types.GrantAction{Relation: a, Titles: titles[a]})
		}
		return out
	}
	relations := exportToRelations(actions)
	levels := make([]types.GrantLevel, 0, len(relations))
	for _, r := range relations {
		switch r {
		case "viewer":
			levels = append(levels, types.GrantLevel{
				Relation: r, Actions: with("can_view"),
				ActionDetails: details("can_view"),
			})
		case "editor":
			levels = append(levels, types.GrantLevel{
				Relation: r, Actions: with("can_view", "can_edit"),
				ActionDetails: details("can_view", "can_edit"),
			})
		case "owner":
			// 管理档 = 该资源全部主操作（查看/修改/删除/配名单），按实际注册展开
			levels = append(levels, types.GrantLevel{
				Relation: r, Actions: with("can_view", "can_edit", "can_delete", "can_manage_grants"),
				ActionDetails: details("can_view", "can_edit", "can_delete", "can_manage_grants"),
			})
		case "sharer":
			levels = append(levels, types.GrantLevel{
				Relation: r, Actions: with("can_share"),
				ActionDetails: details("can_share"),
			})
		}
	}
	return levels
}

// MeGrantRelations 返回某资源类型的可授关系档位（登录即可读）：从注册表
// DslBlock 反解 can_* 出口派生，授权对话框档位数据源。未注册类型 400 fail-closed；
// 注册表存储故障 503 fail-closed（不得误报为"未注册"，与全站口径一致）。
// 同时反查菜单声明，把每个 can_* 出口对应的真实按钮名带回（ActionDetails），
// 让管理员在弹窗里看到"编辑 Topic、删除 Topic"这类实际操作而非抽象词。
func (l *GrantRelationsLogic) MeGrantRelations(req *types.GrantRelationsReq) (*types.GrantRelationsResp, error) {
	rt, err := l.svcCtx.ResourceTypeRepo.Get(l.ctx, req.ResourceType)
	if err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return nil, errors.BadRequest("AUTHZ_TYPE_NOT_REGISTERED", "资源类型未注册")
		}
		l.Logger.Errorf("资源类型注册表读取失败 type=%s: %v", req.ResourceType, err)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "资源类型注册表读取失败")
	}
	actions := engine.ExtractActions(rt.DslBlock)
	// 菜单反查失败不阻断授权弹窗打开——ActionDetails 留空，前端回退抽象词。
	titles := map[string][]string{}
	if menus, merr := l.svcCtx.MenuRepo.List(l.ctx); merr == nil {
		titles = buildActionTitles(menus, req.ResourceType)
	} else {
		logx.WithContext(l.ctx).Errorf("列菜单反查按钮名失败(回退抽象词): %v", merr)
	}
	return &types.GrantRelationsResp{
		Relations: exportToRelations(actions),
		Levels:    exportToLevels(actions, titles),
	}, nil
}
