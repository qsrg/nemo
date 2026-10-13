// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 独立服务改造：判定与授权名单读写全部经 kit/authz 转发集中式 authz。

package logic

import (
	"context"

	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteKafkaTopicLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteKafkaTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteKafkaTopicLogic 构造业务规则层实例。
func NewDeleteKafkaTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteKafkaTopicLogic {
	return &DeleteKafkaTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteKafkaTopic 删除 Topic（删除权判定已上收路由守卫，业务代码只关注业务）。
//
// 执行顺序：
//  1. 删除权判定在路由层完成（routes.go 挂 requireTopicPerm("can_delete")：
//     can_delete 主判定（owner，2026-09-30 起独立边界）+ 管理码回退），无权
//     403 / 判定服务故障 503 fail-closed；
//  2. 资源不存在 → 直接返回成功（**幂等**：重复删除/前端重试不报错；守卫
//     前置后仅管理码持有者可达本分支——普通 owner 的 owner 元组随删除被清，
//     重试已删资源将在守卫处 403，与 PUT 的"判定先于存在性"同口径，同时
//     关闭原实现"403=存在 / 200=不存在"的存在性枚举通道）；悬空元组由
//     reconcile 统一清理；
//  3. 审计先行（writeWithAudit，action=delete_topic），闭包内**先删库、
//     后清元组**：
//     - 先删库 → 资源立即消失，后续判定即不可见。避免旧顺序"元组先删完、
//     删库失败 → 资源还在但 owner 已删、谁都管不了（锁死）"；
//     - 后清元组 → 库已删，清理失败只残留垃圾（不影响"资源已删除"的业务
//     结果），逐条带退避重试，仍失败仅记录日志、不阻断，残留由
//     reconcile/运维清理。
//
// 已知待办（代码评审标记，尚未处理）：幂等分支未区分存储故障与未命中
// （应 errors.Is(gerr, repo.ErrNotFound)）；元组清理失败被吞、审计标
// committed（editor 删除场景清理必然 403——authz 名单接口要求 owner/管理码）。
func (l *DeleteKafkaTopicLogic) DeleteKafkaTopic(req *types.DeleteTopicReq) error {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	id, err := parseTopicID(req.Id)
	if err != nil {
		return err
	}

	// 幂等：资源已不存在（上次删除已生效）→ 成功返回，不抛 404。
	if _, gerr := l.svcCtx.TopicRepo.Get(l.ctx, id); gerr != nil {
		l.Logger.Infof("删除幂等：topic 不存在，视为已删除 sub=%s topic=%d", sub, id)
		return nil
	}

	return writeWithAudit(l.ctx, l.svcCtx, sub, "delete_topic", topicObject(id), req, func() error {
		// 第一步：先删库（资源先消失；删库失败直接返回，元组原样保留、
		// 资源仍可正常访问，重试删除即可——状态安全）。
		if derr := l.svcCtx.TopicRepo.Delete(l.ctx, id); derr != nil {
			return derr
		}
		// 第二步：库已删，再清全部元组。读取/删除均带退避重试；任何失败
		// 都不改变"资源已删除"的结果，仅记录残留（reconcile/运维可清）。
		tuples, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, topicObject(id), "")
		if rerr != nil {
			l.Logger.Errorf("topic 已删，元组读取失败（残留待清理）topic=%s: %v", topicObject(id), rerr)
			return nil
		}
		for _, t := range tuples {
			if derr := deleteRelationWithRetry(l.ctx, l.svcCtx.Authz, t.Object, t.User, t.Relation); derr != nil {
				l.Logger.Errorf("topic 已删，元组清理失败（残留待清理）tuple=%+v: %v", t, derr)
			}
		}
		return nil
	})
}
