// mysql_cluster.go 是 ClusterRepo 的 MySQL 实现：clusters 表（见本服务
// migrations/002_clusters.sql，id BIGINT 自增主键、code 唯一键 uk_code）。
// 集群为基础配置数据 + 集群级授权锚点（rmq_cluster 引擎对象与集群行一一对应）。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// mysqlClusterRepo 是 ClusterRepo 的 MySQL 实现。
type mysqlClusterRepo struct {
	db *sql.DB
}

// NewMySQLClusterRepo 构造 ClusterRepo 的 MySQL 实现（供 svc 按 Dsn 装配）。
func NewMySQLClusterRepo(db *sql.DB) ClusterRepo {
	return &mysqlClusterRepo{db: db}
}

// clusterColumns 是全部业务列（与 Cluster 字段一一对应，共用）。
const clusterColumns = `id, code, remark, created_by, update_by, created_at, updated_at`

// scanCluster 是行扫描公共函数（与 clusterColumns 严格同序）。
func scanCluster(scanner interface{ Scan(dest ...any) error }) (Cluster, error) {
	var c Cluster
	var createdAt, updatedAt interface{}
	if err := scanner.Scan(&c.ID, &c.Code, &c.Remark, &c.CreatedBy,
		&c.UpdateBy, &createdAt, &updatedAt); err != nil {
		return Cluster{}, err
	}
	return c, nil
}

// Create 新建集群：id 列不传（DB 自增生成），code 重复报错，返回落库后的完整记录。
func (m *mysqlClusterRepo) Create(ctx context.Context, c Cluster) (Cluster, error) {
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO clusters (code, remark, created_by, update_by)
		 VALUES (?, ?, ?, ?)`,
		c.Code, c.Remark, c.CreatedBy, c.CreatedBy)
	if err != nil {
		// MySQL 1062 = 唯一键冲突，归一为可判定的重名错误（logic 层转 409）。
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Cluster{}, fmt.Errorf("%w: %s", ErrClusterDuplicate, c.Code)
		}
		return Cluster{}, fmt.Errorf("create cluster %s: %w", c.Code, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Cluster{}, fmt.Errorf("create cluster %s last insert id: %w", c.Code, err)
	}
	c.ID = id
	return c, nil
}

// GetById 按 ID 取记录；未命中返回 ErrClusterNotFound。
func (m *mysqlClusterRepo) GetById(ctx context.Context, id int64) (Cluster, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE id = ?`, id)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Cluster{}, ErrClusterNotFound
	}
	if err != nil {
		return Cluster{}, fmt.Errorf("get cluster %d: %w", id, err)
	}
	return c, nil
}

// GetByCode 按 code 取记录（uk_code 点查，创建路径重名预检）；未命中返回 ErrClusterNotFound。
func (m *mysqlClusterRepo) GetByCode(ctx context.Context, code string) (Cluster, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters WHERE code = ?`, code)
	c, err := scanCluster(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Cluster{}, ErrClusterNotFound
	}
	if err != nil {
		return Cluster{}, fmt.Errorf("get cluster by code %s: %w", code, err)
	}
	return c, nil
}

// List 返回全部记录，按 id 升序保证输出确定性。
func (m *mysqlClusterRepo) List(ctx context.Context) ([]Cluster, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+clusterColumns+` FROM clusters ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	defer rows.Close()
	out := make([]Cluster, 0, 16)
	for rows.Next() {
		c, serr := scanCluster(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan cluster: %w", serr)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Delete 按 ID 删除；未命中返回 ErrClusterNotFound。集群下是否仍有 topic
// 由 logic 层校验（此处不做跨表检查，禁外键规范）。
func (m *mysqlClusterRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete cluster %d: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete cluster %d rows affected: %w", id, err)
	}
	if affected == 0 {
		return ErrClusterNotFound
	}
	return nil
}
