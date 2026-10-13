// memory_topic.go 是 TopicRepo 的内存实现（试点：map + RWMutex，重启丢
// 数据；预置 6 条演示 Topic 供权限链路验证）。
package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"nemo/services/rocketmq/internal/types"
)

// memoryTopicRepo Topic 表内存实现。
type memoryTopicRepo struct {
	mu      sync.RWMutex
	topics  map[int64]types.RocketmqTopic
	names   map[string]int64
	counter int64
}

// NewMemoryTopicRepo 创建带种子数据的内存实现；每次调用返回独立实例。
func NewMemoryTopicRepo() TopicRepo {
	r := &memoryTopicRepo{
		topics: make(map[int64]types.RocketmqTopic),
		names:  make(map[string]int64),
	}
	// 种子数据（演示：全部挂默认集群 id=1，供集群级可见性验证）。
	seed := []types.RocketmqTopic{
		{Name: "ops-alerts", Env: "prod", Description: "运维告警主题", ClusterId: 1, OrgId: "2"},
		{Name: "dev-internal", Env: "dev", Description: "开发内部事件", ClusterId: 1, OrgId: "3"},
		{Name: "verify-org-1", Env: "staging", Description: "组织1验证主题", ClusterId: 1, OrgId: "1"},
		{Name: "pagedemo-01", Env: "prod", Description: "分页演示1", ClusterId: 1, OrgId: "2"},
		{Name: "pagedemo-02", Env: "prod", Description: "分页演示2", ClusterId: 1, OrgId: "2"},
		{Name: "pagedemo-03", Env: "dev", Description: "分页演示3", ClusterId: 1, OrgId: "3"},
	}
	for _, t := range seed {
		_, _ = r.Create(context.Background(), t)
	}
	return r
}

// Create 新建 Topic：counter 自增生成 ID 与时间戳，重名报错。
func (m *memoryTopicRepo) Create(_ context.Context, t types.RocketmqTopic) (types.RocketmqTopic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.names[t.Name]; dup {
		return types.RocketmqTopic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
	}
	m.counter++
	t.Id = m.counter
	m.topics[t.Id] = t
	m.names[t.Name] = t.Id
	return t, nil
}

// Get 按 ID 取记录；未命中返回 ErrNotFound。返回副本。
func (m *memoryTopicRepo) Get(_ context.Context, id int64) (types.RocketmqTopic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.topics[id]
	if !ok {
		return types.RocketmqTopic{}, ErrNotFound
	}
	return t, nil
}

// GetByName 按名字取记录；未命中返回 ErrNotFound。
func (m *memoryTopicRepo) GetByName(_ context.Context, name string) (types.RocketmqTopic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.names[name]
	if !ok {
		return types.RocketmqTopic{}, ErrNotFound
	}
	return m.topics[id], nil
}

// ListByCluster 返回某集群下的记录副本，按 name 升序。
func (m *memoryTopicRepo) ListByCluster(_ context.Context, clusterId int64) ([]types.RocketmqTopic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.RocketmqTopic, 0, len(m.topics))
	for _, t := range m.topics {
		if t.ClusterId == clusterId {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// List 返回全部记录副本，按 name 升序。
func (m *memoryTopicRepo) List(_ context.Context) ([]types.RocketmqTopic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.RocketmqTopic, 0, len(m.topics))
	for _, t := range m.topics {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Update 按 ID 覆盖 env/description（与 MySQL 版语义一致：cluster_id/org_id
// 等归属与身份字段不可改，保留原值）；未命中返回 ErrNotFound。重名报错。
func (m *memoryTopicRepo) Update(_ context.Context, t types.RocketmqTopic) (types.RocketmqTopic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.topics[t.Id]
	if !ok {
		return types.RocketmqTopic{}, ErrNotFound
	}
	if otherID, dup := m.names[t.Name]; dup && otherID != t.Id {
		return types.RocketmqTopic{}, fmt.Errorf("%w: %s", ErrDuplicate, t.Name)
	}
	t.CreatedBy = old.CreatedBy
	t.ClusterId = old.ClusterId
	t.OrgId = old.OrgId
	delete(m.names, old.Name)
	m.topics[t.Id] = t
	m.names[t.Name] = t.Id
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
