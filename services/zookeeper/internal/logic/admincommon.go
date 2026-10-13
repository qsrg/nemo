// admincommon.go 承载 zookeeper 独立服务写路径的公共设施：审计先行封装与
// 当前操作人提取。审计落本服务 audits 表（nemo_zookeeper，独立记账），action
// 语义：create_cluster/delete_cluster；grant/revoke 经
// authz 通用 relations 接口，审计由 authz 侧以 write_relations 记录。
//
// 审计先行的三态模板（CreatePending → write → MarkCommitted/MarkFailed）
// 复用 kit/audit.WriteWithAudit（与 authz/kafka 共用同一权威实现）；本文件
// 保留 writeWithAudit 薄封装 + auditSink 适配器，把本服务 repo.AuditRepo
// 映射为 kit audit.Sink。
package logic

import (
	"context"
	"strings"

	"nemo/kit/audit"
	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/zookeeper/internal/repo"
	"nemo/services/zookeeper/internal/svc"
)

// operatorFrom 从 ctx 提取当前操作人（JWT claims.sub，经 authzkit.IdentityFrom）。
// ctx 无身份（未经 Identity 中间件的异常路径）返回 401 fail-closed。
func operatorFrom(ctx context.Context) (string, error) {
	sess, err := authzkit.IdentityFrom(ctx)
	if err != nil || strings.TrimSpace(sess.Sub) == "" {
		return "", errors.Unauthorized("ZK_UNAUTHORIZED", "缺少用户身份")
	}
	return sess.Sub, nil
}

// writeWithAudit 是审计先行写路径的统一封装（复用 kit/audit 权威实现）：
//  1. AuditRepo.CreatePending 落 pending 条目（先于业务写）；
//  2. 执行 write（写库与/或经 kit/authz 写授权元组）；
//  3. 成功 → MarkCommitted；失败 → MarkFailed（附原因）并返回
//     503 ZK_SERVICE_UNAVAILABLE（fail-closed：授权服务不可用即拒绝；
//     引擎校验拒绝 Invalid tuple 按 400 ZK_ENGINE_REJECTED 区分）。
//
// 参数校验类错误（400/404/409）必须在进入本函数之前完成——本函数之后
// 的失败只可能来自授权服务/存储故障。
func writeWithAudit(ctx context.Context, sc *svc.ServiceContext, operator, action, target string, snapshot any, write func() error) error {
	return audit.WriteWithAudit(ctx, auditSink{r: sc.AuditRepo}, operator, action, target, snapshot, write)
}

// auditSink 把本服务 AuditRepo 适配为 kit audit.Sink（字段映射；
// create_by 按 AGENTS.md 审计字段规范冗余为操作人）。
type auditSink struct {
	// r 本服务审计存取（内存/MySQL 双实现）。
	r repo.AuditRepo
}

// CreatePending 落一条 pending 审计并返回自增 ID（委托本服务 repo）。
func (s auditSink) CreatePending(ctx context.Context, e audit.Entry) (int64, error) {
	return s.r.CreatePending(ctx, repo.AuditEntry{
		Operator: e.Operator,
		CreateBy: e.Operator, // 审计字段规范：create_by = 操作人（写路径唯一入口，恒有身份）。
		Action:   e.Action,
		Target:   e.Target,
		Snapshot: e.Snapshot,
	})
}

// MarkCommitted 把审计条目标记为 committed。
func (s auditSink) MarkCommitted(ctx context.Context, id int64) error {
	return s.r.MarkCommitted(ctx, id)
}

// MarkFailed 把审计条目标记为 failed（附原因）。
func (s auditSink) MarkFailed(ctx context.Context, id int64, reason string) error {
	return s.r.MarkFailed(ctx, id, reason)
}
