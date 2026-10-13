// servicecontext.go 定义 kafka 独立服务的依赖装配：业务存储（TopicRepo/
// AuditRepo）+ 集中式授权接入件（kit/authz Remote，判定与 relations 共用）。
package svc

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/authz"
	"nemo/services/kafka/internal/config"
	"nemo/services/kafka/internal/repo"
)

// ServiceContext 是 kafka 服务的全局依赖容器。
type ServiceContext struct {
	Config config.Config
	// TopicRepo 业务数据存取（内存/MySQL 双实现）。
	TopicRepo repo.TopicRepo
	// ClusterRepo 集群基础信息存取（内存/MySQL 双实现）。
	ClusterRepo repo.ClusterRepo
	// AuditRepo 审计存取（写操作审计先行；复用 authz 审计表）。
	AuditRepo repo.AuditRepo
	// Authz 集中式授权接入件：Check/ListObjects（判定面）+ Relations
	// （授权名单读/写/删），全部转发到 authz 服务。
	Authz *authz.Remote
	// readyErr 就绪探针状态（MySQL 探活失败时非 nil）。
	readyErr error
	// db 业务库连接（MySQL 模式持有，供探活；内存模式为 nil）。
	db *sql.DB
	// once 保护 db 初始化与探活。
	once sync.Once
}

// NewServiceContext 按配置装配依赖：
//   - Mysql.Dsn 非空 → MySQL 模式（TopicRepo/AuditRepo 走 mysql 实现）；
//   - 为空 → 内存模式（骨架/演示期默认，重启丢数据）。
//
// Authz 恒为 Remote（BaseURL 指向集中式 authz 服务）。
func NewServiceContext(c config.Config) *ServiceContext {
	sc := &ServiceContext{Config: c}

	if c.Mysql.Dsn != "" {
		// 与 authz 一致：ParseDSN 强制 ParseTime/Loc/ClientFoundRows 语义。
		cfg, derr := mysql.ParseDSN(c.Mysql.Dsn)
		if derr != nil {
			logx.Errorf("kafka: 解析 MySQL DSN 失败: %v", derr)
			sc.readyErr = derr
		} else {
			cfg.ParseTime = true
			cfg.Loc = time.Local
			cfg.ClientFoundRows = true
			db, oerr := sql.Open("mysql", cfg.FormatDSN())
			if oerr != nil {
				logx.Errorf("kafka: 打开 MySQL 失败: %v", oerr)
				sc.readyErr = oerr
			} else {
				sc.db = db
				sc.TopicRepo = repo.NewMySQLTopicRepo(db)
				sc.AuditRepo = repo.NewMySQLAuditRepo(db)
				sc.ClusterRepo = repo.NewMySQLClusterRepo(db)
			}
		}
	}
	if sc.TopicRepo == nil {
		sc.TopicRepo = repo.NewMemoryTopicRepo()
		sc.AuditRepo = repo.NewMemoryAuditRepo()
		sc.ClusterRepo = repo.NewMemoryClusterRepo()
		logx.Info("kafka: 使用内存存储（Mysql.Dsn 为空，仅演示用）")
	}

	sc.Authz = authz.New(authz.Config{BaseURL: c.Authz.BaseURL, Timeout: c.Authz.Timeout})
	return sc
}

// ReadyHandler 是 /readyz 就绪探针：内存模式恒 200；MySQL 模式探活成功
// 才 200（依赖可用性见 AGENTS.md 可运维性规范）。
func (sc *ServiceContext) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	sc.once.Do(func() {
		if sc.db == nil {
			return
		}
		if err := sc.db.Ping(); err != nil {
			sc.readyErr = err
		}
	})
	if sc.readyErr != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("mysql unavailable: " + sc.readyErr.Error()))
		return
	}
	if sc.db != nil {
		if err := sc.db.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("mysql unavailable: " + err.Error()))
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
