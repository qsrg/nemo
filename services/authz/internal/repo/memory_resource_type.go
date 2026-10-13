package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// memoryResourceTypeRepo 是 ResourceTypeRepo 的内存实现：map（name 为键）+
// RWMutex 保护 + int64 自增计数器；name 即业务唯一键（重名 Create 报错），
// ID 为内部自增主键（与 MySQL 实现的 id BIGINT AUTO_INCREMENT 对齐）。
type memoryResourceTypeRepo struct {
	mu      sync.RWMutex
	types   map[string]ResourceType
	counter int64
}

// NewMemoryResourceTypeRepo 创建空的资源类型内存实现；每次调用返回独立实例。
func NewMemoryResourceTypeRepo() ResourceTypeRepo {
	return &memoryResourceTypeRepo{types: make(map[string]ResourceType)}
}

// Create 注册资源类型：counter 自增生成 ID（int64），填充 CreatedAt（调用方
// 已填则尊重原值），重名报错；返回落库后的完整记录副本。
func (m *memoryResourceTypeRepo) Create(_ context.Context, rt ResourceType) (ResourceType, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.types[rt.Name]; ok {
		return ResourceType{}, fmt.Errorf("资源类型已存在: %s", rt.Name)
	}
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = time.Now()
	}
	m.counter++
	rt.ID = m.counter
	m.types[rt.Name] = rt
	return rt, nil
}

// Get 按名取资源类型；未命中返回 ErrNotFound。返回副本，避免内部状态外泄。
func (m *memoryResourceTypeRepo) Get(_ context.Context, name string) (ResourceType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rt, ok := m.types[name]
	if !ok {
		return ResourceType{}, ErrNotFound
	}
	return rt, nil
}

// List 返回全部资源类型副本，按 name 升序保证输出确定性。
func (m *memoryResourceTypeRepo) List(_ context.Context) ([]ResourceType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ResourceType, 0, len(m.types))
	for _, rt := range m.types {
		out = append(out, rt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Update 更新资源类型的 DSL 块与名单管理码：按 name 定位（类型名不可改），
// 未命中返回 ErrNotFound；保留 ID/CreatedAt/CreateBy。
func (m *memoryResourceTypeRepo) Update(_ context.Context, rt ResourceType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.types[rt.Name]
	if !ok {
		return ErrNotFound
	}
	rt.ID = old.ID
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = old.CreatedAt
	}
	rt.CreateBy = old.CreateBy
	m.types[rt.Name] = rt
	return nil
}

// Delete 按名删除资源类型；未命中返回 ErrNotFound。
func (m *memoryResourceTypeRepo) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.types[name]; !ok {
		return ErrNotFound
	}
	delete(m.types, name)
	return nil
}
