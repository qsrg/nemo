// Package logging 统一 go-zero logx 的初始化与访问日志字段约定
// （架构 §9.2、backend-design §8）：本地 console、生产 JSON 文件轮转，
// 访问日志字段 method/path/status/duration_ms/request_id。
package logging

import (
	"github.com/zeromicro/go-zero/core/logx"
)

// Init 初始化全局日志，必须在服务 main 最先调用。
//
//	prod=false：本地开发，console 输出、debug 级别；
//	prod=true：生产，JSON 编码输出到 logs/<serviceName>/ 并由 logx 内置策略轮转。
//
// prod 与 serviceName 不可用时行为：空 serviceName 仅影响文件命名，不报错。
func Init(serviceName string, prod bool) {
	conf := logx.LogConf{ServiceName: serviceName, Encoding: "json"}
	if prod {
		conf.Mode = "file"
		conf.Path = "logs/" + serviceName
		conf.Level = "info"
	} else {
		conf.Mode = "console"
		conf.Level = "debug"
	}
	logx.Must(logx.SetUp(conf))
}
