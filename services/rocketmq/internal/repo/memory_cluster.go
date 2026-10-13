// memory_cluster.go 是 ClusterRepo 的内存实现（演示用：Dsn 为空时回退，
// 重启丢数据；预置 default 集群供 topic 挂载）。
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
	mu       sync.RWMutex
	clusters map[int64]Cluster
	codes    map[string]int64
	counter  int64
}

// NewMemoryClusterRepo 创建带 default 集群的内存实现；每次调用返回独立实例。
func NewMemoryClusterRepo() ClusterRepo {
	r := &memoryClusterRepo{
		clusters: make(map[int64]Cluster),
		codes:    make(map[string]int64),
	}
	_, _ = r.Create(context.Background(), Cluster{
		Code: "default", Remark: "默认集群（内存回退自动创建）",
		CreatedBy: "system", UpdateBy: "system",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	return r
}

// Create 新建集群：counter 自增生成 ID，code 重复报错。
func (m *memoryClusterRepo) Create(_ context.Context, c Cluster) (Cluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.codes[c.Code]; dup {
		return Cluster{}, fmt.Errorf("%w: %s", ErrClusterDuplicate, c.Code)
	}
	m.counter++
	c.ID = m.counter
	m.clusters[c.ID] = c
	m.codes[c.Code] = c.ID
	return c, nil
}

// GetById 按 ID 取记录；未命中返回 ErrClusterNotFound。
func (m *memoryClusterRepo) GetById(_ context.Context, id int64) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clusters[id]
	if !ok {
		return Cluster{}, ErrClusterNotFound
	}
	return c, nil
}

// GetByCode 按 code 取记录；未命中返回 ErrClusterNotFound。
func (m *memoryClusterRepo) GetByCode(_ context.Context, code string) (Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.codes[code]
	if !ok {
		return Cluster{}, ErrClusterNotFound
	}
	return m.clusters[id], nil
}

// List 返回全部记录副本，按 id 升序。
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

// Delete 按 ID 删除；未命中返回 ErrClusterNotFound。
func (m *memoryClusterRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clusters[id]
	if !ok {
		return ErrClusterNotFound
	}
	delete(m.clusters, id)
	delete(m.codes, c.Code)
	return nil
}
