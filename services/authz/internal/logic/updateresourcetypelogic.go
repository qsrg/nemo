// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	stderrors "errors"
	"strings"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"nemo/kit/errors"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateResourceTypeLogic 承载更新资源类型名单管理码的业务规则。
type UpdateResourceTypeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateResourceTypeLogic 构造业务规则层实例。
func NewUpdateResourceTypeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateResourceTypeLogic {
	return &UpdateResourceTypeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateResourceType 更新资源类型的名单管理码（ManageCode）：
//   - 类型须已注册（404）；管理码为空 = 清空全权码，非空须为合法权限码
//     形态（服务:动作，如 kafka-topics:manage）；
//   - 幂等：manageCode 未变化时也成功（重复配置无副作用）；
//   - 只改 ManageCode，不动 DslBlock/Actions（模型无需重建——管理码是
//     名单策略配置，非引擎模型内容；授权元组判定即时生效）。
//
// 背景（2026-10-11）：收敛 kafka_topic 专用分支后，名单管理码统一由注册表
// ManageCode 声明驱动，本接口提供管理面配置入口（替代手改库），供部署/运维
// 对齐"注册资源类型 → 声明管理码 → 名单管理生效"的接入流程。
func (l *UpdateResourceTypeLogic) UpdateResourceType(req *types.UpdateResourceTypeReq) (*types.ResourceTypeResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.BadRequest("AUTHZ_RESOURCE_TYPE_INVALID", "资源类型名不能为空")
	}
	code := strings.TrimSpace(req.ManageCode)
	if code != "" && !validPermissionCode(code) {
		return nil, errors.BadRequest("AUTHZ_RESOURCE_TYPE_INVALID", "manageCode 非法（须为 服务:动作 码形态，如 kafka-topics:manage）")
	}

	rt, err := l.svcCtx.ResourceTypeRepo.Get(l.ctx, name)
	if err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return nil, errors.NotFound("AUTHZ_RESOURCE_TYPE_NOT_FOUND", "资源类型未注册")
		}
		return nil, errors.Internal("AUTHZ_INTERNAL", "查询资源类型失败")
	}
	rt.ManageCode = code
	rt.UpdateBy = operator // 审计字段（AGENTS.md 规范）：更新人 = 操作人。
	if uerr := l.svcCtx.ResourceTypeRepo.Update(l.ctx, rt); uerr != nil {
		return nil, errors.Internal("AUTHZ_INTERNAL", "更新资源类型失败")
	}
	logx.WithContext(l.ctx).Infof("resource type manage code updated: %s manageCode=%s", name, code)
	return &types.ResourceTypeResp{
		Name:       rt.Name,
		Actions:    engine.ExtractActions(rt.DslBlock),
		ManageCode: rt.ManageCode,
	}, nil
}
