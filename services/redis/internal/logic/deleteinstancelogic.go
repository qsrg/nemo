// deleteinstancelogic.go 承载删除实例的业务规则：先删库、后清元组（审计
// 先行），删除权判定已上收路由守卫（can_delete + 管理码回退），业务代码
// 只关注业务。
package logic

import (
	"context"

	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteInstanceLogic 承载删除实例接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteInstanceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteInstanceLogic 构造业务规则层实例。
func NewDeleteInstanceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteInstanceLogic {
	return &DeleteInstanceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteInstance 删除实例（删除权判定已上收路由守卫 requireInstancePerm
// ("can_delete")：can_delete 主判定（owner）+ redis:manage 回退）。
//
// 执行顺序：
//  1. 判权在路由层完成，无权 403 / 判定服务故障 503 fail-closed；
//  2. 资源不存在 → 直接返回成功（**幂等**：重复删除/前端重试不报错；守卫
//     前置后仅管理码持有者可达本分支——普通 owner 的 owner 元组随删除被清，
//     重试已删资源将在守卫处 403，与 PUT 的"判定先于存在性"同口径，同时
//     关闭"403=存在 / 200=不存在"的存在性枚举通道）；
//  3. 审计先行（writeWithAudit，action=delete_instance），闭包内**先删库、
//     后清元组**：
//     - 先删库 → 资源立即消失，后续判定即不可见。避免"元组先删完、删库
//     失败 → 资源还在但 owner 已删、谁都管不了（锁死）"；
//     - 后清元组 → 库已删，清理失败只残留垃圾（不影响"资源已删除"的业务
//     结果），逐条带退避重试，仍失败仅记录日志、不阻断，残留由运维清理。
func (l *DeleteInstanceLogic) DeleteInstance(req *types.DeleteInstanceReq) error {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	id, err := parseInstanceID(req.Id)
	if err != nil {
		return err
	}

	// 幂等：资源已不存在（上次删除已生效）→ 成功返回，不抛 404。
	if _, gerr := l.svcCtx.InstanceRepo.Get(l.ctx, id); gerr != nil {
		l.Logger.Infof("删除幂等：instance 不存在，视为已删除 sub=%s id=%d", sub, id)
		return nil
	}

	return writeWithAudit(l.ctx, l.svcCtx, sub, "delete_instance", instanceObject(id), req, func() error {
		// 第一步：先删库（资源先消失；删库失败直接返回，元组原样保留、
		// 资源仍可正常访问，重试删除即可——状态安全）。
		if derr := l.svcCtx.InstanceRepo.Delete(l.ctx, id); derr != nil {
			return derr
		}
		// 第二步：库已删，再清全部元组。读取/删除均带退避重试；任何失败
		// 都不改变"资源已删除"的结果，仅记录残留（运维可清）。
		tuples, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, instanceObject(id), "")
		if rerr != nil {
			l.Logger.Errorf("instance 已删，元组读取失败（残留待清理）id=%s: %v", instanceObject(id), rerr)
			return nil
		}
		for _, t := range tuples {
			if derr := deleteRelationWithRetry(l.ctx, l.svcCtx.Authz, t.Object, t.User, t.Relation); derr != nil {
				l.Logger.Errorf("instance 已删，元组清理失败（残留待清理）tuple=%+v: %v", t, derr)
			}
		}
		return nil
	})
}
