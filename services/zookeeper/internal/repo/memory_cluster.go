// memory_cluster.go 是 ClusterRepo 的内存实现（骨架/演示期默认，重启丢数据）：
// map（ID 为键）+ code 索引 + RWMutex 保护，与 memory_instance.go 同模式。
package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// memoryClusterRepo clusters 表内存实现。
type memoryClusterRepo struct {
	mu      sync.RWMutex
	clusters map[int64]Cluster
	// codes 是 code → id 索引（重码 Create 报错用）。
	codes map[string]int64
	// counter 自增 ID（int64）。
	counter int64
}

// NewMemoryClusterRepo 创建空的 clusters 内存实现；每次调用返回独立实例。
func NewMemoryClusterRepo() ClusterRepo {
	return &memoryClusterRepo{clusters: make(map[int64]Cluster), codes: make(map[string]int64)}
}

// Create 新建集群：counter 自增生成 ID（int64）与时间戳，code 重复报错。
func (m *memoryClusterRepo) Create(_ context.Context, c Cluster) (Cluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.codes[c.Code]; dup {
		return Cluster{}, fmt.Errorf("%w: %s", ErrDuplicate, c.Code)
	}
	m.counter++
	c.ID = m.counter
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	m.clusters[c.ID] = c
	m.codes[c.Code] = c.ID
	return c, nil
}

// GetById 按 ID 取记录；未命中返回 ErrNotFound。返回副本。
func (m *memoryClusterRepo) GetById(_ context.Context, id int64) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clusters[id]
	if !ok {
		return Cluster{}, ErrNotFound
	}
	return c, nil
}

// GetByCode 按 code 取记录（codes 索引点查）；未命中返回 ErrNotFound。
func (m *memoryClusterRepo) GetByCode(_ context.Context, code string) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.codes[code]
	if !ok {
		return Cluster{}, ErrNotFound
	}
	return m.clusters[id], nil
}

// List 全量列表，按 ID 升序（与 MySQL 实现同语义）。
func (m *memoryClusterRepo) List(_ context.Context) ([]Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Cluster, 0, len(m.clusters))
	for _, c := range m.clusters {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。集群下是否仍有实例由 logic 层校验。
func (m *memoryClusterRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clusters[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.clusters, id)
	delete(m.codes, c.Code)
	return nil
}
