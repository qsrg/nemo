// repo.go 定义 kafka 独立服务的存储层基础类型：Topic 存取接口与审计
// （AuditEntry/AuditRepo）接口、公共错误哨兵。实现层分内存/MySQL 双实现，
// 共享同一组接口契约。审计复用 authz 的审计表（nemo_authz_biz.audits），
// 写操作"审计先行 + 状态标记"语义与 authz 一致。
package repo

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 是查询/删除未命中目标的统一哨兵错误（errors.Is 判等）。
var ErrNotFound = errors.New("repo: not found")

// ErrDuplicate 是唯一键冲突（如 Topic 重名）的统一哨兵错误——替代创建路径
// 的全表预检（几十万量级下 List 预检是全表扫），由 DB 唯一键兜底判定。
var ErrDuplicate = errors.New("repo: duplicate")

// auditSystemUpdater 是审计条目状态变更方（committed/failed 标记）的固定
// 标识（AGENTS.md 审计字段规范：状态标记属系统行为，不冒充用户操作）。
const auditSystemUpdater = "system"

// AuditStatus 是审计条目状态：pending（已落库未提交）| committed（已提交引擎）| failed。
type AuditStatus string

const (
	// AuditPending 已落库、尚未提交引擎。
	AuditPending AuditStatus = "pending"
	// AuditCommitted 已成功提交引擎。
	AuditCommitted AuditStatus = "committed"
	// AuditFailed 提交引擎失败（附原因，待对账重放）。
	AuditFailed AuditStatus = "failed"
)

// AuditEntry 是一条审计记录。
type AuditEntry struct {
	// ID 审计条目自增主键（落库时由 DB 生成；CreatePending 入参 ID=0 视为新建）。
	ID int64
	// Operator 操作人（用户 ID，JWT sub）。
	Operator string
	// Action 动作（如 create_topic）。
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

// AuditFilter 是审计查询过滤条件（kafka 服务当前不暴露审计查询接口，
// 保留 Query 仅为接口完整性；如后续需要可接 authz /v1/admin/audit）。
type AuditFilter struct {
	// Operator 按操作人过滤（空 = 不过滤）。
	Operator string
	// ExcludeSystem 排除系统操作人（system）条目。
	ExcludeSystem bool
	// Page 页码，从 1 开始。
	Page int
	// PageSize 每页条数。
	PageSize int
}

// AuditRepo 是审计表的存取接口（审计先行）。
type AuditRepo interface {
	// CreatePending 写入一条 pending 审计并返回其 ID（DB 自增生成）；
	// create_by 取 e.CreateBy（为空时实现回落 e.Operator）。
	CreatePending(ctx context.Context, e AuditEntry) (int64, error)
	// MarkCommitted 把审计条目标记为 committed（update_by 置 auditSystemUpdater）。
	MarkCommitted(ctx context.Context, id int64) error
	// MarkFailed 把审计条目标记为 failed（附原因；update_by 置 auditSystemUpdater）。
	MarkFailed(ctx context.Context, id int64, reason string) error
	// ListFailedTargets 查指定动作集合下 status=failed 的去重 target 集合
	//（授权对账用：failed 的创建审计意味着业务行已被回滚或待人工清理，
	// 对账不得为其补写 owner——否则回滚失败的脏行被洗白成有效资源）。
	ListFailedTargets(ctx context.Context, actions []string) (map[string]bool, error)
	// ListPending 列出早于 olderThan 仍 pending 的条目（对账重放用）。
	ListPending(ctx context.Context, olderThan time.Duration) ([]AuditEntry, error)
	// Query 按过滤条件分页查询审计，返回条目列表与总数。
	Query(ctx context.Context, filter AuditFilter) ([]AuditEntry, int, error)
}
