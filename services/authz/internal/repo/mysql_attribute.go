// mysql_attribute.go 是 AttributeRepo 的 MySQL 实现：attributes 表
// （见 migrations/006_attributes.sql）。owner+attr_key 唯一键 uk_owner_key，
// Set 走 INSERT ... ON DUPLICATE KEY UPDATE（幂等 upsert）。
package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// mysqlAttributeRepo 属性存储 MySQL 实现。
type mysqlAttributeRepo struct {
	db *sql.DB
}

// NewMySQLAttributeRepo 构造属性存储 MySQL 实现（dsn 复用业务库连接池）。
func NewMySQLAttributeRepo(db *sql.DB) AttributeRepo {
	return &mysqlAttributeRepo{db: db}
}

// Set 写入单条属性（INSERT ... ON DUPLICATE KEY UPDATE 幂等覆盖）。
func (m *mysqlAttributeRepo) Set(ctx context.Context, a Attribute) error {
	now := time.Now()
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO attributes (owner, attr_key, attr_value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE attr_value = VALUES(attr_value), updated_at = VALUES(updated_at)`,
		a.Owner, a.AttrKey, a.AttrValue, now, now)
	if err != nil {
		return fmt.Errorf("set attribute %s/%s: %w", a.Owner, a.AttrKey, err)
	}
	return nil
}

// Get 返回某 owner 的全部属性；无属性返回空 map（不返回 ErrNotFound）。
func (m *mysqlAttributeRepo) Get(ctx context.Context, owner string) (map[string]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT attr_key, attr_value FROM attributes WHERE owner = ?`, owner)
	if err != nil {
		return nil, fmt.Errorf("get attributes %s: %w", owner, err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan attribute: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attributes %s: %w", owner, err)
	}
	return out, nil
}

// Delete 删除单条属性（幂等：不存在不报错）。
func (m *mysqlAttributeRepo) Delete(ctx context.Context, owner, key string) error {
	if _, err := m.db.ExecContext(ctx,
		`DELETE FROM attributes WHERE owner = ? AND attr_key = ?`, owner, key); err != nil {
		return fmt.Errorf("delete attribute %s/%s: %w", owner, key, err)
	}
	return nil
}

// getAttributeDB 返回属性存储用的 *sql.DB（供 mysql.go 装配共用连接池）。
func getAttributeDB(db *sql.DB) *sql.DB { return db }
