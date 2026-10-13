// org.go 定义组织元数据注册表：组织 ID → 名称/父组织（引擎元组只存关系，
// 不存展示名，管理 API 的组织元数据落这里）。Phase A 提供内存实现，
// MySQL 落库随 Task 8 数据后端一并接入（接口契约此处冻结）。
package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// OrgInfo 是组织元数据：引擎里 org:{ID} 对象的展示信息。
type OrgInfo struct {
	// ID 组织 ID。
	ID string
	// Name 组织名称。
	Name string
	// ParentID 父组织 ID，空表示根组织。
	ParentID string
	// CreateBy 创建操作人（管理 API 为 JWT sub；CAS 同步为 system）。
	CreateBy string
	// UpdateBy 最近更新操作人（口径同 CreateBy）。
	UpdateBy string
	// CreatedAt 创建时间。
	CreatedAt time.Time
}

// OrgStore 是组织元数据注册表的存取接口。
type OrgStore interface {
	// Upsert 新建或更新组织元数据（按 ID 判定），返回落库后的条目。
	Upsert(ctx context.Context, o OrgInfo) (OrgInfo, error)
	// Get 按 ID 取组织元数据，未命中返回 ErrNotFound。
	Get(ctx context.Context, id string) (OrgInfo, error)
	// Delete 按 ID 删除组织元数据，未命中返回 ErrNotFound。
	Delete(ctx context.Context, id string) error
	// List 列出全部组织元数据（按 ID 升序）。
	List(ctx context.Context) ([]OrgInfo, error)
}

// memoryOrgStore 是 OrgStore 的内存实现：map（id 为键）+ RWMutex 保护。
type memoryOrgStore struct {
	mu    sync.RWMutex
	orgs  map[string]OrgInfo
	nowFn func() time.Time
}

// NewMemoryOrgStore 创建空的组织注册表内存实现；每次调用返回独立实例。
func NewMemoryOrgStore() OrgStore {
	return &memoryOrgStore{
		orgs:  make(map[string]OrgInfo),
		nowFn: time.Now,
	}
}

// Upsert 新建或更新组织元数据（按 ID 判定，ID 为空条目由调用方保证不出现，
// 本实现不做二次校验）；CreatedAt 为零值时填充当前时间，返回落库后的条目。
func (m *memoryOrgStore) Upsert(_ context.Context, o OrgInfo) (OrgInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o.CreatedAt.IsZero() {
		o.CreatedAt = m.nowFn()
	}
	m.orgs[o.ID] = o
	return o, nil
}

// Get 按 ID 取组织元数据；未命中返回 ErrNotFound。
func (m *memoryOrgStore) Get(_ context.Context, id string) (OrgInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orgs[id]
	if !ok {
		return OrgInfo{}, ErrNotFound
	}
	return o, nil
}

// Delete 按 ID 删除组织元数据；未命中返回 ErrNotFound。
func (m *memoryOrgStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orgs[id]; !ok {
		return ErrNotFound
	}
	delete(m.orgs, id)
	return nil
}

// List 列出全部组织元数据，按 ID 升序（与 MySQL 实现 ORDER BY id 一致）。
func (m *memoryOrgStore) List(_ context.Context) ([]OrgInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]OrgInfo, 0, len(m.orgs))
	for _, o := range m.orgs {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
