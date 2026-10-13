// memory_menu.go 是 MenuRepo 的内存实现（断言组模式）：map（id 为键）+
// RWMutex 保护，自增分配 ID，与 MySQL 实现共享 repo_test.go 的同一组断言。
package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// memoryMenuRepo 菜单注册表内存实现。
type memoryMenuRepo struct {
	mu      sync.RWMutex
	menus   map[int64]Menu
	nextID  int64
}

// NewMemoryMenuRepo 创建空的菜单内存实现；每次调用返回独立实例。
func NewMemoryMenuRepo() MenuRepo {
	return &memoryMenuRepo{menus: make(map[int64]Menu), nextID: 1}
}

// Create 新建一行：分配自增 ID、填充 CreatedAt/UpdatedAt 后落库；返回记录副本。
func (m *memoryMenuRepo) Create(_ context.Context, menu Menu) (Menu, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	menu.ID = m.nextID
	m.nextID++
	menu.CreatedAt = now
	menu.UpdatedAt = now
	m.menus[menu.ID] = menu
	return menu, nil
}

// Update 按 ID 全量覆盖：保留 CreatedAt 与 CreateBy（与 MySQL UPDATE 不碰
// create_by 的语义一致），刷新 UpdatedAt；未命中返回 ErrNotFound。
func (m *memoryMenuRepo) Update(_ context.Context, menu Menu) (Menu, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev, ok := m.menus[menu.ID]
	if !ok {
		return Menu{}, ErrNotFound
	}
	menu.CreatedAt = prev.CreatedAt
	menu.CreateBy = prev.CreateBy
	menu.UpdatedAt = time.Now()
	m.menus[menu.ID] = menu
	return menu, nil
}

// List 返回全部记录副本，按 (sort, id) 升序。
func (m *memoryMenuRepo) List(_ context.Context) ([]Menu, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Menu, 0, len(m.menus))
	for _, menu := range m.menus {
		out = append(out, menu)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Get 按 ID 取记录；未命中返回 ErrNotFound。返回副本。
func (m *memoryMenuRepo) Get(_ context.Context, id int64) (Menu, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	menu, ok := m.menus[id]
	if !ok {
		return Menu{}, ErrNotFound
	}
	return menu, nil
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。
func (m *memoryMenuRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.menus[id]; !ok {
		return ErrNotFound
	}
	delete(m.menus, id)
	return nil
}
