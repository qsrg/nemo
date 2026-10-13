package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// mysqlAuditRepo 是 AuditRepo 的 MySQL 实现：audits 表（见 migrations/001_biz.sql），
// id 为 BIGINT 自增主键；Status 用 ENUM('pending','committed','failed') 列约束，
// Snapshot 存 JSON 文本，reason 列记录 MarkFailed 的原因（模型无该字段，仅供排障查询）。
type mysqlAuditRepo struct {
	db *sql.DB
}

// auditColumns 是 audits 表查询共用的列清单（不含实现内部的 reason）。
const auditColumns = "id, operator, action, target, snapshot, status, created_at"

// CreatePending 写入一条 pending 审计：id 列不传（DB 自增生成），Status 强制置
// pending（忽略入参状态），create_by 取 e.CreateBy（为空回落 e.Operator），
// CreatedAt 填充，返回 DB 自增生成的 ID（int64）。
// 应用层不再生成 audit-<unixnano> 字符串 ID。
func (m *mysqlAuditRepo) CreatePending(ctx context.Context, e AuditEntry) (int64, error) {
	createBy := e.CreateBy
	if createBy == "" {
		createBy = e.Operator
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO audits (operator, action, target, snapshot, status, reason, create_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?)`,
		e.Operator, e.Action, e.Target, e.Snapshot, AuditPending, createBy, "", now, now)
	if err != nil {
		return 0, fmt.Errorf("create pending audit: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create pending audit last insert id: %w", err)
	}
	return id, nil
}

// MarkCommitted 把审计条目置为 committed；RowsAffected 为 0（ClientFoundRows
// 语义下即未命中）返回 ErrNotFound。
func (m *mysqlAuditRepo) MarkCommitted(ctx context.Context, id int64) error {
	return m.markStatus(ctx, id, AuditCommitted, "")
}

// MarkFailed 把审计条目置为 failed 并记录原因；未命中返回 ErrNotFound。
func (m *mysqlAuditRepo) MarkFailed(ctx context.Context, id int64, reason string) error {
	return m.markStatus(ctx, id, AuditFailed, reason)
}

// markStatus 统一执行状态标记：UPDATE 后按 RowsAffected（匹配行数）判定命中；
// update_by 置 auditSystemUpdater——状态标记由写路径提交与对账进程完成，属
// 系统行为（AGENTS.md 审计字段规范）。
func (m *mysqlAuditRepo) markStatus(ctx context.Context, id int64, status AuditStatus, reason string) error {
	res, err := m.db.ExecContext(ctx,
		"UPDATE audits SET status = ?, reason = ?, update_by = ?, updated_at = ? WHERE id = ?",
		status, reason, auditSystemUpdater, time.Now(), id)
	if err != nil {
		return fmt.Errorf("mark audit %d %s: %w", id, status, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("mark audit %d %s: rows affected: %w", id, status, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPending 列出创建时间早于（now - olderThan）且仍 pending 的条目；
// olderThan 为负时分界线落在未来，即取全部 pending。
func (m *mysqlAuditRepo) ListPending(ctx context.Context, olderThan time.Duration) ([]AuditEntry, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := m.db.QueryContext(ctx,
		"SELECT "+auditColumns+" FROM audits WHERE status = ? AND created_at < ? "+
			"ORDER BY created_at ASC, id ASC",
		AuditPending, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list pending audits: %w", err)
	}
	return scanAuditRows(rows)
}

// Query 按过滤条件分页查询审计：Operator 非空时按操作人过滤，按
//（CreatedAt, ID）倒序（新→旧），返回当前页条目与符合条件的总数。
// Page < 1 视为第 1 页；PageSize < 1 视为默认 20。
func (m *mysqlAuditRepo) Query(ctx context.Context, filter AuditFilter) ([]AuditEntry, int, error) {
	where, args := "WHERE 1=1", []any{}
	if filter.Operator != "" {
		where += " AND operator = ?"
		args = append(args, filter.Operator)
	}
	if filter.ExcludeSystem {
		where += " AND operator <> ?"
		args = append(args, auditSystemUpdater)
	}
	var total int
	if err := m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audits "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audits: %w", err)
	}
	page, size := normalizePage(filter.Page, filter.PageSize)
	rows, err := m.db.QueryContext(ctx,
		"SELECT "+auditColumns+" FROM audits "+where+
			" ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?",
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audits: %w", err)
	}
	entries, err := scanAuditRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// scanAuditRows 把查询结果集扫描为 []AuditEntry（先关闭 rows），并检查迭代错误。
func scanAuditRows(rows *sql.Rows) ([]AuditEntry, error) {
	defer rows.Close()
	out := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Operator, &e.Action, &e.Target,
			&e.Snapshot, &e.Status, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audits: %w", err)
	}
	return out, nil
}

// ListFailedTargets 查指定动作集合下 status=failed 的去重 target 集合
//（授权对账用：failed 的创建审计意味着业务行已被回滚或待人工清理，
// 对账不得为其补写 owner——否则回滚失败的脏行被洗白成有效资源）。
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
