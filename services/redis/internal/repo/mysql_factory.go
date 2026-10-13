// mysql_factory.go 提供 InstanceRepo/AuditRepo/ClusterRepo 的 MySQL 构造器。
package repo

import "database/sql"

// NewMySQLInstanceRepo 构造 InstanceRepo 的 MySQL 实现。
func NewMySQLInstanceRepo(db *sql.DB) InstanceRepo {
	return &mysqlInstanceRepo{db: db}
}

// NewMySQLAuditRepo 构造 AuditRepo 的 MySQL 实现。
func NewMySQLAuditRepo(db *sql.DB) AuditRepo {
	return &mysqlAuditRepo{db: db}
}

// NewMySQLClusterRepo 构造 ClusterRepo 的 MySQL 实现。
func NewMySQLClusterRepo(db *sql.DB) ClusterRepo {
	return &mysqlClusterRepo{db: db}
}
