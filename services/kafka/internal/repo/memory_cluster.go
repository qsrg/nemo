// memory_cluster.go 是 ClusterRepo 的内存实现（骨架/演示模式）：map（ID 为键）
// + RWMutex 保护，code 唯一索引；与 MySQL 实现共享同一组接口契约。
package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// memoryClusterRepo 集群内存实现。
type memoryClusterRepo struct {
	mu       sync.RWMutex
	clusters map[int64]Cluster
	// codes 是 code → id 索引（重名 Create 报错用）。
	codes map[string]int64
	// counter 自增 ID（int64）。
	counter int64
}

// NewMemoryClusterRepo 创建空的集群内存实现；每次调用返回独立实例。
func NewMemoryClusterRepo() ClusterRepo {
	return &memoryClusterRepo{clusters: make(map[int64]Cluster), codes: make(map[string]int64)}
}

// Create 新建集群：counter 自增生成 ID 与时间戳，code 冲突报 ErrDuplicate。
func (m *memoryClusterRepo) Create(_ context.Context, c Cluster) (Cluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.codes[c.Code]; dup {
		return Cluster{}, ErrDuplicate
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

// Update 更新集群属性（code 不可改）；未命中返回 ErrNotFound。
func (m *memoryClusterRepo) Update(_ context.Context, c Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.clusters[c.ID]
	if !ok {
		return ErrNotFound
	}
	c.Code = old.Code // 对象身份不可变
	c.CreatedAt = old.CreatedAt
	c.CreatedBy = old.CreatedBy
	c.UpdatedAt = time.Now()
	m.clusters[c.ID] = c
	return nil
}

// Delete 删除集群；未命中返回 ErrNotFound。
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

// GetById 按 ID 取单条；未命中返回 ErrNotFound。
func (m *memoryClusterRepo) GetById(_ context.Context, id int64) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clusters[id]
	if !ok {
		return Cluster{}, ErrNotFound
	}
	return c, nil
}

// GetByCode 按集群编码取单条；未命中返回 ErrNotFound。
func (m *memoryClusterRepo) GetByCode(_ context.Context, code string) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.codes[code]
	if !ok {
		return Cluster{}, ErrNotFound
	}
	return m.clusters[id], nil
}

// Page 分页查询：按 id 降序（新集群在前），返回当前页记录与总数。
func (m *memoryClusterRepo) Page(_ context.Context, page, pageSize int) ([]Cluster, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	ids := make([]int64, 0, len(m.clusters))
	for id := range m.clusters {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	start := (page - 1) * pageSize
	if start >= len(ids) {
		return []Cluster{}, int64(len(ids)), nil
	}
	end := start + pageSize
	if end > len(ids) {
		end = len(ids)
	}
	items := make([]Cluster, 0, end-start)
	for _, id := range ids[start:end] {
		items = append(items, m.clusters[id])
	}
	return items, int64(len(ids)), nil
}
