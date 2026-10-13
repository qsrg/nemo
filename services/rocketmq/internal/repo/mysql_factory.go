// mysql_factory.go 提供 TopicRepo 的 MySQL 构造器。
package repo

import "database/sql"

// NewMySQLTopicRepo 构造 TopicRepo 的 MySQL 实现。
func NewMySQLTopicRepo(db *sql.DB) TopicRepo {
	return &mysqlTopicRepo{db: db}
}
