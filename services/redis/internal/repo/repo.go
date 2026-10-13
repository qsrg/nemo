// repo.go 定义 redis 独立服务的存储层基础类型：审计（AuditEntry/AuditRepo）
// 接口与公共错误哨兵。实现层分内存/MySQL 双实现。审计表 audits 与 kafka/authz
// 同构（独立记账），写操作"审计先行 + 状态标记"语义一致。
//
// 范围说明：redis 服务无对账/审计查询端点，AuditRepo 仅承载 kit/audit.Sink
// 所需的三方法（CreatePending/MarkCommitted/MarkFailed）——不为想象的需求
// 预留接口（KISS），后续需要时再扩。
package repo

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 是查询/删除未命中目标的统一哨兵错误（errors.Is 判等）。
var ErrNotFound = errors.New("repo: not found")

// ErrDuplicate 是唯一键冲突（如实例重名）的统一哨兵错误——替代创建路径的
// 全表预检，由 DB 唯一键兜底判定。
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
	// Action 动作（如 create_instance）。
	Action string
	// Target 操作对象。
	Target string
	// Snapshot 操作时的快照（JSON 文本）。
	Snapshot string
	// Status 状态（pending|committed|failed）。
	Status AuditStatus
	// Reason 失败原因（仅 failed 态有值）。
	Reason string
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

// AuditRepo 是审计表的存取接口（审计先行，kit/audit.Sink 的三方法子集）。
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
}
