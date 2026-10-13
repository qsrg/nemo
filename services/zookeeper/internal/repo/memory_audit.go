// memory_audit.go 是 AuditRepo 的内存实现（骨架/演示期默认，重启丢数据）。
package repo

import (
	"context"
	"sync"
	"time"
)

// memoryAuditRepo 审计表内存实现。
type memoryAuditRepo struct {
	mu     sync.RWMutex
	audits map[int64]AuditEntry
	// counter 自增 ID（int64）。
	counter int64
}

// NewMemoryAuditRepo 创建空的审计内存实现；每次调用返回独立实例。
func NewMemoryAuditRepo() AuditRepo {
	return &memoryAuditRepo{audits: make(map[int64]AuditEntry)}
}

// CreatePending 写入一条 pending 审计并返回自增 ID；create_by 为空时回落 operator。
func (m *memoryAuditRepo) CreatePending(_ context.Context, e AuditEntry) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	e.ID = m.counter
	if e.CreateBy == "" {
		e.CreateBy = e.Operator
	}
	e.Status = AuditPending
	now := time.Now()
	e.CreatedAt = now
	e.UpdatedAt = now
	m.audits[e.ID] = e
	return e.ID, nil
}

// MarkCommitted 把审计条目标记为 committed（update_by 置系统标识）；未命中返回 ErrNotFound。
func (m *memoryAuditRepo) MarkCommitted(_ context.Context, id int64) error {
	return m.mark(id, AuditCommitted, "")
}

// MarkFailed 把审计条目标记为 failed（附原因）；未命中返回 ErrNotFound。
func (m *memoryAuditRepo) MarkFailed(_ context.Context, id int64, reason string) error {
	return m.mark(id, AuditFailed, reason)
}

// mark 是状态标记的公共实现：仅 pending 态可迁移（幂等护栏），更新时间与
// update_by 一并刷新。
func (m *memoryAuditRepo) mark(id int64, st AuditStatus, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.audits[id]
	if !ok {
		return ErrNotFound
	}
	e.Status = st
	e.Reason = reason
	e.UpdateBy = auditSystemUpdater
	e.UpdatedAt = time.Now()
	m.audits[id] = e
	return nil
}

// ListFailedTargets 查指定动作集合下 status=failed 的去重 target 集合
//（内存实现，与 MySQL 语义一致）。
func (m *memoryAuditRepo) ListFailedTargets(_ context.Context, actions []string) (map[string]bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	want := make(map[string]bool, len(actions))
	for _, a := range actions {
		want[a] = true
	}
	out := make(map[string]bool)
	for _, e := range m.audits {
		if e.Status == AuditFailed && want[e.Action] {
			out[e.Target] = true
		}
	}
	return out, nil
}
