// mysql_topic.go 是 TopicRepo 的 MySQL 实现：topics 表（见本服务 migrations/，
// id 为 BIGINT 自增主键、name 唯一键 uk_name，重名 Create 报错（logic 层转 409））。
// 引擎 object id 形如 rmq_topic:{int64}，由 logic 层用 strconv.FormatInt 拼接。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"

	"nemo/services/rocketmq/internal/types"
)

// mysqlTopicRepo 是 TopicRepo 的 MySQL 实现。
type mysqlTopicRepo struct {
	db *sql.DB
}

// topicColumns 是全部业务列（与 Topic 字段一一对应，Create/Get/List/Update 共用）。
const topicColumns = `id, name, env, description, cluster_id, org_id, created_by, created_at, updated_at`

// scanTopic 是行扫描公共函数（与 topicColumns 严格同序）。
func scanTopic(scanner interface{ Scan(dest ...any) error }) (types.RocketmqTopic, error) {
	var t types.RocketmqTopic
	var createdAt, updatedAt interface{}
	if err := scanner.Scan(&t.Id, &t.Name, &t.Env, &t.Description,
		&t.ClusterId, &t.OrgId, &t.CreatedBy, &createdAt, &updatedAt); err != nil {
		return types.RocketmqTopic{}, err
	}
	return t, nil
}

// Create 新建 Topic：id 列不传（DB 自增生成），重名报错，返回落库后的完整记录。
func (m *mysqlTopicRepo) Create(ctx context.Context, t types.RocketmqTopic) (types.RocketmqTopic, error) {
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO topics (name, env, description, cluster_id, org_id, created_by, update_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Env, t.Description, t.ClusterId, t.OrgId, t.CreatedBy, t.CreatedBy)
	if err != nil {
		// MySQL 1062 = 唯一键冲突，归一为可判定的重名错误（logic 层转 409）。
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return types.RocketmqTopic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		return types.RocketmqTopic{}, fmt.Errorf("create topic %s: %w", t.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return types.RocketmqTopic{}, fmt.Errorf("create topic %s last insert id: %w", t.Name, err)
	}
	t.Id = id
	return t, nil
}

// Get 按 ID 取记录；未命中返回 ErrNotFound。
func (m *mysqlTopicRepo) Get(ctx context.Context, id int64) (types.RocketmqTopic, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE id = ?`, id)
	t, err := scanTopic(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RocketmqTopic{}, ErrNotFound
	}
	if err != nil {
		return types.RocketmqTopic{}, fmt.Errorf("get topic %d: %w", id, err)
	}
	return t, nil
}

// GetByName 按名字取记录（uk_name 点查，创建路径重名预检）；未命中返回 ErrNotFound。
func (m *mysqlTopicRepo) GetByName(ctx context.Context, name string) (types.RocketmqTopic, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE name = ?`, name)
	t, err := scanTopic(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RocketmqTopic{}, ErrNotFound
	}
	if err != nil {
		return types.RocketmqTopic{}, fmt.Errorf("get topic by name %s: %w", name, err)
	}
	return t, nil
}

// ListByCluster 返回某集群下的全部记录，按 name 升序保证输出确定性。
func (m *mysqlTopicRepo) ListByCluster(ctx context.Context, clusterId int64) ([]types.RocketmqTopic, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE cluster_id = ? ORDER BY name`, clusterId)
	if err != nil {
		return nil, fmt.Errorf("list topics by cluster %d: %w", clusterId, err)
	}
	defer rows.Close()
	out := make([]types.RocketmqTopic, 0, 16)
	for rows.Next() {
		t, serr := scanTopic(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan topic: %w", serr)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// List 返回全部记录，按 name 升序保证输出确定性（对账等全局视角用）。
func (m *mysqlTopicRepo) List(ctx context.Context) ([]types.RocketmqTopic, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+topicColumns+` FROM topics ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()
	out := make([]types.RocketmqTopic, 0, 16)
	for rows.Next() {
		t, serr := scanTopic(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan topic: %w", serr)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Update 按 ID 覆盖 env/description/org_id（name 不可改：对象身份锚点），
// 未命中返回 ErrNotFound；org_id 不可改（归属变更走专门流程），保持原值。
func (m *mysqlTopicRepo) Update(ctx context.Context, t types.RocketmqTopic) (types.RocketmqTopic, error) {
	res, err := m.db.ExecContext(ctx,
		`UPDATE topics SET env = ?, description = ? WHERE id = ?`,
		t.Env, t.Description, t.Id)
	if err != nil {
		return types.RocketmqTopic{}, fmt.Errorf("update topic %d: %w", t.Id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return types.RocketmqTopic{}, fmt.Errorf("update topic %d rows affected: %w", t.Id, err)
	}
	if affected == 0 {
		// 行不存在（或值无变化——两者都按未命中处理，编辑语义幂等）。
		if _, gerr := m.Get(ctx, t.Id); gerr != nil {
			if errors.Is(gerr, ErrNotFound) {
				return types.RocketmqTopic{}, ErrNotFound
			}
			return types.RocketmqTopic{}, gerr
		}
	}
	// 返回覆盖后完整记录（org_id/created_by 取库内现值）。
	return m.Get(ctx, t.Id)
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。
func (m *mysqlTopicRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM topics WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete topic %d: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete topic %d rows affected: %w", id, err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
