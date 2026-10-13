// grantpolicy.go 通用资源授权名单的类型级策略（注册配置驱动，零改码接入）。
//
// 通用 relations 接口（/v1/authz/relations 的读/写/撤销）对资源类型的校验
// 不再逐类型写代码：注册资源类型时声明的配置决定一切——
//   - ManageCode（名单管理码，可空）：持有者对该类型**任意**资源拥有名单
//     查看/写/撤销权（如 kafka-topics:manage 之于 topic 的"管理员全权"）；
//   - Actions（can_* 出口勾选）：勾选 can_view ⇒ can_view 持有者可查看名单；
//     owner（模板基础关系，恒存在）可管理名单。
//
// 统一口径（2026-10-11 收敛 kafka_topic 专用分支后仅此一套）：管理 =
// owner or 管理码——**不含 editor**：可编辑属性不放大为名单管理权，防
// editor 自授/改授 owner 等提权越权（最小权限）。查看 = can_view or 管理码。
// ABAC 叠加同资源判定面：管理路径放行后过属性策略（deny 命中即拒绝，
// 见 abac.go）。未注册类型不进入本策略（写入在 WriteRelations 的
// writableObject 已拒；order 为基础模型业务类型，走原语义——业务服务
// 批量写，不做名单 UI）。
package logic

import (
	"context"
	stderrors "errors"
	"strings"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// grantPolicy 是一个已注册资源类型的名单策略视图。
type grantPolicy struct {
	// name 资源类型名（ABAC 叠加取资源类型段）。
	name string
	// manageCode 名单管理码（可空 = 不设全权码）。
	manageCode string
	// createCode 创建码（可空）：创建路径空对象首写 owner/org 的放行依据
	//（writerelationslogic 创建引导分支）。
	createCode string
	// actions 已注册的 can_* 出口集（决定 can_view 查看路径是否可用）。
	actions map[string]bool
}

// grantPolicyFor 读注册表构造某资源类型的名单策略；未注册返回 nil。
func grantPolicyFor(ctx context.Context, sc *svc.ServiceContext, objType string) (*grantPolicy, error) {
	rt, err := sc.ResourceTypeRepo.Get(ctx, objType)
	if err != nil {
		if stderrors.Is(err, repo.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	actions := make(map[string]bool)
	for _, a := range engine.ExtractActions(rt.DslBlock) {
		actions[a] = true
	}
	return &grantPolicy{name: rt.Name, manageCode: rt.ManageCode, createCode: rt.CreateCode, actions: actions}, nil
}

// objectID 从 "type:id" 对象串取 id 段（ABAC 叠加取对象标识用）；
// 格式非法返回空串（abacBlocked 对空对象 id 视为无属性命中，不拦截）。
func objectID(object string) string {
	_, id, _ := strings.Cut(object, ":")
	return id
}

// grantsManageable 通用名单管理权（写/撤销共用，owner-or-管理码口径）：
// manageCode 持有者 or 对象 owner。
// **不含 editor**——编辑属性不放大为名单管理权，防 editor 自授/改授
// owner 提权。放行后叠加 ABAC（deny 命中即拒绝，与资源判定面同语义，
// 对齐原 kafka_topic 专用分支 topicCanManage 的叠加行为）。
// 无权返回 (false, nil)；引擎判定故障按错误透传（调用方转 503 fail-closed）。
func grantsManageable(ctx context.Context, policy *grantPolicy, sc *svc.ServiceContext, sub, object string) (bool, error) {
	if policy == nil {
		return false, nil
	}
	if policy.manageCode != "" {
		ok, err := menuManageGranted(ctx, sc.Engine, sub, policy.manageCode)
		if err != nil {
			return false, err
		}
		if ok {
			return !abacBlocked(ctx, sc, policy.name, objectID(object), sub), nil
		}
	}
	ok, err := sc.Engine.Check(ctx, "user:"+sub, "owner", object)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return !abacBlocked(ctx, sc, policy.name, objectID(object), sub), nil
}

// grantsViewable 通用名单查看权（对齐 topic 的 can_view-or-管理码口径）：
// manageCode 持有者 or 对象 can_view（仅当类型注册了 can_view 出口——
// 未勾选的类型机密性更高，只有管理码可看名单）。
func grantsViewable(ctx context.Context, policy *grantPolicy, sc *svc.ServiceContext, sub, object string) (bool, error) {
	if policy == nil {
		return false, nil
	}
	if policy.manageCode != "" {
		ok, err := menuManageGranted(ctx, sc.Engine, sub, policy.manageCode)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	if !policy.actions["can_view"] {
		return false, nil
	}
	ok, err := sc.Engine.Check(ctx, "user:"+sub, "can_view", object)
	if err != nil {
		return false, err
	}
	return ok, nil
}
