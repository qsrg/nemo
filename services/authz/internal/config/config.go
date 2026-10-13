// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package config

import (
	"time"

	"github.com/zeromicro/go-zero/rest"
)

// MysqlConf 是业务库（nemo_authz_biz：规则/资源类型/模型版本/审计）配置。
// Dsn 为空表示内存模式（骨架/演示期默认），Task 4 填实 MySQL repo。
type MysqlConf struct {
	// Dsn MySQL 连接串；为空表示内存模式，非空时由 repo 层建立真实连接。
	Dsn string
}

// EngineConf 是内嵌 OpenFGA 引擎配置（回环 HTTP，datastore 本期默认 memory）。
type EngineConf struct {
	// HTTPAddr 引擎 HTTP 监听地址（回环），如 127.0.0.1:18084。
	HTTPAddr string
	// StoreName 默认授权 Store 的名称（EnsureModel 按名幂等创建）。
	StoreName string
	// Datastore 数据存储后端：memory 或 mysql（mysql 为持久化后端，Task 8 填实）。
	Datastore string
	// MySQLDSN 引擎 MySQL 连接串（Datastore 为 mysql 时使用，指向 nemo_authz 引擎库）。
	MySQLDSN string
	// ModelFile 是基础模型 DSL 文件路径（相对进程工作目录，默认 model.fga）。
	// 资源类型注册的合并基准：基础模型 + 全部已注册类型块（Task 6 起）。
	ModelFile string
}

// CasConf 是 CAS 服务端配置（登录/回调换用户信息；服务合并后 auth 的
// 登录链路并入 authz，原 auth 配置同构迁移）。
type CasConf struct {
	// BaseURL CAS 服务基地址（本地 CAS 容器明文 HTTP 回环地址）。
	BaseURL string
	// ValidatePath 票据校验路径（CAS p3 协议 serviceValidate；空回落 /serviceValidate）。
	ValidatePath string
	// Service 登录回调服务地址：发起登录重定向时作为 service 参数拼入 CAS
	// 登录页 URL；须与 callback 请求携带的 service 一致（CAS 校验票据与
	// service 匹配）。
	Service string
	// InsecureSkipVerify 跳过 CAS 证书校验（仅 dev 自签证书场景，生产必须为 false）。
	InsecureSkipVerify bool
}

// ReconcileConf 是审计对账定时任务配置（main 内 Ticker 驱动）。
type ReconcileConf struct {
	// Interval 对账轮询间隔（go-zero conf 支持 "1m" 等时长写法）；
	// 非正值时 main 兜底 1m。
	Interval time.Duration
	// OlderThan pending 超时分界（早于该时长的 pending 才入扫）；
	// 非正值时 main 兜底 5m。
	OlderThan time.Duration
}

// CasSyncConf 是 CAS 组织架构同步配置（internal/orgsync 全量对账，main 内
// Ticker 驱动 + 管理面手动触发接口）。生产语义：组织架构由 CAS 统一维护并
// 对外提供 /orgs、/users 目录接口，本系统只做同步副本落授权引擎。
type CasSyncConf struct {
	// BaseURL 是 CAS 服务基地址（与 auth 服务 Cas.BaseURL 同源；mock 为
	// 明文 http://localhost:8443）。
	BaseURL string
	// Timeout 是单次目录请求超时；非正值时代码兜底 5s。
	Timeout time.Duration
	// InsecureSkipVerify 生产 CAS https 自签场景跳过证书校验（明文 HTTP 无影响）。
	InsecureSkipVerify bool `json:",optional"`
	// Enabled 定时同步开关；false 时不启 Ticker（手动触发接口仍可用）。
	Enabled bool `json:",optional"`
	// Interval 同步周期（即最大撤权滞后）；非正值时代码兜底 5m。
	Interval time.Duration
}

// BootstrapConf 是幂等引导（超管自举）配置：服务初始化时声明初始超管，
// 解决"admin 无码进不了授权页、没人能给 admin 授权"的引导死锁。
// 引导只负责初始装配（WriteTuples 读后写幂等）；超管变更走控制台授权页，
// 本配置的移除不会自动回收旧元组。
type BootstrapConf struct {
	// AdminUsers 超管用户登录名（如 admin）。引导时对每个用户授
	// permission:*.* 全通配权限并挂根组织 member。为空时默认 ["admin"]
	// （向后兼容：未声明配置的环境保持 admin 为初始超管）。
	AdminUsers []string
	// AdminOrgs 超管组织编码（如 root/ops）。引导时对每个组织的成员集合
	// （org:{编码}#member）授 permission:*.*——组织下全部成员继承超管。
	// 为空时不下发组织维度超管。
	AdminOrgs []string
}

// Config 是 authz 服务的全部配置项。
type Config struct {
	rest.RestConf
	// Auth 是 JWT 鉴权配置（go-zero jwt 中间件）。
	Auth struct {
		// AccessSecret 签名密钥；etc 默认为 dev 值，真实 secret 生产由环境变量注入。
		AccessSecret string
		// AccessExpire token 有效期（秒）。
		AccessExpire int64
	}
	// Engine 是内嵌 OpenFGA 引擎配置。
	Engine EngineConf
	// Mysql 是业务库配置（规则/资源类型/模型版本/审计）。
	Mysql MysqlConf
	// Bootstrap 是初始超管声明配置（见 BootstrapConf）。
	Bootstrap BootstrapConf
	// AdminRoles 是历史遗留配置（deprecated）：去角色化改造后超管鉴权统一
	// 走引擎 Check(can, permission:*.*)，本配置已停用，仅为兼容既有 yaml/
	// 测试保留（不再被任何逻辑读取）。
	AdminRoles []string
	// Reconcile 是审计对账定时任务配置。
	Reconcile ReconcileConf
	// CasSync 是 CAS 组织架构同步配置。
	CasSync CasSyncConf
	// Cas 是 CAS 服务端配置（登录票据校验；auth 并入后的登录链路消费）。
	Cas CasConf
}
