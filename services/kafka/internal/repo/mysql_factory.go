// mysql_factory.go 提供 TopicRepo/AuditRepo 的 MySQL 构造器。
// （authz 侧由 NewMySQLRepos 统一装配，kafka 独立服务按需单取。）
package repo

import "database/sql"

// NewMySQLTopicRepo 构造 TopicRepo 的 MySQL 实现。
func NewMySQLTopicRepo(db *sql.DB) TopicRepo {
	return &mysqlTopicRepo{db: db}
}

// NewMySQLClusterRepo 构造 ClusterRepo 的 MySQL 实现。
func NewMySQLClusterRepo(db *sql.DB) ClusterRepo {
	return &mysqlClusterRepo{db: db}
}

// NewMySQLAuditRepo 构造 AuditRepo 的 MySQL 实现。
func NewMySQLAuditRepo(db *sql.DB) AuditRepo {
	return &mysqlAuditRepo{db: db}
}
