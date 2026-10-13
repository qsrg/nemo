// migrate 命令是 sample 服务的 SQL 迁移 runner：按文件名顺序执行
// migrations/*.sql，用 schema_migrations 表记录已应用版本，重复执行幂等。
//
// 用法（在 services/sample 目录下，migrations 目录按相对 cwd 定位）：
//
//	MYSQL_DSN="user:pass@tcp(127.0.0.1:3306)/nemo?charset=utf8mb4" go run ./cmd/migrate
//
// MYSQL_DSN 未设置时输出错误并以非零码退出，绝不静默跑空。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"

	kitlog "nemo/kit/logging"
)

// migrationsDir 是迁移脚本目录，相对运行时的工作目录（约定在 services/sample 下运行）。
const migrationsDir = "migrations"

// schemaMigrationsDDL 是版本记录表的定义；IF NOT EXISTS 保证 runner 自身可重复执行。
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    VARCHAR(255) PRIMARY KEY,
    applied_at DATETIME NOT NULL
);`

// main 装配迁移流程：读 DSN → 连库 → 建版本表 → 按序应用未执行的迁移；
// 任何一步失败都以非零码退出，成功时打印应用/跳过清单。
func main() {
	kitlog.Init("migrate", false)
	if err := run(); err != nil {
		logx.Errorf("迁移失败: %v", err)
		os.Exit(1)
	}
	logx.Info("迁移全部完成")
}

// run 执行迁移主流程。
// 流程：1) 校验 MYSQL_DSN；2) 规范化 DSN（parseTime、multiStatements 以支持
// 单文件多语句）；3) 建 schema_migrations；4) 逐个未应用版本开事务执行脚本
// 并记录版本。返回 error 交由 main 统一退出。
func run() error {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		return fmt.Errorf("环境变量 MYSQL_DSN 未设置（用法: MYSQL_DSN=... go run ./cmd/migrate）")
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("解析 MySQL DSN 失败: %w", err)
	}
	cfg.ParseTime = true
	// 整个脚本文件一次 Exec 依赖 multiStatements；迁移脚本可能含多条语句。
	cfg.MultiStatements = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	if _, err := db.Exec(schemaMigrationsDDL); err != nil {
		return fmt.Errorf("创建 schema_migrations 表失败: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return fmt.Errorf("扫描 %s 失败: %w", migrationsDir, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("目录 %s 下没有迁移脚本", migrationsDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, path := range files {
		version := filepath.Base(path)
		applied, err := isApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			logx.Infof("跳过已应用迁移: %s", version)
			continue
		}
		if err := apply(ctx, db, path, version); err != nil {
			return err
		}
		logx.Infof("已应用迁移: %s", version)
	}
	return nil
}

// isApplied 查询 version 是否已记录在 schema_migrations 表中；
// 仅 ErrNoRows 视为未应用，其余查询错误显式上抛，避免静默重跑已应用迁移。
func isApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx,
		"SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&one)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("查询迁移版本 %s 失败: %w", version, err)
}

// apply 在单个事务内执行迁移脚本并写入版本记录；任一步失败整体回滚。
func apply(ctx context.Context, db *sql.DB, path, version string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取迁移脚本 %s 失败: %w", path, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(string(content)); err != nil {
		return fmt.Errorf("执行迁移脚本 %s 失败: %w", path, err)
	}
	if _, err = tx.Exec(
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		version, time.Now()); err != nil {
		return fmt.Errorf("记录迁移版本 %s 失败: %w", version, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移 %s 失败: %w", version, err)
	}
	return nil
}
