// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package config

import (
	"time"

	"github.com/zeromicro/go-zero/rest"
)

// MysqlConf 是关系库配置（goctl 模板无此扩展点，按 plan 要求在生成物上显式补充；
// 命名化类型供 svc 装配层引用，避免匿名 struct 无法在包外表达）。
type MysqlConf struct {
	// Dsn MySQL 连接串；为空表示内存模式（骨架/演示期默认），非空时由 repo 层建立真实连接。
	Dsn string
}

// AuthzConf 是集中式授权服务（authz）接入配置，供 kit/authz Require 中间件使用。
type AuthzConf struct {
	// BaseURL authz 服务根地址（kit/authz Remote 的上游）。
	BaseURL string
	// Timeout 单次判定调用超时（go-zero conf 支持 "5s" 等时长写法）；非正值时
	// kit/authz 兜底默认 5s。
	Timeout time.Duration
}

// AuthConf 是平台 JWT 校验配置（统一鉴权）：与 authz 共享同一 AccessSecret
// 才能解析平台签发的 token（Identity 中间件按此 secret 解析 claims.sub）。
type AuthConf struct {
	// AccessSecret 平台 JWT 签名密钥（须与 authz Auth.AccessSecret 一致）。
	AccessSecret string
	// AccessExpire 过期秒数（保留占位，本服务不签发 token）。
	AccessExpire int64
}

// Config 是 sample 服务的全部配置项。
type Config struct {
	rest.RestConf
	// Auth 平台 JWT 校验配置（统一鉴权，Identity 中间件使用）。
	Auth AuthConf
	// Mysql 关系库配置。
	Mysql MysqlConf
	// Authz 授权服务接入配置（Require 冒烟路由使用）。
	Authz AuthzConf
}
