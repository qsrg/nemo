// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListMenuRelationsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListMenuRelationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListMenuRelationsLogic 构造业务规则层实例。
func NewListMenuRelationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMenuRelationsLogic {
	return &ListMenuRelationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListMenuRelations 返回资源级按钮 relation 的引擎出口白名单（有序）：
// 菜单管理页「资源关系」下拉选项的数据源。清单与 modelgen canExits 同源
// （代码常量，随版本发布演进），本接口只读透出、不做运行时配置——
// 新增语义须同步扩展引擎模型出口 + 白名单 + 重放模型，三步一起发布。
func (l *ListMenuRelationsLogic) ListMenuRelations() (*types.MenuRelationsResp, error) {
	return &types.MenuRelationsResp{Items: MenuRelationOptions()}, nil
}
