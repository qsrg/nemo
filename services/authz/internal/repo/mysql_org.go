// mysql_org.go 是 OrgStore 的 MySQL 实现：orgs 表（见 migrations/004_orgs.sql），
// org_id 为业务唯一键（uk_org_id），id 为 BIGINT 自增主键（不外露接口）；
// Upsert 走 INSERT ... ON DUPLICATE KEY UPDATE（按 org_id 唯一键判定新建/更新）。
// 组织元数据随 CAS 同步与管理 API 写入，审计字段（create_by/update_by）由
// 调用方填充：管理 API 为 JWT sub，定时同步为 system（AGENTS.md 审计规范）。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// mysqlOrgStore 是 OrgStore 的 MySQL 实现（共享 NewMySQLRepos 连接池）。
type mysqlOrgStore struct {
	db *sql.DB
}

// Upsert 按 org_id 新建或更新组织元数据（INSERT ... ON DUPLICATE KEY UPDATE），
// id 列不传（DB 自增生成）；更新分支不覆盖 create_by（创建人语义不可变），
// 返回落库后的完整记录（含创建时间回读）。
func (m *mysqlOrgStore) Upsert(ctx context.Context, o OrgInfo) (OrgInfo, error) {
	now := time.Now()
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO orgs (org_id, name, parent_id, create_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   name = VALUES(name), parent_id = VALUES(parent_id),
		   update_by = VALUES(update_by), updated_at = VALUES(updated_at)`,
		o.ID, o.Name, o.ParentID, o.CreateBy, o.UpdateBy, now, now)
	if err != nil {
		return OrgInfo{}, fmt.Errorf("upsert org %s: %w", o.ID, err)
	}
	return m.Get(ctx, o.ID)
}

// Get 按 org_id 取组织元数据；未命中返回 ErrNotFound。
func (m *mysqlOrgStore) Get(ctx context.Context, id string) (OrgInfo, error) {
	var o OrgInfo
	var createdAt time.Time
	err := m.db.QueryRowContext(ctx,
		`SELECT org_id, name, parent_id, create_by, update_by, created_at
		 FROM orgs WHERE org_id = ?`, id).
		Scan(&o.ID, &o.Name, &o.ParentID, &o.CreateBy, &o.UpdateBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgInfo{}, ErrNotFound
	}
	if err != nil {
		return OrgInfo{}, fmt.Errorf("get org %s: %w", id, err)
	}
	o.CreatedAt = createdAt
	return o, nil
}

// Delete 按 org_id 删除组织元数据；未命中返回 ErrNotFound（与内存实现语义一致，
// 由调用方决定删除是否容错）。
func (m *mysqlOrgStore) Delete(ctx context.Context, id string) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM orgs WHERE org_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete org %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// List 返回全部组织元数据，按 org_id 升序（与内存实现排序语义一致）。
func (m *mysqlOrgStore) List(ctx context.Context) ([]OrgInfo, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT org_id, name, parent_id, create_by, update_by, created_at
		 FROM orgs ORDER BY org_id`)
	if err != nil {
		return nil, fmt.Errorf("list orgs: %w", err)
	}
	defer rows.Close()
	out := make([]OrgInfo, 0)
	for rows.Next() {
		var o OrgInfo
		var createdAt time.Time
		if err := rows.Scan(&o.ID, &o.Name, &o.ParentID, &o.CreateBy, &o.UpdateBy, &createdAt); err != nil {
			return nil, fmt.Errorf("scan org: %w", err)
		}
		o.CreatedAt = createdAt
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orgs: %w", err)
	}
	return out, nil
}
