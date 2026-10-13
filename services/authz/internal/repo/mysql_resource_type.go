package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// mysqlResourceTypeRepo 是 ResourceTypeRepo 的 MySQL 实现：resource_types 表
// （见 migrations/001_biz.sql）。主键改造：id 为 BIGINT 自增主键，name 为
// 业务唯一键（uk_name，重名 Create 报错，logic 层转 409）。
type mysqlResourceTypeRepo struct {
	db *sql.DB
}

// Create 注册资源类型：id 列不传（DB 自增生成），填充 CreatedAt（调用方已填
// 则尊重原值）与 create_by，重名报错，返回落库后的完整记录（含 DB 生成的 ID）。
func (m *mysqlResourceTypeRepo) Create(ctx context.Context, rt ResourceType) (ResourceType, error) {
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = time.Now()
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		"INSERT INTO resource_types (name, dsl_block, manage_code, create_code, create_by, update_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		rt.Name, rt.DslBlock, rt.ManageCode, rt.CreateCode, rt.CreateBy, rt.UpdateBy, rt.CreatedAt, now)
	if err != nil {
		return ResourceType{}, fmt.Errorf("create resource type %s: %w", rt.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ResourceType{}, fmt.Errorf("create resource type %s last insert id: %w", rt.Name, err)
	}
	rt.ID = id
	return rt, nil
}

// Get 按名取资源类型；未命中统一返回 ErrNotFound。
func (m *mysqlResourceTypeRepo) Get(ctx context.Context, name string) (ResourceType, error) {
	row := m.db.QueryRowContext(ctx,
		"SELECT id, name, dsl_block, manage_code, create_code, created_at FROM resource_types WHERE name = ?", name)
	var rt ResourceType
	if err := row.Scan(&rt.ID, &rt.Name, &rt.DslBlock, &rt.ManageCode, &rt.CreateCode, &rt.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResourceType{}, ErrNotFound
		}
		return ResourceType{}, fmt.Errorf("get resource type %s: %w", name, err)
	}
	return rt, nil
}

// List 返回全部资源类型，按 name 升序（与内存实现排序语义一致）。
func (m *mysqlResourceTypeRepo) List(ctx context.Context) ([]ResourceType, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT id, name, dsl_block, manage_code, create_code, created_at FROM resource_types ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list resource types: %w", err)
	}
	defer rows.Close()
	out := make([]ResourceType, 0)
	for rows.Next() {
		var rt ResourceType
		if err := rows.Scan(&rt.ID, &rt.Name, &rt.DslBlock, &rt.ManageCode, &rt.CreateCode, &rt.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan resource type: %w", err)
		}
		out = append(out, rt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resource types: %w", err)
	}
	return out, nil
}

// Update 更新资源类型的 DSL 块与名单管理码：按 name 定位（类型名不可改），
// 未命中返回 ErrNotFound；保留 create_by，更新 update_by 与 updated_at。
// **不碰 create_code**：创建码是管理面一次性声明（006 迁移/部署设置），
// 派生同步（resourcetypesync）永远不覆盖它。
func (m *mysqlResourceTypeRepo) Update(ctx context.Context, rt ResourceType) error {
	res, err := m.db.ExecContext(ctx,
		"UPDATE resource_types SET dsl_block = ?, manage_code = ?, update_by = ?, updated_at = NOW() WHERE name = ?",
		rt.DslBlock, rt.ManageCode, rt.UpdateBy, rt.Name)
	if err != nil {
		return fmt.Errorf("update resource type %s: %w", rt.Name, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("update resource type %s rows: %w", rt.Name, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete 按名删除资源类型；未命中返回 ErrNotFound。
func (m *mysqlResourceTypeRepo) Delete(ctx context.Context, name string) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM resource_types WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete resource type %s: %w", name, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete resource type %s rows: %w", name, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
