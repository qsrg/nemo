// memory_attribute.go 是 AttributeRepo 的内存实现（Mysql.Dsn 为空时装配）：
// map[owner]map[key]value + RWMutex，与 MySQL 实现共享断言组（内存优先测试）。
package repo

import (
	"context"
	"sync"
)

// memoryAttributeRepo 属性存储内存实现。
type memoryAttributeRepo struct {
	mu   sync.RWMutex
	data map[string]map[string]string
}

// NewMemoryAttributeRepo 构造内存属性存储。
func NewMemoryAttributeRepo() AttributeRepo {
	return &memoryAttributeRepo{data: make(map[string]map[string]string)}
}

// Set 写入单条属性（幂等 upsert）。
func (m *memoryAttributeRepo) Set(_ context.Context, a Attribute) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[a.Owner] == nil {
		m.data[a.Owner] = make(map[string]string)
	}
	m.data[a.Owner][a.AttrKey] = a.AttrValue
	return nil
}

// Get 返回某 owner 的全部属性；无属性返回空 map（不返回 ErrNotFound）。
func (m *memoryAttributeRepo) Get(_ context.Context, owner string) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	attrs := m.data[owner]
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		out[k] = v
	}
	return out, nil
}

// Delete 删除单条属性（幂等：不存在不报错）。
func (m *memoryAttributeRepo) Delete(_ context.Context, owner, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if attrs := m.data[owner]; attrs != nil {
		delete(attrs, key)
	}
	return nil
}
