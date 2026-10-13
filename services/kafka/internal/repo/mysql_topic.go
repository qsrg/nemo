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

// mysqlTopicRepo 是 TopicRepo 的 MySQL 实现：topics 表（见本服务 migrations/，
// id 为 BIGINT 自增主键、name 唯一键 uk_name，重名 Create 报错（logic 层转 409））。
// 引擎 object id 形如 kafka_topic:{int64}，由 logic 层用 strconv.FormatInt 拼接。
type mysqlTopicRepo struct {
	db *sql.DB
}

// topicColumns 是全部业务列（与 Topic 字段一一对应，Create/Get/List/Page 共用，
// 改列名只动这里 + Topic 结构体 + 迁移）。
const topicColumns = `id, name, env, description, org_id, created_by, created_at, updated_at`

// scanTopic 是行扫描公共函数（与 topicColumns 严格同序）。
func scanTopic(scanner interface{ Scan(dest ...any) error }) (Topic, error) {
	var t Topic
	if err := scanner.Scan(&t.ID, &t.Name, &t.Env, &t.Description,
		&t.OrgID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Topic{}, err
	}
	return t, nil
}

// Create 新建 Topic：id 列不传（DB 自增生成），填充时间戳与审计字段
// （create_by/update_by/org_id 由 logic 层填充——归属列与引擎 org 元组双写），
// 重名报错，返回落库后的完整记录（含 DB 生成的 ID）。
func (m *mysqlTopicRepo) Create(ctx context.Context, t Topic) (Topic, error) {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`INSERT INTO topics (name, env, description, org_id, created_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Env, t.Description, t.OrgID, t.CreatedBy, t.UpdateBy, t.CreatedAt, now)
	if err != nil {
		// MySQL 1062 = 唯一键冲突，归一为可判定的重名错误（logic 层转 409）。
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Topic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		return Topic{}, fmt.Errorf("create topic %s: %w", t.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Topic{}, fmt.Errorf("create topic %s last insert id: %w", t.Name, err)
	}
	t.ID = id
	return t, nil
}

// Get 按 ID 取记录；未命中统一返回 ErrNotFound。
func (m *mysqlTopicRepo) Get(ctx context.Context, id int64) (Topic, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE id = ?`, id)
	t, err := scanTopic(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrNotFound
	}
	if err != nil {
		return Topic{}, fmt.Errorf("get topic %d: %w", id, err)
	}
	return t, nil
}

// GetByName 按名字取记录（uk_name 点查）；未命中统一返回 ErrNotFound。
func (m *mysqlTopicRepo) GetByName(ctx context.Context, name string) (Topic, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT `+topicColumns+` FROM topics WHERE name = ?`, name)
	t, err := scanTopic(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrNotFound
	}
	if err != nil {
		return Topic{}, fmt.Errorf("get topic by name %s: %w", name, err)
	}
	return t, nil
}

// List 返回全部记录，按 name 升序（对账/补偿任务用）。
func (m *mysqlTopicRepo) List(ctx context.Context) ([]Topic, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT `+topicColumns+` FROM topics ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()
	out := make([]Topic, 0)
	for rows.Next() {
		t, serr := scanTopic(rows)
		if serr != nil {
			return nil, fmt.Errorf("scan topic: %w", serr)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topics: %w", err)
	}
	return out, nil
}

// Page 按可见性谓词分页查询（列表页主路径，替代"list-objects 全量展开 +
// IN 下推"）：谓词 = org_id IN (...) OR id IN (...)，各吃 idx_org / 主键
// 索引；All 视角无谓词直接翻页。ORDER BY id 保证分页稳定，OFFSET 深翻页
// 数据漂移由前端容忍（量级到游标需求时再切换）。total 为满足谓词的行数
//（COUNT 同 WHERE），页内行仍由 logic 层经引擎逐行复核。
func (m *mysqlTopicRepo) Page(ctx context.Context, f TopicPageFilter) ([]Topic, int, error) {
	page, size := normalizePage(f.Page, f.PageSize)

	where, args := topicWhereClause(f)
	query := `SELECT ` + topicColumns + ` FROM topics` + where + ` ORDER BY id LIMIT ? OFFSET ?`
	args = append(args, size, (page-1)*size)
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("page topics: %w", err)
	}
	defer rows.Close()
	out := make([]Topic, 0, size)
	for rows.Next() {
		t, serr := scanTopic(rows)
		if serr != nil {
			return nil, 0, fmt.Errorf("scan topic: %w", serr)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate topics: %w", err)
	}

	var total int
	if err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM topics`+where, args[:len(args)-2]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count topics: %w", err)
	}
	return out, total, nil
}

// topicWhereClause 构造可见性谓词（WHERE 子串与参数）；All 或两路谓词
// 皆空时返回空串（全量视角）。IN 占位符按参数生成，空集合跳过该路。
func topicWhereClause(f TopicPageFilter) (string, []any) {
	if f.All {
		return "", nil
	}
	var conds []string
	var args []any
	if len(f.OrgIDs) > 0 {
		conds = append(conds, "org_id IN ("+placeholders(len(f.OrgIDs))+")")
		for _, id := range f.OrgIDs {
			args = append(args, id)
		}
	}
	if len(f.IDs) > 0 {
		conds = append(conds, "id IN ("+placeholders(len(f.IDs))+")")
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE (" + strings.Join(conds, " OR ") + ")", args
}

// placeholders 生成 n 个问号占位符（"?,?,?"）。
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Update 按 ID 全量覆盖其余字段（update_by 由 logic 层填充）；未命中
//（影响行数 0）返回 ErrNotFound。
func (m *mysqlTopicRepo) Update(ctx context.Context, t Topic) (Topic, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		`UPDATE topics SET name = ?, env = ?, description = ?, org_id = ?, update_by = ?, updated_at = ?
		 WHERE id = ?`,
		t.Name, t.Env, t.Description, t.OrgID, t.UpdateBy, now, t.ID)
	if err != nil {
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return Topic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		return Topic{}, fmt.Errorf("update topic %d: %w", t.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Topic{}, ErrNotFound
	}
	return m.Get(ctx, t.ID)
}

// Delete 按 ID 删除；未命中（影响行数 0）返回 ErrNotFound。
func (m *mysqlTopicRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM topics WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete topic %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
