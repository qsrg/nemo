// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：ServiceContext 显式装配依赖，无 DI 容器；repo/client 等依赖接口
// 在此构造注入，供 handler/logic 通过字段访问。

package svc

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"

	"nemo/services/authz/internal/casclient"
	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/middleware"
	"nemo/services/authz/internal/repo"
)

// ServiceContext 聚合服务全部依赖：config、内嵌授权引擎与业务库 repo。
type ServiceContext struct {
	Config    config.Config
	AdminRole rest.Middleware
	// Engine 是内嵌 OpenFGA 引擎句柄（判定面经 go-sdk HTTP client 访问）。
	Engine *engine.Handle
	// 四个业务 repo：Mysql.Dsn 为空时装配内存实现（骨架/演示期默认），
	// 非空时装配 MySQL 实现（Task 4 起），logic 层统一依赖接口。
	RuleRepo         repo.RuleRepo         // 规则表（服务:动作 → 角色映射）
	ResourceTypeRepo repo.ResourceTypeRepo // 资源类型注册表
	ModelVersionRepo repo.ModelVersionRepo // 授权模型版本记录
	AuditRepo        repo.AuditRepo        // 审计先行条目
	// Orgs 是组织元数据注册表（组织 ID → 名称/父组织，Task 5 起管理 API 使用；
	// 引擎元组只存关系不存展示名）。Phase A 为内存实现，MySQL 落库 Task 8 接入。
	Orgs repo.OrgStore
	// MenuRepo 是菜单注册表（Phase C 改造①：me/menus 过滤 + 管理面 CRUD/seed）。
	MenuRepo repo.MenuRepo
	// AttributeRepo 是鉴权属性存储（ABAC 方案 A：主体/资源属性统一管理，
	// 配合 ABAC deny 策略评估；内存/MySQL 双实现）。
	AttributeRepo repo.AttributeRepo
	// PolicyRepo 是 ABAC deny 策略存储（方案 A：策略管理 + 鉴权叠加评估）。
	PolicyRepo repo.PolicyRepo
	// Users 是用户档案 repo（IdP 身份 → 平台用户映射；CAS 首登建档）。
	// 服务合并：auth 的用户链路并入 authz，与业务 repo 同按 Mysql.Dsn 装配。
	Users repo.UserRepo
	// Directory 是用户目录查询（/v1/auth/users 数据面，CAS 为事实源）。
	Directory repo.UserDirectory
	// ApiKeys 是 API key 存储 repo（签发落存，只存 SHA-256 摘要）。
	ApiKeys repo.ApiKeyRepo
	// CasClient 是 CAS 协议客户端（票据校验 + 用户目录查询）。
	CasClient CasClient
	// userNames 是 CAS 用户显示名索引（username → displayName）：orgsync
	// 每次全量同步后整体重建（atomic.Value 存不可变 map，读侧 Load 快照，
	// 无锁并发安全）；members 等接口经 UserDisplayName 查询，未同步/未知
	// 用户返回空串。索引只存显示名，不承载任何授权语义。
	userNames atomic.Value
	// mysqlClose 关闭业务库连接池（仅 MySQL 模式非 nil），Stop 时统一释放。
	mysqlClose func()
}

// CasClient 是 CAS 协议客户端契约（票据换用户信息 + 用户目录查询），由
// internal/casclient.Client 实现；logic 层依赖本接口以便单测注入替身。
type CasClient interface {
	// ValidateTicket 用 service ticket 向 CAS 换取登录用户信息。
	ValidateTicket(ctx context.Context, ticket, service string) (casclient.CasUser, error)
	// QueryUsers 实时查询 CAS 用户目录（用户信息+组织归属，目录查询事实源）。
	QueryUsers(ctx context.Context, usernames []string, keyword string, page, pageSize int) (casclient.CasDirectoryResp, error)
}

// SetUserNames 整体替换用户显示名索引（orgsync 同步完成路径调用；传 nil 清空）。
func (s *ServiceContext) SetUserNames(m map[string]string) {
	s.userNames.Store(m)
}

// UserDisplayName 按用户名查显示名（索引未就绪/未知返回空串，调用方回退展示用户名）。
func (s *ServiceContext) UserDisplayName(username string) string {
	m, _ := s.userNames.Load().(map[string]string)
	if m == nil {
		return ""
	}
	return m[username]
}

