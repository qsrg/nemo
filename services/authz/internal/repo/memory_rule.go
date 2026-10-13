package repo

import (
	"context"
	"sort"
	"sync"
)

// memoryRuleRepo 是 RuleRepo 的内存实现：map 存储（id 为键，int64）+
// RWMutex 保护 + int64 自增计数器，用于骨架/演示期与单测，无外部依赖。
type memoryRuleRepo struct {
	mu      sync.RWMutex
	rules   map[int64]Rule
	counter int64
}

// NewMemoryRuleRepo 创建空的规则内存实现；每次调用返回独立实例，互不共享状态。
func NewMemoryRuleRepo() RuleRepo {
	return &memoryRuleRepo{rules: make(map[int64]Rule)}
}

// Upsert 新建或更新规则：ID=0 视为新建，counter 自增生成 ID 后回填；
// ID 非 0 视为更新，按 ID 全量覆盖（保留原 CreateBy，与 MySQL UPDATE 不碰
// create_by 的语义一致），记录不存在返回 ErrNotFound（与 MySQL 实现
// UPDATE ... WHERE id=? 的 RowsAffected=0 语义对齐——自增主键不支持由调用方
// 手工指定 ID 新建）。
func (m *memoryRuleRepo) Upsert(_ context.Context, r Rule) (Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == 0 {
		m.counter++
		r.ID = m.counter
	} else if prev, ok := m.rules[r.ID]; !ok {
		return Rule{}, ErrNotFound
	} else {
		r.CreateBy = prev.CreateBy
	}
	m.rules[r.ID] = r
	return r, nil
}

// List 返回全部规则副本，按 ID 升序保证输出确定性。
func (m *memoryRuleRepo) List(_ context.Context) ([]Rule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Rule, 0, len(m.rules))
	for _, r := range m.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Delete 按 ID 删除规则；未命中返回 ErrNotFound。
func (m *memoryRuleRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[id]; !ok {
		return ErrNotFound
	}
	delete(m.rules, id)
	return nil
}
