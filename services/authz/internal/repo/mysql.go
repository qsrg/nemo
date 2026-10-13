package repo

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// MySQLRepos 聚合业务库四个 repo 的 MySQL 实现：共享同一个连接池
// （*sql.DB），由 svc 装配层按 Mysql.Dsn 非空时整体启用。
type MySQLRepos struct {
	// db 业务库（nemo_authz_biz）共享连接池。
	db *sql.DB
	// RuleRepo 规则表实现。
	RuleRepo RuleRepo
	// ResourceTypeRepo 资源类型注册表实现。
	ResourceTypeRepo ResourceTypeRepo
	// ModelVersionRepo 模型版本实现。
	ModelVersionRepo ModelVersionRepo
	// AuditRepo 审计表实现。
	AuditRepo AuditRepo
	// MenuRepo 菜单注册表实现（Phase C 改造①）。
	MenuRepo MenuRepo
	// AttributeRepo 鉴权属性存储实现（ABAC 方案 A：主体/资源属性）。
	AttributeRepo AttributeRepo
	// PolicyRepo ABAC 策略存储实现（ABAC 方案 A：deny 策略）。
	PolicyRepo PolicyRepo
	// OrgStore 组织元数据注册表实现（orgs 表，migrations/004_orgs.sql）。
	OrgStore OrgStore
}

// NewMySQLRepos 打开业务库连接并构造四个 repo 的 MySQL 实现，对 DSN 做
// Ping 校验，连接不可用立即返回 error（svc 装配层快速失败）。
//
// DSN 规范化（与 sample 参考实现同款，理由详见各字段注释）：
//   - ParseTime=true：DATETIME 扫描进 time.Time 必需；
//   - Loc=time.Local：与写入侧 time.Now() 时区一致，避免读写错位；
//   - ClientFoundRows=true：RowsAffected 语义改为"匹配的行数"。必须项——
//     MarkCommitted/MarkFailed 依赖 RowsAffected 区分命中/未命中，MySQL
//     DATETIME 默认秒精度下同一秒内的重复标记改变行数为 0，不设置会把
//     存在的条目误判为 ErrNotFound（违反接口契约），且与内存实现不一致。
func NewMySQLRepos(dsn string) (*MySQLRepos, error) {
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
	return &MySQLRepos{
		db:               db,
		RuleRepo:         &mysqlRuleRepo{db: db},
		ResourceTypeRepo: &mysqlResourceTypeRepo{db: db},
		ModelVersionRepo: &mysqlModelVersionRepo{db: db},
		AuditRepo:        &mysqlAuditRepo{db: db},
		MenuRepo:         &mysqlMenuRepo{db: db},
		AttributeRepo:    &mysqlAttributeRepo{db: db},
		PolicyRepo:       &mysqlPolicyRepo{db: db},
		OrgStore:         &mysqlOrgStore{db: db},
	}, nil
}

// Close 关闭共享连接池（幂等），供服务停机时释放连接。
func (r *MySQLRepos) Close() error {
	return r.db.Close()
}
