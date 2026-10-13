// mysql_audit.go 是 AuditRepo 的 MySQL 实现：audits 表（nemo_redis 库，
// 与 kafka/authz 审计表同构，独立记账）。
package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// mysqlAuditRepo 审计表 MySQL 实现。
type mysqlAuditRepo struct {
	db *sql.DB
}

// CreatePending 写入一条 pending 审计并返回自增 ID；create_by 为空时回落 operator。
func (m *mysqlAuditRepo) CreatePending(ctx context.Context, e AuditEntry) (int64, error) {
	if e.CreateBy == "" {
		e.CreateBy = e.Operator
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO audits (operator, action, target, snapshot, status, reason, create_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Operator, e.Action, e.Target, e.Snapshot, AuditPending, e.Reason, e.CreateBy, "", now, now)
	if err != nil {
		return 0, fmt.Errorf("create pending audit: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create pending audit last insert id: %w", err)
	}
	return id, nil
}

// MarkCommitted 把审计条目标记为 committed（update_by 置系统标识）。
func (m *mysqlAuditRepo) MarkCommitted(ctx context.Context, id int64) error {
	return m.mark(ctx, id, AuditCommitted, "")
}

// MarkFailed 把审计条目标记为 failed（附原因）。
func (m *mysqlAuditRepo) MarkFailed(ctx context.Context, id int64, reason string) error {
	return m.mark(ctx, id, AuditFailed, reason)
}

// mark 是状态标记的公共实现：影响行数 0 视为未命中（ErrNotFound）。
func (m *mysqlAuditRepo) mark(ctx context.Context, id int64, st AuditStatus, reason string) error {
	res, err := m.db.ExecContext(ctx,
		`UPDATE audits SET status = ?, reason = ?, update_by = ?, updated_at = ? WHERE id = ?`,
		st, reason, auditSystemUpdater, time.Now(), id)
	if err != nil {
		return fmt.Errorf("mark audit %d %s: %w", id, st, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListFailedTargets 查指定动作集合下 status=failed 的去重 target 集合
// （idx_status 索引过滤 + 应用层去重；对账端点用，量级为失败笔数）。
func (m *mysqlAuditRepo) ListFailedTargets(ctx context.Context, actions []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(actions) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(actions)), ",")
	args := make([]any, 0, len(actions))
	for _, a := range actions {
		args = append(args, a)
	}
	rows, err := m.db.QueryContext(ctx,
		"SELECT DISTINCT target FROM audits WHERE status = ? AND action IN ("+placeholders+")",
		append([]any{AuditFailed}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list failed targets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan failed target: %w", err)
		}
		out[t] = true
	}
	return out, rows.Err()
}
