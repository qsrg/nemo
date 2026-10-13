// mysql_policy.go 是 PolicyRepo 的 MySQL 实现：policies 表
// （见 migrations/007_policies.sql）。id BIGINT 自增主键。
package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// mysqlPolicyRepo 策略存储 MySQL 实现。
type mysqlPolicyRepo struct {
	db *sql.DB
}

// NewMySQLPolicyRepo 构造策略存储 MySQL 实现（复用业务库连接池）。
func NewMySQLPolicyRepo(db *sql.DB) PolicyRepo {
	return &mysqlPolicyRepo{db: db}
}

// Create 新建策略（id 由 DB 自增生成）。
func (m *mysqlPolicyRepo) Create(ctx context.Context, p Policy) (Policy, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO policies (name, resource_type, effect, subject_attr, op, resource_attr, attr_value, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.ResourceType, p.Effect, p.SubjectAttr, string(p.Op), p.ResourceAttr, p.Value, p.Enabled, now, now)
	if err != nil {
		return Policy{}, fmt.Errorf("create policy: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Policy{}, fmt.Errorf("create policy last insert id: %w", err)
	}
	p.ID = id
	p.CreatedAt = now
	p.UpdatedAt = now
	return p, nil
}

// List 返回全部策略（按 ID 升序）。
func (m *mysqlPolicyRepo) List(ctx context.Context) ([]Policy, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, name, resource_type, effect, subject_attr, op, resource_attr, attr_value, enabled, created_at, updated_at
		 FROM policies ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer rows.Close()
	out := make([]Policy, 0)
	for rows.Next() {
		var p Policy
		var op string
		if err := rows.Scan(&p.ID, &p.Name, &p.ResourceType, &p.Effect, &p.SubjectAttr, &op,
			&p.ResourceAttr, &p.Value, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		p.Op = PolicyOp(op)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate policies: %w", err)
	}
	return out, nil
}

// Get 按 ID 查单条；未命中返回 ErrNotFound。
func (m *mysqlPolicyRepo) Get(ctx context.Context, id int64) (Policy, error) {
	var p Policy
	var op string
	err := m.db.QueryRowContext(ctx,
		`SELECT id, name, resource_type, effect, subject_attr, op, resource_attr, attr_value, enabled, created_at, updated_at
		 FROM policies WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.ResourceType, &p.Effect, &p.SubjectAttr, &op,
			&p.ResourceAttr, &p.Value, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, fmt.Errorf("get policy %d: %w", id, err)
	}
	p.Op = PolicyOp(op)
	return p, nil
}

// Update 按 ID 全量覆盖其余字段；未命中返回 ErrNotFound。
func (m *mysqlPolicyRepo) Update(ctx context.Context, p Policy) (Policy, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`UPDATE policies SET name = ?, resource_type = ?, effect = ?, subject_attr = ?, op = ?,
		 resource_attr = ?, attr_value = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.ResourceType, p.Effect, p.SubjectAttr, string(p.Op), p.ResourceAttr, p.Value, p.Enabled, now, p.ID)
	if err != nil {
		return Policy{}, fmt.Errorf("update policy %d: %w", p.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Policy{}, ErrNotFound
	}
	p.UpdatedAt = now
	return p, nil
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。
func (m *mysqlPolicyRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM policies WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete policy %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
