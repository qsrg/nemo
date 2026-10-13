// apikeys.go 提供 auth 服务的 API key 落存：只存 SHA-256 摘要，key 原文
// 不落存（原文仅在签发响应里一次性出现，服务端任何位置都拿不到它）。
// 骨架期/单测用内存实现，MySQL 等持久化实现后续任务接入（接口契约由本包
// 定义，logic 层只依赖 ApiKeyRepo 接口）。
package repo

import (
	"context"
	"sync"
)

// ApiKey 是签发的 API key 落存记录：原文 key 不进本结构（编译期保证
// 不可能误存明文），只存 SHA-256(key) 的 hex 摘要。
type ApiKey struct {
	// ID key 记录唯一标识（后续管理/吊销用）。
	ID string
	// Name key 名称（备注用途，签发时必填）。
	Name string
	// Prefixes 允许访问的资源前缀白名单（scope，permission-design §5.4；
	// 一期只存不消费）。
	Prefixes []string
	// KeyHash SHA-256(key) 的 hex 摘要——原文不可反查。
	KeyHash string
}

// ApiKeyRepo 是 API key 存储 repo 契约：一期仅签发落存（列表/撤销为 spec
// 排除项，后续任务接入时再扩接口）。
type ApiKeyRepo interface {
	// Create 落存一条 key 记录并返回落存结果（存储副本，调用方持有的
	// 切片/结构体事后修改不影响表内数据）。
	Create(ctx context.Context, k ApiKey) (*ApiKey, error)
}

// memApiKeyRepo 是 ApiKeyRepo 的内存实现（单测/骨架期）。
type memApiKeyRepo struct {
	// mu 保护 apiKeys 的并发读写。
	mu sync.RWMutex
	// apiKeys 以 ID 为键的 key 记录表。
	apiKeys map[string]*ApiKey
}

// NewMemApiKeyRepo 构造内存 ApiKeyRepo。
func NewMemApiKeyRepo() ApiKeyRepo {
	return &memApiKeyRepo{apiKeys: make(map[string]*ApiKey)}
}

// Create 落存记录：深拷贝入参（Prefixes 切片复制副本），返回值也是副本，
// 避免调用方持有引用篡改表内数据。
func (r *memApiKeyRepo) Create(_ context.Context, k ApiKey) (*ApiKey, error) {
	cp := k
	cp.Prefixes = append([]string(nil), k.Prefixes...)

	r.mu.Lock()
	r.apiKeys[k.ID] = &cp
	r.mu.Unlock()

	out := cp
	out.Prefixes = append([]string(nil), cp.Prefixes...)
	return &out, nil
}
