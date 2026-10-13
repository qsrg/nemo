package repo

import (
	"context"
	"sync"
	"time"
)

// memoryModelVersionRepo 是 ModelVersionRepo 的内存实现：map（id 为键，int64）+
// 保存顺序切片 + RWMutex 保护 + int64 自增计数器；Latest 按保存顺序取最后一条。
// 主键改造：ID 为 int64 自增主键（DB 内部序号），新增 EnginemodelID（string）
// 作为引擎返回 modelID 的镜像，应用层经此字段与引擎侧状态对应、与引擎 modelID
// 比对版本。
type memoryModelVersionRepo struct {
	mu       sync.RWMutex
	versions map[int64]ModelVersion
	order    []int64 // 保存顺序（同 ID 覆盖时移到队尾），Latest 取队尾
	counter  int64
}

// NewMemoryModelVersionRepo 创建空的模型版本内存实现；每次调用返回独立实例。
func NewMemoryModelVersionRepo() ModelVersionRepo {
	return &memoryModelVersionRepo{versions: make(map[int64]ModelVersion)}
}

// Save 保存模型版本：ID=0 时 counter 自增生成（int64），填充 CreatedAt
// （调用方已填则尊重原值）；按 EnginemodelID 去重——同 EnginemodelID 视为
// 同一版本覆盖（保留原 CreateBy，与 MySQL ODKU 不碰 create_by 的语义一致），
// 且把该 ID 移到保存顺序队尾（最新）。
func (m *memoryModelVersionRepo) Save(_ context.Context, mv ModelVersion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 按 EnginemodelID 找既有记录：若存在则复用其 ID（语义对齐 MySQL 的
	// ON DUPLICATE KEY UPDATE by engine_model_id），否则 counter 自增新建。
	for id, existing := range m.versions {
		if existing.EnginemodelID == mv.EnginemodelID && mv.EnginemodelID != "" {
			mv.ID = id
			mv.CreateBy = existing.CreateBy
			break
		}
	}
	if mv.ID == 0 {
		m.counter++
		mv.ID = m.counter
	}
	if mv.CreatedAt.IsZero() {
		mv.CreatedAt = time.Now()
	}
	if _, ok := m.versions[mv.ID]; ok {
		for i, id := range m.order {
			if id == mv.ID {
				m.order = append(m.order[:i], m.order[i+1:]...)
				break
			}
		}
	}
	m.versions[mv.ID] = mv
	m.order = append(m.order, mv.ID)
	return nil
}

// Latest 取最新保存的模型版本；无记录返回 ErrNotFound。
func (m *memoryModelVersionRepo) Latest(_ context.Context) (ModelVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.order) == 0 {
		return ModelVersion{}, ErrNotFound
	}
	return m.versions[m.order[len(m.order)-1]], nil
}
