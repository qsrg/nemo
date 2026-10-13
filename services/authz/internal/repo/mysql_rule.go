package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// mysqlRuleRepo 是 RuleRepo 的 MySQL 实现：rules 表（见 migrations/001_biz.sql），
// id 为 BIGINT 自增主键；Upsert 按 ID=0/非0 区分新建（INSERT 不含 id 列，
// DB 自增）与更新（UPDATE WHERE id=?）。
type mysqlRuleRepo struct {
	db *sql.DB
}

// Upsert 新建或更新规则：ID=0 视为新建，DB 自增生成 ID 后回填；
// ID 非 0 按 ID 覆盖更新，未命中返回 ErrNotFound（保留 upsert 双语义）。
// 审计字段：新建写 create_by，更新写 update_by（AGENTS.md 规范）。
// 应用层不再生成字符串 ID（原 rule-<unixnano> 已下线）。
func (m *mysqlRuleRepo) Upsert(ctx context.Context, r Rule) (Rule, error) {
	now := time.Now()
	if r.ID == 0 {
		res, err := m.db.ExecContext(ctx,
			`INSERT INTO rules (role_key, prefix, action, create_by, update_by, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.RoleKey, r.Prefix, r.Action, r.CreateBy, r.UpdateBy, now, now)
		if err != nil {
			return Rule{}, fmt.Errorf("create rule: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return Rule{}, fmt.Errorf("create rule last insert id: %w", err)
		}
		r.ID = id
		return r, nil
	}
	res, err := m.db.ExecContext(ctx,
		`UPDATE rules SET role_key = ?, prefix = ?, action = ?, update_by = ?, updated_at = ? WHERE id = ?`,
		r.RoleKey, r.Prefix, r.Action, r.UpdateBy, now, r.ID)
	if err != nil {
		return Rule{}, fmt.Errorf("update rule %d: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Rule{}, ErrNotFound
	}
	return r, nil
}

// List 返回全部规则，按 id 升序（与内存实现排序语义一致）。
func (m *mysqlRuleRepo) List(ctx context.Context) ([]Rule, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT id, role_key, prefix, action FROM rules ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	out := make([]Rule, 0)
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.RoleKey, &r.Prefix, &r.Action); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return out, nil
}

// Delete 按 ID 删除规则；RowsAffected 为 0 视为未命中返回 ErrNotFound。
func (m *mysqlRuleRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM rules WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete rule %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete rule %d: rows affected: %w", id, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
