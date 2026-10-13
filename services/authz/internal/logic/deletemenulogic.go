// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器返回。

package logic

import (
	"context"
	"fmt"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteMenuLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteMenuLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteMenuLogic 构造业务规则层实例。
func NewDeleteMenuLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMenuLogic {
	return &DeleteMenuLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteMenu 删除菜单/按钮行（菜单模型简化，docs/menu-simplify-design.md）：
// id（路径参数）定位行，未命中 404；审计先行（writeWithAudit，
// action=delete_menu）。
// 树级联：删除菜单行时递归删除其下全部后代（子菜单行 + 按钮行，parent_id
// 逐级指向），先删最深再删父。孤儿授权防护：所有被删行的权限码（菜单行
// read + 派生 manage、页面级按钮的功能码）进入候选集，其授予元组在同一
// 审计单元内清理——仍被其他启用菜单共享的码不误清。
func (l *DeleteMenuLogic) DeleteMenu(req *types.MenuIdReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}

	menu, err := l.svcCtx.MenuRepo.Get(l.ctx, req.Id)
	if err != nil {
		return errors.NotFound("AUTHZ_MENU_NOT_FOUND", fmt.Sprintf("菜单不存在: %d", req.Id))
	}

	writeErr := writeWithAudit(l.ctx, l.svcCtx, operator, "delete_menu", fmt.Sprintf("menu:%d", req.Id), req, func() error {
		// 收集全部后代（parent_id 链）：孤儿码候选集 = 所有被删行关联的
		// 码（menuPermCodes：菜单行 read/manage + 按钮/页面级按钮功能码；
		// 空码项在清理函数内跳过）。
		all, lerr := l.svcCtx.MenuRepo.List(l.ctx)
		if lerr != nil {
			return lerr
		}
		childrenByParent := make(map[int64][]repo.Menu, 8)
		for _, m := range all {
			if m.ParentID != 0 {
				childrenByParent[m.ParentID] = append(childrenByParent[m.ParentID], m)
			}
		}
		codes := menuPermCodes(menu)
		var remove func(id int64) error
		remove = func(id int64) error {
			for _, child := range childrenByParent[id] {
				if derr := remove(child.ID); derr != nil {
					return derr
				}
				codes = append(codes, menuPermCodes(child)...)
				if derr := l.svcCtx.MenuRepo.Delete(l.ctx, child.ID); derr != nil {
					return derr
				}
			}
			return nil
		}
		if menu.Type == "" || menu.Type == "menu" {
			if rerr := remove(req.Id); rerr != nil {
				return rerr
			}
		}
		if derr := l.svcCtx.MenuRepo.Delete(l.ctx, req.Id); derr != nil {
			return derr
		}
		if n, rerr := revokeOrphanedMenuCodes(l.ctx, l.svcCtx, req.Id, nil, codes); rerr != nil {
			return rerr
		} else if n > 0 {
			logx.WithContext(l.ctx).Infof("删除菜单清理孤儿授权 menu=%d 清理 %d 条元组", req.Id, n)
		}
		return nil
	})
	if writeErr != nil {
		return writeErr
	}
	// 资源类型派生同步：删除可能移除某类型的全部引用（类型不再派生）；
	// 落库后重算，失败 fail-closed（菜单已生效，重试可归一）。
	if serr := SyncResourceTypesFromMenus(l.ctx, l.svcCtx); serr != nil {
		return resourceTypeSyncErrTo503(serr)
	}
	return nil
}
