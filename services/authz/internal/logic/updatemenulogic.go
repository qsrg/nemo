// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器返回。

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

// UpdateMenuLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type UpdateMenuLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateMenuLogic 构造业务规则层实例。
func NewUpdateMenuLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateMenuLogic {
	return &UpdateMenuLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateMenu 更新菜单/按钮行（菜单模型简化，docs/menu-simplify-design.md）：
// id（路径参数）定位行，未命中 404；其余字段全量覆盖（PUT 语义，未传按零值
// 覆盖——前端编辑对话框应回填完整表单）。type/parentId 是行身份锚点不可改
// （按钮归属与层级创建后固定，避免移动引发深度/挂载重校验）。
// 权限码规则与 create 同构：菜单行可选（非空须合法）、按钮行必填（页面级
// = 功能码、资源级 = 动作分派标识，均不改变授权判定）；非空码全表唯一
// （排除自身，重复 409）。
// 孤儿授权防护：停用或改权限码后，不再被矩阵管理的码（read + 派生 manage）
// 其授予元组在同一审计单元内一并清理——存量授权不残留为矩阵中不可见、
// 不可撤销的孤儿（仍被其他启用菜单共享的码不误清）。
func (l *UpdateMenuLogic) UpdateMenu(req *types.UpdateMenuReq) (*types.MenuResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	prev, err := l.svcCtx.MenuRepo.Get(l.ctx, req.Id)
	if err != nil {
		return nil, errors.NotFound("AUTHZ_MENU_NOT_FOUND", fmt.Sprintf("菜单不存在: %d", req.Id))
	}
	prevType := prev.Type
	if prevType == "" {
		prevType = "menu"
	}
	if req.Type != "" && req.Type != prev.Type {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "type 不可修改（按钮/菜单身份创建后固定）")
	}

	// resourceType 可空；非空支持逗号分隔多值（PUT 全量覆盖：未传按空值
	// 清空，回退为纯菜单语义），逐项须匹配引擎资源类型名。
	resTypes, rerr := validateResourceTypes(req.ResourceType)
	if rerr != nil {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", rerr.Error())
	}
	resType := strings.Join(resTypes, ",")
	code := strings.TrimSpace(req.PermissionCode)
	relation := strings.TrimSpace(req.Relation)
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "标题不能为空")
	}

	// 非空权限码形态校验（可空性按行类型/按钮级别在下方约束）。
	if code != "" && !validPermissionCode(code) {
		return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "permissionCode 非法（须为 服务:动作 码形态，如 kafka-topics:create）")
	}

	// 权限码/关系/路径语义按行类型约束（按钮行允许 path 为空——不入侧边栏）。
	switch prevType {
	case "menu":
		if relation != "" {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "菜单行不能声明资源关系（relation 仅资源级按钮使用）")
		}
		if strings.TrimSpace(req.Path) == "" {
			return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "path 不能为空（菜单行为导航入口）")
		}
	case "button":
		if relation != "" {
			// 资源级按钮：授权跟随资源关系三档（码不参与判定）；权限码必填
			// 但仅作前端动作分派标识（与 create 同构）。
			if code == "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "资源级按钮必须声明权限码（前端动作分派标识，permissionCode 必填）")
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
			if code == "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "页面级按钮必须声明权限码（permissionCode 必填）")
			}
			if resType != "" {
				return nil, errors.BadRequest("AUTHZ_MENU_INVALID", "页面级按钮无需资源类型（资源类型仅资源级按钮/资源驱动菜单使用）")
			}
		}
	}

	// 非空权限码全表唯一（排除自身；先查后写 409 语义，并发窗口由 DB
	// UNIQUE 索引兜底）。
	if code != "" {
		all, lerr := l.svcCtx.MenuRepo.List(l.ctx)
		if lerr != nil {
			return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "菜单注册表读取失败")
		}
		for _, m := range all {
			if m.ID != prev.ID && m.PermissionCode == code {
				return nil, errors.Conflict("AUTHZ_MENU_EXISTS", fmt.Sprintf("权限码已被菜单 %q 占用: %s", m.Title, code))
			}
		}
	}

	updated := repo.Menu{
		ID:             prev.ID,
		ParentID:       prev.ParentID, // 行身份锚点：不可改。
		Type:           prev.Type,
		Title:          strings.TrimSpace(req.Title),
		Icon:           req.Icon,
		Path:           req.Path,
		Sort:           req.Sort,
		PermissionCode: code,
		ResourceType:   resType,
		Relation:       relation,
		Enabled:        req.Enabled,
		UpdateBy:       operator, // 审计字段（AGENTS.md 规范）：更新人 = 操作人。
	}
	// 清理范围 = 变更前后码全集（read/manage 派生码）；keep = 变更后仍启用
	// 生效的码集。
	candidates := menuPermCodes(prev)
	var keep map[string]bool
	if updated.Enabled {
		keep = make(map[string]bool, 4)
		for _, c := range menuPermCodes(updated) {
			keep[c] = true
			candidates = append(candidates, c)
		}
	}
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "update_menu", fmt.Sprintf("menu:%d", prev.ID), req, func() error {
		saved, werr := l.svcCtx.MenuRepo.Update(l.ctx, updated)
		if werr != nil {
			return werr
		}
		updated = saved // 回填存储层刷新的时间戳（响应用）。
		if n, rerr := revokeOrphanedMenuCodes(l.ctx, l.svcCtx, prev.ID, keep, candidates); rerr != nil {
			return rerr
		} else if n > 0 {
			logx.WithContext(l.ctx).Infof("菜单停用/改码清理孤儿授权 menu=%d 清理 %d 条元组", prev.ID, n)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 资源类型派生同步：更新可能改变类型声明（relation/resourceType 变化），
	// 落库后重算；失败 fail-closed（菜单已生效，重试可归一）。
	if err := SyncResourceTypesFromMenus(l.ctx, l.svcCtx); err != nil {
		return nil, resourceTypeSyncErrTo503(err)
	}
	resp := menuResp(updated)
	return &resp, nil
}
