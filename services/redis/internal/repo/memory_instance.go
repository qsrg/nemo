// memory_instance.go 是 InstanceRepo 的内存实现（骨架/演示期默认，重启丢
// 数据）：map（ID 为键，int64）+ RWMutex 保护。
package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// memoryInstanceRepo instances 表内存实现。
type memoryInstanceRepo struct {
	mu        sync.RWMutex
	instances map[int64]Instance
	// names 是 name → id 索引（重名 Create 报错用）。
	names map[string]int64
	// counter 自增 ID（int64）。
	counter int64
}

// NewMemoryInstanceRepo 创建空的 instances 内存实现；每次调用返回独立实例。
func NewMemoryInstanceRepo() InstanceRepo {
	return &memoryInstanceRepo{instances: make(map[int64]Instance), names: make(map[string]int64)}
}

// Create 新建实例：counter 自增生成 ID（int64）与时间戳，重名报错。
func (m *memoryInstanceRepo) Create(_ context.Context, t Instance) (Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.names[t.Name]; dup {
		return Instance{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
	}
	m.counter++
	t.ID = m.counter
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	m.instances[t.ID] = t
	m.names[t.Name] = t.ID
	return t, nil
}

// Get 按 ID 取记录；未命中返回 ErrNotFound。返回副本。
func (m *memoryInstanceRepo) Get(_ context.Context, id int64) (Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.instances[id]
	if !ok {
		return Instance{}, ErrNotFound
	}
	return t, nil
}

// GetByName 按名字取记录（names 索引点查）；未命中返回 ErrNotFound。
func (m *memoryInstanceRepo) GetByName(_ context.Context, name string) (Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.names[name]
	if !ok {
		return Instance{}, ErrNotFound
	}
	return m.instances[id], nil
}

// List 全量记录（id 升序，对账端点用；与 MySQL 实现同语义）。
func (m *memoryInstanceRepo) List(_ context.Context) ([]Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Instance, 0, len(m.instances))
	for _, t := range m.instances {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Page 按可见性谓词分页查询（与 MySQL Page 同语义）：All 或 ID 集合为空
// = 空结果视角（普通用户无任何关系时列表为空，非全量）；过滤 id IN (...)，
// 按 ID 升序 OFFSET 切页，返回当页与满足谓词的总数。
func (m *memoryInstanceRepo) Page(_ context.Context, f InstancePageFilter) ([]Instance, int, error) {
	page, size := normalizePage(f.Page, f.PageSize)
	idSet := make(map[int64]bool, len(f.IDs))
	for _, id := range f.IDs {
		idSet[id] = true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	matched := make([]Instance, 0, len(m.instances))
	for _, t := range m.instances {
		if f.ClusterID > 0 && t.ClusterID != f.ClusterID {
			continue // 集群维度过滤（先选集群再出实例）
		}
		if f.All || idSet[t.ID] {
			matched = append(matched, t)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	total := len(matched)
	start := (page - 1) * size
	if start >= total {
		return []Instance{}, total, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
}

// Update 按 ID 全量覆盖其余字段（保留原 CreatedAt/CreatedBy，与 MySQL UPDATE
// 不碰 created_at/created_by 的语义一致）；未命中返回 ErrNotFound。重名
//（改成已有名）报错。
func (m *memoryInstanceRepo) Update(_ context.Context, t Instance) (Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev, ok := m.instances[t.ID]
	if !ok {
		return Instance{}, ErrNotFound
	}
	if prev.Name != t.Name {
		if _, dup := m.names[t.Name]; dup {
			return Instance{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
		}
		delete(m.names, prev.Name)
		m.names[t.Name] = t.ID
	}
	t.CreatedAt = prev.CreatedAt
	t.CreatedBy = prev.CreatedBy
	t.UpdatedAt = time.Now()
	m.instances[t.ID] = t
	return t, nil
}

// Delete 按 ID 删除；未命中返回 ErrNotFound。
func (m *memoryInstanceRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.instances[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.instances, id)
	delete(m.names, t.Name)
	return nil
}

// CountByClusterID 统计某集群下的实例数（删除集群的非空校验用）。
func (m *memoryInstanceRepo) CountByClusterID(_ context.Context, clusterID int64) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, t := range m.instances {
		if t.ClusterID == clusterID {
			n++
		}
	}
	return n, nil
}

// ClusterIDsByInstanceIDs 查给定实例 ID 集合归属的集群 ID 去重集合；ids
// 为空返回空集合（语义与 MySQL 实现一致）。
func (m *memoryInstanceRepo) ClusterIDsByInstanceIDs(_ context.Context, ids []int64) ([]int64, error) {
	idSet := make(map[int64]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[int64]bool)
	out := make([]int64, 0, 8)
	for _, t := range m.instances {
		if idSet[t.ID] && !seen[t.ClusterID] {
			seen[t.ClusterID] = true
			out = append(out, t.ClusterID)
		}
	}
	return out, nil
}
