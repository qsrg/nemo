// Package repo 定义业务库（nemo_authz_biz）四个 repo 的接口与模型。
// 本文件只固化接口契约（计划 Task 4 Interfaces 摘录），内存/MySQL 实现
// 由 Task 4 填实（internal/repo/{rule,resource_type,model_version,audit}.go）。
package repo

import (
	"context"
	"time"
)

// auditSystemUpdater 是审计条目状态变更方（committed/failed 标记）的固定
// 操作人：状态标记由写路径提交（writeWithAudit）与对账进程（reconcile）完成，
// 属系统行为，非具体用户，update_by 统一记该值（AGENTS.md 审计字段规范）。
const auditSystemUpdater = "system"

// Rule 是授权规则：角色 key 对资源前缀（Prefix）的授权，Action 空 = 服务级。
type Rule struct {
	// ID 规则 ID（自增主键，落库时由 DB 生成；Upsert 入参 ID=0 视为新建）。
	ID int64
	// RoleKey 角色 key。
	RoleKey string
	// Prefix 资源前缀（如 order:）。
	Prefix string
	// Action 动作码，空 = 服务级授权。
	Action string
	// CreateBy 创建人（JWT sub；新建时由 logic 层填充）。
	CreateBy string
	// UpdateBy 最近更新人（JWT sub；更新时由 logic 层填充）。
	UpdateBy string
}

// ResourceType 是注册的资源类型（服务名 + 支持的动作码），DslBlock 供模型装配。
type ResourceType struct {
	// ID 资源类型自增主键（落库时由 DB 生成；业务键为 Name，经唯一索引保证）。
	ID int64
	// Name 资源类型名（业务唯一键）。
	Name string
	// DslBlock 模型 DSL 中该类型对应的块（Task 6 注册机制消费）。
	DslBlock string
	// ManageCode 授权名单管理码（注册时声明，可空）：持有者对该类型
	// 任意资源拥有名单查看/写/撤销权（如 kafka-topics:manage之于 topic）。
	// 空 = 不设全权码，名单只能由资源的 owner/editor 管理。
	ManageCode string
	// CreateCode 创建码（注册时声明，可空）：创建路径自举放行依据——普通
	// 创建者对新对象无任何元组，写首个 owner 元组时若持有本码（或管理码）
	// 则放行（writerelationslogic 空对象分支）。空 = 创建入口仅管理码。
	CreateCode string
	// CreateBy 创建人（JWT sub；由 logic 层填充）。
	CreateBy string
	// UpdateBy 最近更新人（当前无更新路径，保留字段满足审计规范）。
	UpdateBy string
	// CreatedAt 创建时间。
	CreatedAt time.Time
}

// ModelVersion 是授权模型版本记录（biz 侧），用于模型变更比对与回滚决策。
// 主键改造：ID 为 BIGINT 自增主键（DB 内部序号，不暴露给业务）；
// EnginemodelID 存 OpenFGA 引擎返回的字符串 modelID（唯一键，与引擎侧状态对应）。
type ModelVersion struct {
	// ID 模型版本自增主键（落库时由 DB 生成；Save 入参 ID=0 视为新建）。
	ID int64
	// EnginemodelID 引擎返回的 modelID 字符串（应用层与引擎侧状态对应的桥梁）。
	EnginemodelID string
	// Dsl 模型 DSL 全文。
	Dsl string
	// CreateBy 创建人（JWT sub；由 logic 层填充）。
	CreateBy string
	// UpdateBy 最近更新人（幂等覆盖路径更新；由 logic 层填充）。
	UpdateBy string
	// CreatedAt 创建时间。
	CreatedAt time.Time
}

// AuditStatus 是审计条目状态：pending（已落库未提交）| committed（已提交引擎）| failed。
type AuditStatus string

// 审计条目三态常量。
const (
	// AuditPending 已落库、尚未提交引擎。
	AuditPending AuditStatus = "pending"
	// AuditCommitted 已成功提交引擎。
	AuditCommitted AuditStatus = "committed"
	// AuditFailed 提交引擎失败（附原因，待对账重放）。
	AuditFailed AuditStatus = "failed"
)

