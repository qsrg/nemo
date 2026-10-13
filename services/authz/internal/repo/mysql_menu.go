package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// mysqlMenuRepo 是 MenuRepo 的 MySQL 实现：menus 表（见 migrations/），
// id 为 BIGINT 自增主键（行身份），parent_id 引用父行（顶级为 NULL），
// permission_code 可空唯一（NULL = 全员可见，非空码全表唯一）。
// 应用层以 0 表示"无父行"、空串表示"无码"，落库前统一转 NULL。
type mysqlMenuRepo struct {
	db *sql.DB
}

// nullParentID 应用层父引用（0 = 顶级）→ 存储层 NULL。
func nullParentID(parentID int64) any {
	if parentID == 0 {
		return nil
	}
	return parentID
}

// nullCode 应用层权限码（空串 = 无码/全员可见）→ 存储层 NULL。
func nullCode(code string) any {
	if code == "" {
		return nil
	}
	return code
}

// Create 新建一行（ID 自增生成），返回落库后的完整记录（含创建时间回读）。
// 审计字段：新建写 create_by（AGENTS.md 规范）。
func (m *mysqlMenuRepo) Create(ctx context.Context, menu Menu) (Menu, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO menus (parent_id, type, title, icon, path, sort, permission_code, resource_type, relation, enabled, create_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullParentID(menu.ParentID), menu.Type, menu.Title, menu.Icon, menu.Path,
		menu.Sort, nullCode(menu.PermissionCode), menu.ResourceType, menu.Relation,
		menu.Enabled, menu.CreateBy, menu.UpdateBy, now, now)
	if err != nil {
		return Menu{}, fmt.Errorf("create menu %s: %w", menu.Title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Menu{}, fmt.Errorf("create menu %s: last insert id: %w", menu.Title, err)
	}
	return m.Get(ctx, id)
}

// Update 按 ID 全量覆盖（PUT 语义）；未命中（影响行数为 0）返回 ErrNotFound
//（NewMySQLRepos 已设 ClientFoundRows=true，其他 update 同口径）。
// 审计字段：覆盖写 update_by，不碰 create_by。
func (m *mysqlMenuRepo) Update(ctx context.Context, menu Menu) (Menu, error) {
	res, err := m.db.ExecContext(ctx,
		`UPDATE menus SET parent_id = ?, type = ?, title = ?, icon = ?, path = ?,
		   sort = ?, permission_code = ?, resource_type = ?, relation = ?,
		   enabled = ?, update_by = ?
		 WHERE id = ?`,
		nullParentID(menu.ParentID), menu.Type, menu.Title, menu.Icon, menu.Path,
		menu.Sort, nullCode(menu.PermissionCode), menu.ResourceType, menu.Relation,
		menu.Enabled, menu.UpdateBy, menu.ID)
	if err != nil {
		return Menu{}, fmt.Errorf("update menu %d: %w", menu.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Menu{}, ErrNotFound
	}
	return m.Get(ctx, menu.ID)
}

// menuScanColumns 菜单行 SELECT 列（List/Get 共用）。
const menuScanColumns = `id, parent_id, type, title, icon, path, sort, permission_code, resource_type, relation, enabled, created_at, updated_at`

// scanMenu 公共行扫描：parent_id/permission_code 的 NULL 落模型零值
//（0 = 顶级；空串 = 全员可见）。
func scanMenu(rows interface{ Scan(...any) error }) (Menu, error) {
	var menu Menu
	var parentID sql.NullInt64
	var code sql.NullString
	if err := rows.Scan(&menu.ID, &parentID, &menu.Type, &menu.Title,
		&menu.Icon, &menu.Path, &menu.Sort, &code,
		&menu.ResourceType, &menu.Relation, &menu.Enabled,
		&menu.CreatedAt, &menu.UpdatedAt); err != nil {
		return Menu{}, err
	}
	menu.ParentID = parentID.Int64
	menu.PermissionCode = code.String
	return menu, nil
}

// List 返回全部记录，按 (sort, id) 升序（与内存实现排序语义一致）。
func (m *mysqlMenuRepo) List(ctx context.Context) ([]Menu, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+menuScanColumns+`
		 FROM menus ORDER BY sort, id`)
	if err != nil {
		return nil, fmt.Errorf("list menus: %w", err)
	}
	defer rows.Close()
	out := make([]Menu, 0)
	for rows.Next() {
		menu, serr := scanMenu(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan menu: %w", serr)
		}
		out = append(out, menu)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate menus: %w", err)
	}
	return out, nil
}

// Get 按 ID 取记录；未命中统一返回 ErrNotFound。
func (m *mysqlMenuRepo) Get(ctx context.Context, id int64) (Menu, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+menuScanColumns+`
		 FROM menus WHERE id = ?`, id)
	menu, err := scanMenu(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Menu{}, ErrNotFound
		}
		return Menu{}, fmt.Errorf("get menu %d: %w", id, err)
	}
	return menu, nil
}

// Delete 按 ID 删除；未命中（影响行数为 0）返回 ErrNotFound。
func (m *mysqlMenuRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM menus WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete menu %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
