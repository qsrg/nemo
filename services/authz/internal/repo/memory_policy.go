// memory_policy.go 是 PolicyRepo 的内存实现（Mysql.Dsn 为空时装配）：
// slice + RWMutex，ID 自增，与 MySQL 实现共享断言组。
package repo

import (
	"context"
	"sync"
)

// memoryPolicyRepo 策略存储内存实现。
type memoryPolicyRepo struct {
	mu    sync.RWMutex
	seq   int64
	items []Policy
}

// NewMemoryPolicyRepo 构造内存策略存储。
func NewMemoryPolicyRepo() PolicyRepo {
	return &memoryPolicyRepo{}
}

// Create 新建策略（ID 自增）。
func (m *memoryPolicyRepo) Create(_ context.Context, p Policy) (Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	p.ID = m.seq
	m.items = append(m.items, p)
	return p, nil
}

// List 返回全部策略（按 ID 升序）。
func (m *memoryPolicyRepo) List(_ context.Context) ([]Policy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Policy, len(m.items))
	copy(out, m.items)
	return out, nil
}

// Get 按 ID 查单条；未命中返回 ErrNotFound。
func (m *memoryPolicyRepo) Get(_ context.Context, id int64) (Policy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.items {
		if p.ID == id {
			return p, nil
		}
	}
	return Policy{}, ErrNotFound
}

// Update 按 ID 全量覆盖；未命中返回 ErrNotFound。
func (m *memoryPolicyRepo) Update(_ context.Context, p Policy) (Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == p.ID {
			p.CreatedAt = m.items[i].CreatedAt
			m.items[i] = p
			return p, nil
		}
	}
	return Policy{}, ErrNotFound
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。
func (m *memoryPolicyRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}