// AuditEntry 是审计先行模式的条目：先落 pending，提交引擎成功后置 committed。
type AuditEntry struct {
	// ID 审计条目自增主键（落库时由 DB 生成；CreatePending 入参 ID=0 视为新建）。
	ID int64
	// Operator 操作人（用户 ID，JWT sub）。
	Operator string
	// Action 动作（如 create_org）。
	Action string
	// Target 操作对象。
	Target string
	// Snapshot 操作时的快照（JSON 文本）。
	Snapshot string
	// Status 状态（pending|committed|failed）。
	Status AuditStatus
	// CreateBy 创建人 = Operator 冗余（满足审计字段规范，落库时填充）。
	CreateBy string
	// UpdateBy 状态变更方：CreatePending 置 ''；MarkCommitted/MarkFailed 由
	// 实现置 auditSystemUpdater（状态标记属系统行为）。
	UpdateBy string
	// CreatedAt 创建时间。
	CreatedAt time.Time
	// UpdatedAt 状态变更时间（MarkCommitted/MarkFailed 时由实现刷新；
	// CreatePending 时与 CreatedAt 相同）。
	UpdatedAt time.Time
}

// AuditFilter 是审计查询过滤条件。
type AuditFilter struct {
	// Operator 按操作人过滤（空 = 不过滤）。
	Operator string
	// ExcludeSystem 排除系统操作人（system）条目——定时任务（如 CAS 组织
	// 同步）每周期落一条审计，默认排除防止刷屏淹没人工操作。
	ExcludeSystem bool
	// Page 页码，从 1 开始。
	Page int
	// PageSize 每页条数。
	PageSize int
}

// RuleRepo 是规则表的存取接口。
type RuleRepo interface {
	// Upsert 新建或更新规则：ID=0 视为新建，DB 自增生成 ID 后回填，
	// create_by 取 r.CreateBy；ID 非 0 按主键覆盖更新，update_by 取 r.UpdateBy。
	// 返回落库后的规则（含 DB 生成的 ID）。
	Upsert(ctx context.Context, r Rule) (Rule, error)
	// List 列出全部规则。
	List(ctx context.Context) ([]Rule, error)
	// Delete 按 ID 删除规则。
	Delete(ctx context.Context, id int64) error
}

// ResourceTypeRepo 是资源类型注册表的存取接口。
type ResourceTypeRepo interface {
	// Create 注册资源类型（重名幂等或报错由实现决定，Task 4 填实）；
	// create_by 取 rt.CreateBy。
	Create(ctx context.Context, rt ResourceType) (ResourceType, error)
	// Get 按名取资源类型。
	Get(ctx context.Context, name string) (ResourceType, error)
	// List 列出全部资源类型。
	List(ctx context.Context) ([]ResourceType, error)
	// Update 更新资源类型的 DSL 块与名单管理码（类型名不可改，按 name 定位）；
	// update_by 取 rt.UpdateBy，updated_at 置当前时间。
	Update(ctx context.Context, rt ResourceType) error
	// Delete 按名删除资源类型；未命中返回 ErrNotFound。
	Delete(ctx context.Context, name string) error
}

// ModelVersionRepo 是模型版本的存取接口。
type ModelVersionRepo interface {
	// Latest 取最新模型版本（无记录时由实现返回语义错误，Task 4 填实）。
	Latest(ctx context.Context) (ModelVersion, error)
	// Save 保存模型版本：ID=0 视为新建，DB 自增生成 ID 后回填；
	// EnginemodelID 必填（引擎返回的 modelID，经唯一索引去重）；
	// create_by/update_by 取 mv 对应字段。
	Save(ctx context.Context, mv ModelVersion) error
}

// AuditRepo 是审计表的存取接口（审计先行 + 定时对账）。
type AuditRepo interface {
	// CreatePending 写入一条 pending 审计并返回其 ID（DB 自增生成）；
	// create_by 取 e.CreateBy（为空时实现回落 e.Operator）。
	CreatePending(ctx context.Context, e AuditEntry) (int64, error)
	// MarkCommitted 把审计条目标记为 committed（update_by 置 auditSystemUpdater）。
	MarkCommitted(ctx context.Context, id int64) error
	// MarkFailed 把审计条目标记为 failed（附原因；update_by 置 auditSystemUpdater）。
	MarkFailed(ctx context.Context, id int64, reason string) error
	// ListPending 列出早于 olderThan 仍 pending 的条目（对账重放用）。
	// 排序契约（两实现一致，钉死）：按 (created_at, id) 升序——对账重放
	// 按落库顺序回放，与 Query 的倒序展示语义区分。
	ListPending(ctx context.Context, olderThan time.Duration) ([]AuditEntry, error)
	// Query 按过滤条件分页查询审计，返回条目列表与总数。
	Query(ctx context.Context, filter AuditFilter) ([]AuditEntry, int, error)
}
