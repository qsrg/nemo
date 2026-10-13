// mysql_instance.go 是 InstanceRepo 的 MySQL 实现：instances 表（nemo_redis 库，
// 见本服务 migrations/，id 为 BIGINT 自增主键、name 唯一键 uk_name，重名
// Create 报错（logic 层转 409））。
// 引擎 object id 形如 redis_instance:{int64}，由 logic 层用 strconv.FormatInt 拼接。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlInstanceRepo instances 表 MySQL 实现。
type mysqlInstanceRepo struct {
	db *sql.DB
}

// instanceColumns 是全部业务列（与 Instance 字段一一对应，Create/Get/Page/Update
// 共用，改列名只动这里 + Instance 结构体 + 迁移）。
const instanceColumns = `id, name, addr, description, cluster_id, created_by, update_by, created_at, updated_at`

// scanInstance 是行扫描公共函数（与 instanceColumns 严格同序）。
func scanInstance(scanner interface{ Scan(dest ...any) error }) (Instance, error) {
	var t Instance
	if err := scanner.Scan(&t.ID, &t.Name, &t.Addr, &t.Description, &t.ClusterID,
		&t.CreatedBy, &t.UpdateBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Instance{}, err
	}
	return t, nil
}

// Create 新建实例：id 列不传（DB 自增生成），填充时间戳与审计字段，
// 重名报错，返回落库后的完整记录（含 DB 生成的 ID）。
func (m *mysqlInstanceRepo) Create(ctx context.Context, t Instance) (Instance, error) {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO instances (name, addr, description, cluster_id, created_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Addr, t.Description, t.ClusterID, t.CreatedBy, t.UpdateBy, t.CreatedAt, now)
	if err != nil {
		// MySQL 1062 = 唯一键冲突，归一为可判定的重名错误（logic 层转 409）。
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Instance{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		return Instance{}, fmt.Errorf("create instance %s: %w", t.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Instance{}, fmt.Errorf("create instance %s last insert id: %w", t.Name, err)
	}
	t.ID = id
	return t, nil
}

// Get 按 ID 取记录；未命中统一返回 ErrNotFound。
func (m *mysqlInstanceRepo) Get(ctx context.Context, id int64) (Instance, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+instanceColumns+` FROM instances WHERE id = ?`, id)
	t, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, fmt.Errorf("get instance %d: %w", id, err)
	}
	return t, nil
}

// GetByName 按名字取记录（uk_name 点查）；未命中统一返回 ErrNotFound。
func (m *mysqlInstanceRepo) GetByName(ctx context.Context, name string) (Instance, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+instanceColumns+` FROM instances WHERE name = ?`, name)
	t, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, fmt.Errorf("get instance by name %s: %w", name, err)
	}
	return t, nil
}

// List 全量记录（id 升序，对账端点用）。
func (m *mysqlInstanceRepo) List(ctx context.Context) ([]Instance, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+instanceColumns+` FROM instances ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()
	out := make([]Instance, 0, 32)
	for rows.Next() {
		t, serr := scanInstance(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan instance: %w", serr)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return out, nil
}

// Page 按可见性谓词分页查询（列表页主路径，纯引擎口径）：谓词 = id IN (...)，
// 吃主键索引；All 视角无谓词直接翻页。ORDER BY id 保证分页稳定，OFFSET 深翻页
// 数据漂移由前端容忍（量级到游标需求时再切换）。total 为满足谓词的行数
// （COUNT 同 WHERE），页内行仍由 logic 层经引擎批量复核。
func (m *mysqlInstanceRepo) Page(ctx context.Context, f InstancePageFilter) ([]Instance, int, error) {
	page, size := normalizePage(f.Page, f.PageSize)

	where, args := instanceWhereClause(f)
	query := `SELECT ` + instanceColumns + ` FROM instances` + where + ` ORDER BY id LIMIT ? OFFSET ?`
	args = append(args, size, (page-1)*size)
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("page instances: %w", err)
	}
	defer rows.Close()
	out := make([]Instance, 0, size)
	for rows.Next() {
		t, serr := scanInstance(rows)
		if serr != nil {
			return nil, 0, fmt.Errorf("scan instance: %w", serr)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate instances: %w", err)
	}

	var total int
	if err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instances`+where, args[:len(args)-2]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count instances: %w", err)
	}
	return out, total, nil
}

// instanceWhereClause 构造可见性谓词（WHERE 子串与参数）：
//   - 基础可见性：All → 空；ID 集合为空（普通用户无任何可见对象）→ 恒假
//     谓词（空页，不返回全量）；否则 → id IN (...)（吃主键索引）；
//   - 集群维度：ClusterID > 0 时 AND cluster_id = ?（列表页恒传——先选集群
//     再出实例，idx_cluster 索引）。
func instanceWhereClause(f InstancePageFilter) (string, []any) {
	var where string
	var args []any
	switch {
	case f.All:
		// 无可见性谓词
	case len(f.IDs) == 0:
		return " WHERE 1 = 0", nil // 空集合 = 恒假谓词（空页，不返回全量）
	default:
		ph := strings.TrimSuffix(strings.Repeat("?,", len(f.IDs)), ",")
		args = make([]any, 0, len(f.IDs))
		for _, id := range f.IDs {
			args = append(args, id)
		}
		where = " WHERE id IN (" + ph + ")"
	}
	if f.ClusterID > 0 {
		if where == "" {
			where = " WHERE cluster_id = ?"
		} else {
			where += " AND cluster_id = ?"
		}
		args = append(args, f.ClusterID)
	}
	return where, args
}

// Update 按 ID 全量覆盖其余字段（update_by 由 logic 层填充）；未命中
// （影响行数 0）返回 ErrNotFound。
func (m *mysqlInstanceRepo) Update(ctx context.Context, t Instance) (Instance, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`UPDATE instances SET name = ?, addr = ?, description = ?, update_by = ?, updated_at = ?
		 WHERE id = ?`,
		t.Name, t.Addr, t.Description, t.UpdateBy, now, t.ID)
	if err != nil {
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Instance{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		return Instance{}, fmt.Errorf("update instance %d: %w", t.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Instance{}, ErrNotFound
	}
	return m.Get(ctx, t.ID)
}

// Delete 按 ID 删除；未命中（影响行数 0）返回 ErrNotFound。
func (m *mysqlInstanceRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM instances WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete instance %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountByClusterID 统计某集群下的实例数（idx_cluster 索引；删除集群的
// 非空校验用，logic 层据此 409 拒绝删除非空集群）。
func (m *mysqlInstanceRepo) CountByClusterID(ctx context.Context, clusterID int64) (int, error) {
	var n int
	if err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instances WHERE cluster_id = ?`, clusterID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count instances by cluster %d: %w", clusterID, err)
	}
	return n, nil
}

// ClusterIDsByInstanceIDs 查给定实例 ID 集合归属的集群 ID 去重集合
// （DISTINCT 走主键 IN + idx_cluster；ids 为空返回空集合——调用方语义为
// "无任何可见实例"，恒不返回全量）。
func (m *mysqlInstanceRepo) ClusterIDsByInstanceIDs(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT DISTINCT cluster_id FROM instances WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("distinct cluster ids by %d instances: %w", len(ids), err)
	}
	defer rows.Close()
	out := make([]int64, 0, 8)
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			return nil, fmt.Errorf("scan cluster id: %w", err)
		}
		out = append(out, cid)
	}
	return out, rows.Err()
}
