package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlRepo 是 ItemRepo 的 MySQL 参考实现：database/sql 直写（无 ORM），
// 五种操作各一条参数化 SQL，演示 repo 层 SQL 直写的参考写法。
type mysqlRepo struct {
	db *sql.DB
}

// NewMySQL 创建 MySQL 参考实现并对 DSN 做 Ping 校验，连接不可用立即返回 error
// （svc 装配层如何处置由调用方决定）。存储模型见 migrations/001_init.sql。
//
// DSN 规范化：
//   - ParseTime=true：DATETIME 扫描进 time.Time 必需，缺失时 Scan 会报
//     unsupported Scan；
//   - Loc=time.Local：与写入侧 time.Now() 时区一致，避免驱动默认 UTC 造成读写错位；
//   - ClientFoundRows=true：把 RowsAffected 语义从"实际改变的行数"改为"匹配的
//     行数"。必须项——Update 依赖 RowsAffected 区分命中/未命中，MySQL DATETIME
//     默认秒精度下同一秒内的同值更新改变行数为 0，不设置会把存在的记录误判为
//     ErrNotFound（违反接口契约：ErrNotFound 仅表示未命中），且与内存实现不一致。
func NewMySQL(dsn string) (ItemRepo, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析 MySQL DSN 失败: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.ClientFoundRows = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	return &mysqlRepo{db: db}, nil
}

// itemColumns 是全部查询共用的列清单（descr 命名规避 MySQL 保留字 desc；
// 审计列 create_by/update_by 仅写路径使用，查询不投影）。
const itemColumns = "id, name, descr, created_at, updated_at"

// Create 插入一条记录：id 列不传（DB 自增主键生成，LastInsertId 回填），
// 时间戳与审计字段（create_by/update_by 取入参，缺省 '' = 未知操作人）由本
// 实现填充，返回完整记录。
func (m *mysqlRepo) Create(ctx context.Context, it Item) (Item, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		"INSERT INTO items (name, descr, create_by, update_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		it.Name, it.Desc, it.CreateBy, it.UpdateBy, now, now)
	if err != nil {
		return Item{}, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, fmt.Errorf("insert item last insert id: %w", err)
	}
	it.ID = id
	it.CreatedAt, it.UpdatedAt = now, now
	return it, nil
}

// Get 按 id 查询单条记录；未命中统一返回 ErrNotFound。
func (m *mysqlRepo) Get(ctx context.Context, id int64) (Item, error) {
	row := m.db.QueryRowContext(ctx,
		"SELECT "+itemColumns+" FROM items WHERE id = ?", id)
	it, err := scanItem(row.Scan)
	if err != nil {
		return Item{}, err
	}
	return it, nil
}

// List 返回全部记录，按 id 升序，与内存实现的排序语义一致。
func (m *mysqlRepo) List(ctx context.Context) ([]Item, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT "+itemColumns+" FROM items ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	out := make([]Item, 0)
	for rows.Next() {
		it, err := scanItem(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items: %w", err)
	}
	return out, nil
}

// Update 按 it.ID 覆盖 Name/Desc 并刷新 UpdatedAt 与 update_by（CreatedAt/
// create_by 不动）；通过 RowsAffected 判定未命中返回 ErrNotFound，再回读保证
// 返回完整记录。
func (m *mysqlRepo) Update(ctx context.Context, it Item) (Item, error) {
	now := time.Now()
	res, err := m.db.ExecContext(ctx,
		"UPDATE items SET name = ?, descr = ?, update_by = ?, updated_at = ? WHERE id = ?",
		it.Name, it.Desc, it.UpdateBy, now, it.ID)
	if err != nil {
		return Item{}, fmt.Errorf("update item %d: %w", it.ID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		if err != nil {
			return Item{}, fmt.Errorf("update item %d: rows affected: %w", it.ID, err)
		}
		return Item{}, ErrNotFound
	}
	return m.Get(ctx, it.ID)
}

// Delete 按 id 删除记录；RowsAffected 为 0 视为未命中返回 ErrNotFound。
func (m *mysqlRepo) Delete(ctx context.Context, id int64) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete item %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete item %d: rows affected: %w", id, err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanFn 抽象 *sql.Row.Scan 与 *sql.Rows.Scan 的共同签名，供 scanItem 复用。
type scanFn func(dest ...any) error

// scanItem 将一行查询结果扫描为存储模型 Item；sql.ErrNoRows 统一转 ErrNotFound。
func scanItem(scan scanFn) (Item, error) {
	var it Item
	if err := scan(&it.ID, &it.Name, &it.Desc, &it.CreatedAt, &it.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("scan item: %w", err)
	}
	return it, nil
}
