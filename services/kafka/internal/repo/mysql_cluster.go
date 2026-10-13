// mysql_cluster.go 是 ClusterRepo 的 MySQL 实现：clusters 表（见本服务
// migrations/003_clusters.sql，id 自增主键、code 唯一键 uk_code，重名 Create
// 报错（logic 层转 409））。分页按 id 降序（新集群在前），COUNT 取总数。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlClusterRepo 是 ClusterRepo 的 MySQL 实现。
type mysqlClusterRepo struct {
	db *sql.DB
}

// clusterColumns 是全部业务列（与 Cluster 字段一一对应，各方法共用）。
const clusterColumns = `id, code, dc, address, remark, created_by, update_by, created_at, updated_at`

// scanCluster 是行扫描公共函数（与 clusterColumns 严格同序）。
func scanCluster(scanner interface{ Scan(dest ...any) error }) (Cluster, error) {
	var c Cluster
	if err := scanner.Scan(&c.ID, &c.Code, &c.Dc, &c.Address, &c.Remark,
		&c.CreatedBy, &c.UpdateBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return Cluster{}, err
	}
	return c, nil
}

// Create 新建集群：id 列不传（DB 自增生成），填充时间戳与审计字段，
// code 冲突返回 ErrDuplicate（MySQL 1062 归一，logic 层转 409）。
func (m *mysqlClusterRepo) Create(ctx context.Context, c Cluster) (Cluster, error) {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO clusters (code, dc, address, remark, created_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Code, c.Dc, c.Address, c.Remark, c.CreatedBy, c.UpdateBy, c.CreatedAt, now)
	if err != nil {
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Cluster{}, ErrDuplicate
		}
		return Cluster{}, err
	}
	c.ID, err = res.LastInsertId()
	if err != nil {
		return Cluster{}, err
	}
	c.UpdatedAt = now
	return c, nil
}

// Update 更新集群属性（code 不参与更新——对象身份不可变）；未命中返回
// ErrNotFound（RowsAffected==0 判定，需 ClientFoundRows 语义一致）。
func (m *mysqlClusterRepo) Update(ctx context.Context, c Cluster) error {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`UPDATE clusters SET dc = ?, address = ?, remark = ?, update_by = ?, updated_at = ?
		 WHERE id = ?`,
		c.Dc, c.Address, c.Remark, c.UpdateBy, now, c.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete 删除集群；未命中返回 ErrNotFound。
func (m *mysqlClusterRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetById 按 ID 取单条；未命中返回 ErrNotFound。
func (m *mysqlClusterRepo) GetById(ctx context.Context, id int64) (Cluster, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE id = ?`, id)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Cluster{}, ErrNotFound
	}
	return c, err
}

// GetByCode 按集群编码取单条；未命中返回 ErrNotFound（重名预检用）。
func (m *mysqlClusterRepo) GetByCode(ctx context.Context, code string) (Cluster, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE code = ?`, code)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Cluster{}, ErrNotFound
	}
	return c, err
}

// Page 分页查询：id 降序（新集群在前），返回当前页记录与总数。
// 页码从 1 开始，越界页返回空列表（与 topics 列表口径一致）。
func (m *mysqlClusterRepo) Page(ctx context.Context, page, pageSize int) ([]Cluster, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	var total int64
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clusters`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters ORDER BY id DESC LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Cluster, 0, pageSize)
	for rows.Next() {
		c, serr := scanCluster(rows)
		if serr != nil {
			return nil, 0, serr
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}
