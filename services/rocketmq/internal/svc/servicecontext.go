// servicecontext.go 定义 rocketmq 独立服务的依赖装配：业务存储（TopicRepo
// MySQL 实现，Dsn 留空时回退内存演示）+ 集中式授权接入件（kit/authz
// Remote，判定 + 元组写面）。
package svc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"nemo/kit/authz"
	"nemo/services/rocketmq/internal/config"
	"nemo/services/rocketmq/internal/repo"
)

// ServiceContext 是 rocketmq 服务的全局依赖容器。
type ServiceContext struct {
	Config config.Config
	// TopicRepo 业务数据存取（MySQL 实现；Dsn 为空时内存实现）。
	TopicRepo repo.TopicRepo
	// ClusterRepo 集群存取（MySQL 实现；Dsn 为空时内存实现）。
	ClusterRepo repo.ClusterRepo
	// Authz 集中式授权接入件：Check/ListObjects/WriteRelations（判定 +
	// 元组写），全部转发到 authz。
	Authz *authz.Remote
}

// NewServiceContext 按配置装配依赖：Dsn 非空时建 MySQL 连接（探活失败即
// 启动失败，不静默回退内存）；Authz 恒为 Remote（BaseURL 指向集中式
// authz 服务）。
func NewServiceContext(c config.Config) *ServiceContext {
	sc := &ServiceContext{Config: c}
	if c.Mysql.Dsn != "" {
		db, err := sql.Open("mysql", c.Mysql.Dsn)
		if err != nil {
			panic(fmt.Sprintf("rocketmq mysql open: %v", err))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			panic(fmt.Sprintf("rocketmq mysql ping: %v", err))
		}
		sc.TopicRepo = repo.NewMySQLTopicRepo(db)
		sc.ClusterRepo = repo.NewMySQLClusterRepo(db)
	} else {
		// 内存回退：仅演示用（重启丢数据）。
		sc.TopicRepo = repo.NewMemoryTopicRepo()
		sc.ClusterRepo = repo.NewMemoryClusterRepo()
	}
	sc.Authz = authz.New(authz.Config{BaseURL: c.Authz.BaseURL, Timeout: c.Authz.Timeout})
	return sc
}
