package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// memoryRepo 是 ItemRepo 的内存实现：map 存储 + RWMutex 保护 +
// int64 自增 id（与 DB 自增主键语义一致，不再生成 it-N 字符串前缀），
// 用于骨架/演示期与单测，无外部依赖。
type memoryRepo struct {
	mu      sync.RWMutex
	items   map[int64]Item
	counter int64
}

// NewMemory 创建空的内存实现；每次调用返回独立实例，互不共享状态。
func NewMemory() ItemRepo {
	return &memoryRepo{items: make(map[int64]Item)}
}

// Create 自增生成 id 并填充创建/更新时间与审计字段（CreateBy 取入参，缺省
// '' = 未知操作人）后落内存；忽略入参 ID（自增主键语义）。返回完整记录副本。
func (m *memoryRepo) Create(_ context.Context, it Item) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	now := time.Now()
	stored := Item{
		ID:        m.counter,
		Name:      it.Name,
		Desc:      it.Desc,
		CreateBy:  it.CreateBy,
		UpdateBy:  it.UpdateBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.items[stored.ID] = stored
	return stored, nil
}

// Get 按 id 读取记录；未命中返回 ErrNotFound。返回副本，避免内部状态外泄。
func (m *memoryRepo) Get(_ context.Context, id int64) (Item, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	it, ok := m.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	return it, nil
}

// List 返回全部记录副本，按 id 升序保证输出确定性。
func (m *memoryRepo) List(_ context.Context) ([]Item, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Item, 0, len(m.items))
	for _, it := range m.items {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Update 按 it.ID 覆盖 Name/Desc 并刷新 UpdatedAt 与 UpdateBy（CreatedAt/
// CreateBy 保留原值，与 MySQL UPDATE 不碰 created_at/created_by 语义一致）；
// 未命中返回 ErrNotFound。
func (m *memoryRepo) Update(_ context.Context, it Item) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.items[it.ID]
	if !ok {
		return Item{}, ErrNotFound
	}
	stored.Name = it.Name
	stored.Desc = it.Desc
	stored.UpdateBy = it.UpdateBy
	stored.UpdatedAt = time.Now()
	m.items[it.ID] = stored
	return stored, nil
}

// Delete 按 id 删除记录；未命中返回 ErrNotFound。
func (m *memoryRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return ErrNotFound
	}
	delete(m.items, id)
	return nil
}