// NewServiceContext 根据 config 显式构造并注入全部依赖（无 DI 容器）：
// 进程内启动 OpenFGA 引擎（Datastore=memory，MySQL 后端 Task 8 填实），
// 装配四个业务 repo（Mysql.Dsn 空 = 内存实现，非空 = MySQL 实现）；
// 启动期依赖不可用必须快速失败，不吞错——引擎启动或业务库连接失败均 panic。
func NewServiceContext(c config.Config) *ServiceContext {
	h, err := engine.Start(context.Background(), engine.Options{
		HTTPAddr:  c.Engine.HTTPAddr,
		StoreName: c.Engine.StoreName,
		Datastore: c.Engine.Datastore,
		MySQLDSN:  c.Engine.MySQLDSN,
	})
	if err != nil {
		panic(err)
	}
	sc := &ServiceContext{
		Config: c,
		// 管理面鉴权（去角色化后）：引擎实时 Check(can, permission:*.*)，
		// 撤权即时生效，不再读 claims.roles（config.AdminRoles 已停用仅留兼容）。
		AdminRole: middleware.NewAdminRoleMiddleware(h).Handle,
		Engine:    h,
		Orgs:      repo.NewMemoryOrgStore(),
		// 登录链路依赖（服务合并后统一装配）：CAS 客户端 + API key repo +
		// 用户档案 repo（Directory 一律以 CAS 为事实源，见下）。
		CasClient: casclient.NewClient(c.Cas.BaseURL, c.Cas.ValidatePath, c.Cas.InsecureSkipVerify),
		ApiKeys:   repo.NewMemApiKeyRepo(),
	}
	if c.Mysql.Dsn == "" {
		sc.RuleRepo = repo.NewMemoryRuleRepo()
		sc.ResourceTypeRepo = repo.NewMemoryResourceTypeRepo()
		sc.ModelVersionRepo = repo.NewMemoryModelVersionRepo()
		sc.AuditRepo = repo.NewMemoryAuditRepo()
		sc.MenuRepo = repo.NewMemoryMenuRepo()
		sc.AttributeRepo = repo.NewMemoryAttributeRepo()
		sc.PolicyRepo = repo.NewMemoryPolicyRepo()
		sc.Users = repo.NewMemUserRepo()
		sc.Directory = repo.NewCasDirectory(sc.CasClient)
		logx.Info("业务库内存模式（Mysql.Dsn 为空），用户目录查询走 CAS")
	} else {
		repos, err := repo.NewMySQLRepos(c.Mysql.Dsn)
		if err != nil {
			// 引擎已启动，panic 前先停掉，避免监听端口泄漏。
			h.Stop()
			panic(fmt.Errorf("业务库 MySQL 连接失败: %w", err))
		}
		sc.RuleRepo = repos.RuleRepo
		sc.ResourceTypeRepo = repos.ResourceTypeRepo
		sc.ModelVersionRepo = repos.ModelVersionRepo
		sc.AuditRepo = repos.AuditRepo
		sc.MenuRepo = repos.MenuRepo
		sc.AttributeRepo = repos.AttributeRepo
		sc.PolicyRepo = repos.PolicyRepo
		// 组织元数据注册表 MySQL 落库（004_orgs.sql）：MySQL 模式下管理 API
		// 与 CAS 同步的组织名称/父组织持久化，不再随重启丢失。
		sc.Orgs = repos.OrgStore
		// 用户档案 repo 与业务库同一 Dsn（库合并：nemo_authz 单一库）：
		// 连接池独立（sql.DB 与 gorm 各自持有），Stop 时一并释放。
		ur, err := repo.NewMysqlUserRepo(c.Mysql.Dsn)
		if err != nil {
			h.Stop()
			panic(fmt.Errorf("用户库 MySQL 连接失败: %w", err))
		}
		sc.Users = ur
		sc.Directory = repo.NewCasDirectory(sc.CasClient)
		closeFn := sc.mysqlClose
		sc.mysqlClose = func() {
			if closeFn != nil {
				closeFn()
			}
			_ = ur.Close()
		}
		logx.Info("业务库 MySQL 模式（库合并：用户表/业务表/引擎同库）")
	}
	return sc
}

// Stop 关闭 ServiceContext 持有的进程内引擎与业务库连接池（均幂等），
// 供 main defer 调用。
func (sc *ServiceContext) Stop() {
	if sc.Engine != nil {
		sc.Engine.Stop()
	}
	if sc.mysqlClose != nil {
		sc.mysqlClose()
	}
}

// ReadyHandler 是 /readyz 的就绪探针：引擎 HTTP 端口 dial 成功才 200，
// 失败返回 503（engine.Handle.Ping 短超时拨号，Stop 后端口关闭即 503）。
func (sc *ServiceContext) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if sc.Engine == nil || sc.Engine.Ping() != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
