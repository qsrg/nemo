// remoteretry.go 承载 redis 服务对集中式 authz 远程元组操作的重试包装：
// 写 owner / 删元组等关键远程调用在遇到瞬时网络抖动、引擎短暂不可用（5xx、
// 超时）时按固定退避自动重试，覆盖绝大多数偶发故障；校验类拒绝（4xx）
// 不重试——重试不会改变结果。
//
// 设计边界：重试只消除"瞬时故障"，不保证跨服务操作的原子性。重试耗尽后
// 的持久故障仍由审计 failed 记录 + 运维清理补偿，不静默忽略。
package logic

import (
	"context"
	"errors"
	"time"

	authzkit "nemo/kit/authz"
	kiterrors "nemo/kit/errors"
)

const (
	// remoteRetryAttempts 是远程元组操作的最大尝试次数（含首次执行）。
	remoteRetryAttempts = 3
	// remoteRetryBackoff 是退避基数：第 n 次失败后等待 base*n（线性退避）。
	remoteRetryBackoff = 120 * time.Millisecond
)

// withRemoteRetry 执行远程操作闭包，失败时按线性退避重试，直到成功或达到
// 最大尝试次数。
//
// 参数：
//   - ctx：请求上下文，取消/超时时立即终止重试并返回 ctx.Err；
//   - op：单次远程操作闭包，返回 nil 表示成功。
//
// 返回：成功为 nil；重试耗尽返回最后一次错误。仅 5xx/网络类错误触发重试，
// 4xx 类明确响应立即返回不重试。
func withRemoteRetry(ctx context.Context, op func() error) error {
	var err error
	for attempt := 1; attempt <= remoteRetryAttempts; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		if !retryableRemoteErr(err) || attempt == remoteRetryAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * remoteRetryBackoff):
		}
	}
	return err
}

// retryableRemoteErr 判断远程错误是否值得重试。
//
// 判断规则：
//   - *kiterrors.Error：仅 Status >= 500（服务端/引擎故障）可重试，
//     400/401/403/404/409 等明确判定不重试；
//   - 非结构化错误（网络不可达、超时、连接重置等）：默认可重试。
func retryableRemoteErr(err error) bool {
	var ke *kiterrors.Error
	if errors.As(err, &ke) {
		return ke.Status >= 500
	}
	return true
}

// writeRelationsWithRetry 带退避重试地批量写入授权元组。
// 参数与返回语义同 authzkit.Remote.WriteRelations，仅在可重试错误时重试。
func writeRelationsWithRetry(ctx context.Context, c *authzkit.Remote, tuples []authzkit.Tuple) (int, error) {
	var written int
	err := withRemoteRetry(ctx, func() error {
		n, werr := c.WriteRelations(ctx, tuples)
		if werr == nil {
			written = n
		}
		return werr
	})
	return written, err
}

// deleteRelationWithRetry 带退避重试地删除单条授权元组。
// 参数与返回语义同 authzkit.Remote.DeleteRelation，仅在可重试错误时重试。
func deleteRelationWithRetry(ctx context.Context, c *authzkit.Remote, object, user, relation string) error {
	return withRemoteRetry(ctx, func() error {
		return c.DeleteRelation(ctx, object, user, relation)
	})
}

// listRelationsWithRetry 带退避重试地读取对象的授权元组。
// 参数与返回语义同 authzkit.Remote.ListRelations，仅在可重试错误时重试。
func listRelationsWithRetry(ctx context.Context, c *authzkit.Remote, object, relation string) ([]authzkit.Tuple, error) {
	var items []authzkit.Tuple
	err := withRemoteRetry(ctx, func() error {
		rows, lerr := c.ListRelations(ctx, object, relation)
		if lerr == nil {
			items = rows
		}
		return lerr
	})
	return items, err
}
