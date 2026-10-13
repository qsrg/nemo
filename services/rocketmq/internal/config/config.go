// config.go 定义 rocketmq 独立服务的全部配置项。
// 服务形态：独立 go-zero 微服务，试点期 Topic 数据内存自管，权限判定经
// kit/authz 远程调用集中式 authz 服务（不内嵌引擎）。
package config

import (
	"time"

	"github.com/zeromicro/go-zero/rest"
)

// MysqlConf 是 rocketmq 业务库配置。Dsn 为空表示内存模式（试点默认），
// 非空时由 repo 层建立真实连接。
type MysqlConf struct {
	// Dsn MySQL 连接串；为空表示内存模式。
	Dsn string
}

// AuthzConf 是集中式授权服务（authz）接入配置，供 kit/authz Remote 使用：
// 权限判定（check/list-objects）与通用 relations（名单读/写/删）都经它转发。
type AuthzConf struct {
	// BaseURL authz 服务根地址（kit/authz Remote 的上游）。
	BaseURL string
	// Timeout 单次调用超时；非正值时 kit/authz 兜底默认 5s。
	Timeout time.Duration
}

// Config 是 rocketmq 服务的全部配置项。
type Config struct {
	rest.RestConf
	// Auth JWT 校验配置：与 authz 共享同一 AccessSecret 才能解析平台 token。
	Auth struct {
		// AccessSecret 平台 JWT 签名密钥（须与 authz Auth.AccessSecret 一致）。
		AccessSecret string
		// AccessExpire 过期秒数（保留占位，本服务不签发 token）。
		AccessExpire int64
	}
	// Mysql 业务库配置。
	Mysql MysqlConf
	// Authz 授权服务接入配置。
	Authz AuthzConf
}
