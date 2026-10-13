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

// MeOrgsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type MeOrgsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMeOrgsLogic 构造业务规则层实例。
func NewMeOrgsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MeOrgsLogic {
	return &MeOrgsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MeOrgs 组织目录（登录即可读，仅 id/名称/父级三字段）：委托管理面
// ListOrgs 的同一套合并逻辑（注册表元数据 + 引擎 org 元组推导），不另设
// 规则。定位：授权 UI 的部门选择器数据源——资源创建/授权名单场景普通
// 用户（非平台超管）也要选部门，管理面 /admin/orgs 的超管门槛对其不可
// 用；本接口只读目录，组织写操作仍全部留在管理面。
func (l *MeOrgsLogic) MeOrgs() (*types.OrgListResp, error) {
	return NewListOrgsLogic(l.ctx, l.svcCtx).ListOrgs()
}
