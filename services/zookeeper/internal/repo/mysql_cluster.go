// mysql_cluster.go 是 ClusterRepo 的 MySQL 实现：clusters 表（nemo_redis 库，
// migrations/002_clusters.sql，id 为 BIGINT 自增主键、code 唯一键 uk_code，
// 重码 Create 报错（logic 层转 409））。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlClusterRepo clusters 表 MySQL 实现。
type mysqlClusterRepo struct {
	db *sql.DB
}

// clusterColumns 是全部业务列（与 Cluster 字段一一对应，改列名只动这里 +
// Cluster 结构体 + 迁移）。
const clusterColumns = `id, code, hosts, remark, created_by, update_by, created_at, updated_at`

// scanCluster 是行扫描公共函数（与 clusterColumns 严格同序）。
func scanCluster(scanner interface{ Scan(dest ...any) error }) (Cluster, error) {
	var c Cluster
	if err := scanner.Scan(&c.ID, &c.Code, &c.Hosts, &c.Remark,
		&c.CreatedBy, &c.UpdateBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return Cluster{}, err
	}
	return c, nil
}

// Create 新建集群：id 列不传（DB 自增生成），填充时间戳与审计字段，
// code 重复报错，返回落库后的完整记录（含 DB 生成的 ID）。
func (m *mysqlClusterRepo) Create(ctx context.Context, c Cluster) (Cluster, error) {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO clusters (code, hosts, remark, created_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Code, c.Hosts, c.Remark, c.CreatedBy, c.UpdateBy, c.CreatedAt, now)
	if err != nil {
		// MySQL 1062 = 唯一键冲突，归一为可判定的重码错误（logic 层转 409）。
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Cluster{}, fmt.Errorf("%w: %s", ErrDuplicate, c.Code)
		}
		return Cluster{}, fmt.Errorf("create cluster %s: %w", c.Code, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Cluster{}, fmt.Errorf("create cluster %s last insert id: %w", c.Code, err)
	}
	c.ID = id
	c.UpdatedAt = now
	return c, nil
}

// GetById 按 ID 取记录；未命中统一返回 ErrNotFound。
func (m *mysqlClusterRepo) GetById(ctx context.Context, id int64) (Cluster, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE id = ?`, id)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Cluster{}, ErrNotFound
	}
	if err != nil {
		return Cluster{}, fmt.Errorf("get cluster %d: %w", id, err)
	}
	return c, nil
}

// GetByCode 按 code 取记录（uk_code 点查）；未命中统一返回 ErrNotFound。
func (m *mysqlClusterRepo) GetByCode(ctx context.Context, code string) (Cluster, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE code = ?`, code)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Cluster{}, ErrNotFound
	}
	if err != nil {
		return Cluster{}, fmt.Errorf("get cluster by code %s: %w", code, err)
	}
	return c, nil
}

// List 全量列表（集群量级个位/十级，不分页），ORDER BY id 升序保证选择器顺序稳定。
func (m *mysqlClusterRepo) List(ctx context.Context) ([]Cluster, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	defer rows.Close()
	out := make([]Cluster, 0, 8)
	for rows.Next() {
		c, serr := scanCluster(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan cluster: %w", serr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clusters: %w", err)
	}
	return out, nil
}

// Delete 按 ID 删除；未命中（影响行数 0）返回 ErrNotFound。集群下是否仍有
// 实例由 logic 层校验（跨表一致性，409 拒绝非空集群）。
func (m *mysqlClusterRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM clusters WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete cluster %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
