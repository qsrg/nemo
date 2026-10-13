// Code scaffolded by goctl. Safe to edit.
// goctl {{.version}}
//
// 约定：ServiceContext 显式装配依赖，无 DI 容器；repo/client 等依赖接口
// 在此构造注入，供 handler/logic 通过字段访问。

package svc

import (
	{{.configImport}}
)

// ServiceContext 聚合服务全部依赖：config 与显式装配的 repo/client 等接口。
type ServiceContext struct {
	Config {{.config}}
	{{.middleware}}
}

// NewServiceContext 根据 config 显式构造并注入全部依赖（无 DI 容器）。
func NewServiceContext(c {{.config}}) *ServiceContext {
	return &ServiceContext{
		Config: c,
		{{.middlewareAssignment}}
	}
}
