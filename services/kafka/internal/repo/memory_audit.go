package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// memoryAuditRepo 是 AuditRepo 的内存实现：map（id 为键，int64）+ RWMutex 保护 +
// int64 自增计数器；MarkFailed 的 reason 存于独立 map（AuditEntry 模型无
// 该字段，仅与 MySQL 落库语义对齐用）。
type memoryAuditRepo struct {
	mu      sync.RWMutex
	entries map[int64]AuditEntry
	reasons map[int64]string
	counter int64
}

// NewMemoryAuditRepo 创建空的审计内存实现；每次调用返回独立实例。
func NewMemoryAuditRepo() AuditRepo {
	return &memoryAuditRepo{
		entries: make(map[int64]AuditEntry),
		reasons: make(map[int64]string),
	}
}

// CreatePending 写入一条 pending 审计：counter 自增生成 ID（int64），Status
// 强制置 pending（忽略入参状态），CreatedAt 填充，返回条目 ID。
func (m *memoryAuditRepo) CreatePending(_ context.Context, e AuditEntry) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	e.ID = m.counter
	e.Status = AuditPending
	e.CreatedAt = time.Now()
	m.entries[e.ID] = e
	return e.ID, nil
}

// MarkCommitted 把审计条目置为 committed（update_by 置 auditSystemUpdater，
// 与 MySQL markStatus 语义一致）；未命中返回 ErrNotFound。
func (m *memoryAuditRepo) MarkCommitted(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return ErrNotFound
	}
	e.Status = AuditCommitted
	e.UpdateBy = auditSystemUpdater
	e.UpdatedAt = time.Now()
	m.entries[id] = e
	return nil
}

// MarkFailed 把审计条目置为 failed 并记录原因（update_by 置 auditSystemUpdater）；
// 未命中返回 ErrNotFound。
func (m *memoryAuditRepo) MarkFailed(_ context.Context, id int64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return ErrNotFound
	}
	e.Status = AuditFailed
	e.UpdateBy = auditSystemUpdater
	e.UpdatedAt = time.Now()
	m.entries[id] = e
	m.reasons[id] = reason
	return nil
}

// ListPending 列出创建时间早于（now - olderThan）且仍 pending 的条目；
// 排序按（CreatedAt, ID）升序（与 MySQL 的 ORDER BY created_at ASC, id ASC
// 一致，对账重放按落库顺序回放）；olderThan 为负时分界线落在未来，
// 即取全部 pending（测试用，规避秒精度）。
func (m *memoryAuditRepo) ListPending(_ context.Context, olderThan time.Duration) ([]AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cutoff := time.Now().Add(-olderThan)
	out := make([]AuditEntry, 0)
	for _, e := range m.entries {
		if e.Status == AuditPending && e.CreatedAt.Before(cutoff) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Query 按过滤条件分页查询审计：Operator 非空时按操作人过滤，按
// （CreatedAt, ID）倒序（新→旧），返回当前页条目与符合条件的总数。
// Page < 1 视为第 1 页；PageSize < 1 视为默认 20。
func (m *memoryAuditRepo) Query(_ context.Context, filter AuditFilter) ([]AuditEntry, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	matched := make([]AuditEntry, 0)
	for _, e := range m.entries {
		if filter.Operator != "" && e.Operator != filter.Operator {
			continue
		}
		if filter.ExcludeSystem && e.Operator == auditSystemUpdater {
			continue
		}
		matched = append(matched, e)
	}
	sortAuditEntries(matched)
	page, size := normalizePage(filter.Page, filter.PageSize)
	start := (page - 1) * size
	if start >= len(matched) {
		return []AuditEntry{}, len(matched), nil
	}
	end := start + size
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], len(matched), nil
}

// sortAuditEntries 按（CreatedAt, ID）倒序排列，与 MySQL 实现的
// ORDER BY created_at DESC, id DESC 语义一致。
func sortAuditEntries(entries []AuditEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].ID > entries[j].ID
	})
}

// normalizePage 规整分页参数：Page < 1 → 1，PageSize < 1 → 默认 20，
// 上限 100（防一次拉全表——Topic Page 与审计查询共用）。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
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
	for _, e := range m.entries {
		if e.Status == AuditFailed && want[e.Action] {
			out[e.Target] = true
		}
	}
	return out, nil
}
