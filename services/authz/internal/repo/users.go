// Package repo 提供 auth 服务的用户资料存储：按 IdP 身份（类型 + 主体）映射
// 平台用户。内存实现供骨架期/单测；MySQL 实现见 users_mysql.go（B 阶段改造 1：
// Dsn 配置非空启用，空 = 内存回退，与 authz 的 repo 装配同模式）。logic 层只依赖
// 本包定义的接口契约。
package repo

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// User 是平台用户资料（IdP 映射 + 展示信息）。
type User struct {
	// ID 存储层生成的自增主键（十进制字符串形式），无业务含义；
	// 业务身份以 (IdpType, IdpSubject) 为准（uk_idp 唯一约束）。
	ID string
	// Name 用户展示名（首次登录建档后不再随 IdP 变更覆盖）。
	Name string
	// Email 用户邮箱（attributes 缺失时回落用户名）。
	Email string
	// IdpType 身份源类型（一期固定 "cas"）。
	IdpType string
	// IdpSubject 身份源主体标识（CAS 场景即 CAS username）。
	IdpSubject string
	// OrgID 用户所属组织（authz 组织 id；来自 CAS 目录组织架构信息，
	// 目录查询透传/登录同步用；无归属为空。本地建档快照不落此字段）。
	OrgID string
	// OrgName 组织展示名（与 OrgID 同源，目录查询透传/展示用）。
	OrgName string
}

// 注：ErrNotFound 与 errors.go 的统一哨兵合并（服务合并：auth 并入 authz，
// 用户 repo 与业务 repo 共用同一哨兵，errors.Is 判定语义不变）。

// UserRepo 是用户资料 repo 契约：按 IdP 身份查取与建档。
type UserRepo interface {
	// GetByIdp 按 IdP 类型与主体取用户；未命中返回 ErrNotFound（包装后保持
	// errors.Is 可判定）。
	GetByIdp(ctx context.Context, idpType, idpSubject string) (*User, error)
	// Create 建档并返回建档后的用户；ID 由存储层生成（忽略入参 ID，自增主键），
	// create_by/update_by 由实现层填充（建档人即 IdpSubject）；同一
	// (idpType, idpSubject) 已存在时幂等返回既有记录（首次登录双提交不重复建）。
	Create(ctx context.Context, u User) (*User, error)
}

// ListUsersQuery 是用户目录查询参数（GET /v1/auth/users 的存储层投影）：
// Keyword 按 name / idp_subject 模糊（子串，大小写不敏感由实现决定）；
// Page/PageSize 为 1 起始分页（logic 层已归一化，存储层不再兜底）。
type ListUsersQuery struct {
	Keyword  string
	Page     int
	PageSize int
}

// UserDirectory 是用户目录查询契约（管理面/前端选人与姓名反查用，B 阶段
// 改造 1.2 起）：与 UserRepo 拆分为独立接口——建档路径（登录）与目录读路径
// （管理面）的消费者不同，接口最小化便于替身注入。内存与 MySQL 实现均满足。
type UserDirectory interface {
	// List 按 keyword 模糊 + 分页列出用户，返回按 IdpSubject 升序的一页
	// 与全量总数（keyword 命中口径）。
	List(ctx context.Context, q ListUsersQuery) ([]User, int, error)
	// GetBySubjects 按 IdP 主体（裸用户名，即 JWT sub 口径）批量取用户，
	// 未命中主体静默跳过（调用方回退展示裸 ID）；subjects 全部空返回空切片。
	GetBySubjects(ctx context.Context, subjects []string) ([]User, error)
}

// memUserRepo 是 UserRepo 的内存实现（单测/骨架期）。
type memUserRepo struct {
	// mu 保护 users 与 counter 的并发读写。
	mu sync.RWMutex
	// users 以 "idpType:idpSubject" 为键的用户表。
	users map[string]*User
	// counter 自增主键计数器（模拟 DB AUTO_INCREMENT，ID 生成后转字符串）。
	counter int64
}

// NewMemUserRepo 构造内存 UserRepo（同时满足 UserDirectory，可一并注入目录
// 消费方）。返回具体类型以便同一实例同时以 UserRepo / UserDirectory 两种
// 接口视图使用。
func NewMemUserRepo() *memUserRepo {
	return &memUserRepo{users: make(map[string]*User)}
}

// GetByIdp 按 IdP 身份查用户，未命中返回包装后的 ErrNotFound。
func (r *memUserRepo) GetByIdp(_ context.Context, idpType, idpSubject string) (*User, error) {
	r.mu.RLock()
	u, ok := r.users[idpKey(idpType, idpSubject)]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

// Create 建档；ID 由存储层自增生成（忽略入参 ID，与 MySQL 自增主键语义一致），
// create_by/update_by 以建档人 IdpSubject 填充；键已存在时幂等返回既有记录，
// 不覆盖（首次登录语义：展示资料以首次建档为准）。存储副本，避免调用方
// 持有引用篡改表内数据。
func (r *memUserRepo) Create(_ context.Context, u User) (*User, error) {
	key := idpKey(u.IdpType, u.IdpSubject)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.users[key]; ok {
		cp := *existing
		return &cp, nil
	}
	r.counter++
	cp := u
	cp.ID = strconv.FormatInt(r.counter, 10)
	r.users[key] = &cp
	out := cp
	return &out, nil
}

// idpKey 拼接 IdP 身份的 map 键。
func idpKey(idpType, idpSubject string) string {
	return idpType + ":" + idpSubject
}

// List 内存实现的目录查询：keyword 对 name / idp_subject 做不区分大小写的
// 子串匹配，按 IdpSubject 升序排序后切页，总数为 keyword 命中口径。
func (r *memUserRepo) List(_ context.Context, q ListUsersQuery) ([]User, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	all := make([]User, 0, len(r.users))
	for _, u := range r.users {
		if kw != "" &&
			!strings.Contains(strings.ToLower(u.Name), kw) &&
			!strings.Contains(strings.ToLower(u.IdpSubject), kw) {
			continue
		}
		all = append(all, *u)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].IdpSubject < all[j].IdpSubject })

	total := len(all)
	start := (q.Page - 1) * q.PageSize
	if start < 0 || start >= total {
		return []User{}, total, nil
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

// GetBySubjects 内存实现的批量反查：IdpSubject 精确命中即返回（跨 IdP 同
// subject 只取先建档者，语义与建档表唯一性一致），未命中主体静默跳过。
func (r *memUserRepo) GetBySubjects(_ context.Context, subjects []string) ([]User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]User, 0, len(subjects))
	for _, s := range subjects {
		for _, u := range r.users {
			if u.IdpSubject == s {
				out = append(out, *u)
				break
			}
		}
	}
	return out, nil
}
