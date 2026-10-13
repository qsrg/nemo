// memory_topic.go 是 TopicRepo 的内存实现（Task 4 断言组模式）：map（ID 为键，
// int64）+ RWMutex 保护，与 MySQL 实现共享 repo_test.go 的同一组断言。
package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// memoryTopicRepo Topic 表内存实现。
type memoryTopicRepo struct {
	mu     sync.RWMutex
	topics map[int64]Topic
	// names 是 name → id 索引（重名 Create 报错用）。
	names map[string]int64
	// counter 自增 ID（int64）。
	counter int64
}

// NewMemoryTopicRepo 创建空的 Topic 内存实现；每次调用返回独立实例。
func NewMemoryTopicRepo() TopicRepo {
	return &memoryTopicRepo{topics: make(map[int64]Topic), names: make(map[string]int64)}
}

// Create 新建 Topic：counter 自增生成 ID（int64）与时间戳，重名报错。
// 应用层不再调用 newResourceID（rand hex 已下线）。
func (m *memoryTopicRepo) Create(_ context.Context, t Topic) (Topic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.names[t.Name]; dup {
		return Topic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
	}
	m.counter++
	t.ID = m.counter
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	m.topics[t.ID] = t
	m.names[t.Name] = t.ID
	return t, nil
}

// Get 按 ID 取记录；未命中返回 ErrNotFound。返回副本。
func (m *memoryTopicRepo) Get(_ context.Context, id int64) (Topic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.topics[id]
	if !ok {
		return Topic{}, ErrNotFound
	}
	return t, nil
}

// GetByName 按名字取记录（names 索引点查）；未命中返回 ErrNotFound。
func (m *memoryTopicRepo) GetByName(_ context.Context, name string) (Topic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.names[name]
	if !ok {
		return Topic{}, ErrNotFound
	}
	return m.topics[id], nil
}

// List 返回全部记录副本，按 name 升序。
func (m *memoryTopicRepo) List(_ context.Context) ([]Topic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Topic, 0, len(m.topics))
	for _, t := range m.topics {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Page 按可见性谓词分页查询（与 MySQL Page 同语义）：All 或两路谓词皆空
// = 全量视角；过滤 org_id IN (...) OR id IN (...)，按 ID 升序 OFFSET 切页，
// 返回当页与满足谓词的总数。
func (m *memoryTopicRepo) Page(_ context.Context, f TopicPageFilter) ([]Topic, int, error) {
	page, size := normalizePage(f.Page, f.PageSize)
	orgSet := make(map[string]bool, len(f.OrgIDs))
	for _, id := range f.OrgIDs {
		orgSet[id] = true
	}
	idSet := make(map[int64]bool, len(f.IDs))
	for _, id := range f.IDs {
		idSet[id] = true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	matched := make([]Topic, 0, len(m.topics))
	for _, t := range m.topics {
		if f.All || orgSet[t.OrgID] || idSet[t.ID] {
			matched = append(matched, t)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	total := len(matched)
	start := (page - 1) * size
	if start >= total {
		return []Topic{}, total, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
}

// Update 按 ID 全量覆盖其余字段（保留原 CreatedAt/CreatedBy，与 MySQL UPDATE
// 不碰 created_at/created_by 的语义一致）；未命中返回 ErrNotFound。重名
// （改成已有名）报错。
func (m *memoryTopicRepo) Update(_ context.Context, t Topic) (Topic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev, ok := m.topics[t.ID]
	if !ok {
		return Topic{}, ErrNotFound
	}
	if prev.Name != t.Name {
		if _, dup := m.names[t.Name]; dup {
			return Topic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		delete(m.names, prev.Name)
		m.names[t.Name] = t.ID
	}
	t.CreatedAt = prev.CreatedAt
	t.CreatedBy = prev.CreatedBy
	t.UpdatedAt = time.Now()
	m.topics[t.ID] = t
	return t, nil
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。
func (m *memoryTopicRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.topics[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.topics, id)
	delete(m.names, t.Name)
	return nil
}
