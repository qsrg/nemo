// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：ServiceContext 显式装配依赖，无 DI 容器；repo/client 等依赖接口
// 在此构造注入，供 handler/logic 通过字段访问。

package svc

import (
	"net/http"

	"nemo/services/sample/internal/config"
	"nemo/services/sample/internal/repo"
)

// ServiceContext 聚合服务全部依赖：config 与显式装配的 repo/client 等接口。
type ServiceContext struct {
	Config   config.Config
	ItemRepo repo.ItemRepo
}

// NewServiceContext 根据 config 显式构造并注入全部依赖（无 DI 容器）：
// Mysql.Dsn 为空装配内存实现（骨架/演示期默认），否则装配 MySQL 实现；
// 连接失败直接 panic——启动期依赖不可用必须快速失败，不吞错。
func NewServiceContext(c config.Config) *ServiceContext {
	var (
		r   repo.ItemRepo
		err error
	)
	if c.Mysql.Dsn == "" {
		r = repo.NewMemory()
	} else {
		if r, err = repo.NewMySQL(c.Mysql.Dsn); err != nil {
			panic(err)
		}
	}
	return &ServiceContext{
		Config:   c,
		ItemRepo: r,
	}
}

// NewTestServiceContext 构造单测专用 ServiceContext：内存 repo，无外部依赖。
// 仅限 logic/clientx 单测使用，禁止进入生产装配路径。
func NewTestServiceContext() *ServiceContext {
	return &ServiceContext{ItemRepo: repo.NewMemory()}
}

// ReadyHandler 是 /readyz 的就绪探针：内存模式恒 200；MySQL 模式下以一次只读
// 查询（List）探活——不用 sql.Ping 而用业务只读查询，可同时暴露连接与表结构
// （迁移未执行）两类不可用，且无需为探针在 repo 接口上扩展 Ping 方法；
// 探活失败返回 503。
func (sc *ServiceContext) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if sc.Config.Mysql.Dsn != "" {
		if _, err := sc.ItemRepo.List(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
