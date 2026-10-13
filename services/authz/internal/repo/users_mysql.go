// users_mysql.go 是 UserRepo + UserDirectory 的 MySQL 实现（B 阶段改造 1.1）：
// 用户档案持久化到业务库 nemo_auth 的 users 表。Dsn 配置非空时由 svc 装配层
// 启用，空 = 内存回退（与 authz 的 repo 装配同模式）。
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

// mysqlUserRepo 基于 nemo_auth 业务库连接池的用户 repo 实现。
type mysqlUserRepo struct {
	// db 业务库（nemo_auth）连接池。
	db *sql.DB
}

// NewMysqlUserRepo 打开 nemo_auth 业务库连接并构造 MySQL 实现，对 DSN 做
// Ping 校验，连接不可用立即返回 error（svc 装配层快速失败）。
//
// DSN 规范化（与 authz repo 同款，理由详见其注释）：ParseTime=true（DATETIME
// 扫描进 time.Time 必需）、Loc=time.Local（与写入侧 time.Now() 时区一致）。
func NewMysqlUserRepo(dsn string) (*mysqlUserRepo, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析 MySQL DSN 失败: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = time.Local
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	return &mysqlUserRepo{db: db}, nil
}

// Close 关闭连接池（幂等），供服务停机时释放连接。
func (r *mysqlUserRepo) Close() error {
	return r.db.Close()
}

// GetByIdp 按 IdP 身份查用户，未命中返回 ErrNotFound（包装 sql.ErrNoRows，
// 保持 errors.Is 可判定——登录路径以此判定首登建档）。
func (r *mysqlUserRepo) GetByIdp(ctx context.Context, idpType, idpSubject string) (*User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, email, idp_type, idp_subject FROM users WHERE idp_type = ? AND idp_subject = ?",
		idpType, idpSubject)
	var u User
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.IdpType, &u.IdpSubject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("查询用户(idp=%s subject=%s): %w", idpType, idpSubject, err)
	}
	return &u, nil
}

// Create 建档：INSERT 不含 id（DB 自增主键），create_by/update_by 以建档人
// IdpSubject 填充；ON DUPLICATE KEY UPDATE id = id（同 (idp_type, idp_subject)
// 唯一键命中时不覆盖既有展示资料——首登语义展示资料以首次建档为准，与内存
// 实现幂等语义一致），再回读返回落库记录（含 DB 生成的 ID）。参数化 SQL。
func (r *mysqlUserRepo) Create(ctx context.Context, u User) (*User, error) {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO users (name, email, idp_type, idp_subject, create_by, update_by, created_at, updated_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE id = id",
		u.Name, u.Email, u.IdpType, u.IdpSubject, u.IdpSubject, u.IdpSubject, now, now)
	if err != nil {
		return nil, fmt.Errorf("建档用户(idp=%s subject=%s): %w", u.IdpType, u.IdpSubject, err)
	}
	return r.GetByIdp(ctx, u.IdpType, u.IdpSubject)
}

// List 目录查询：keyword 对 name / idp_subject 模糊（LOWER + LIKE 子串，
// 参数化通配符防注入）；MySQL 侧 LIMIT/OFFSET 分页，COUNT 同口径取总数。
func (r *mysqlUserRepo) List(ctx context.Context, q ListUsersQuery) ([]User, int, error) {
	where := " WHERE (? = '' OR LOWER(name) LIKE ? OR LOWER(idp_subject) LIKE ?)"
	args := []any{strings.ToLower(q.Keyword)}
	if kw := strings.ToLower(q.Keyword); kw != "" {
		args = append(args, "%"+kw+"%", "%"+kw+"%")
	} else {
		args = append(args, "", "")
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计用户总数: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, email, idp_type, idp_subject FROM users"+where+
			" ORDER BY idp_subject LIMIT ? OFFSET ?",
		append(args, q.PageSize, (q.Page-1)*q.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("列出用户: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.IdpType, &u.IdpSubject); err != nil {
			return nil, 0, fmt.Errorf("扫描用户行: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历用户行: %w", err)
	}
	return users, total, nil
}

// GetBySubjects 批量反查：idp_subject IN (...) 参数化；未命中主体静默跳过。
// 空入参直接返回空切片（IN () 非法 SQL，显式短路）。
func (r *mysqlUserRepo) GetBySubjects(ctx context.Context, subjects []string) ([]User, error) {
	cleaned := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if s = strings.TrimSpace(s); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return []User{}, nil
	}

	placeholders := make([]string, len(cleaned))
	args := make([]any, len(cleaned))
	for i, s := range cleaned {
		placeholders[i] = "?"
		args[i] = s
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, email, idp_type, idp_subject FROM users WHERE idp_subject IN ("+
			strings.Join(placeholders, ",")+") ORDER BY idp_subject", args...)
	if err != nil {
		return nil, fmt.Errorf("批量反查用户: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0, len(cleaned))
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.IdpType, &u.IdpSubject); err != nil {
			return nil, fmt.Errorf("扫描用户行: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
