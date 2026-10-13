// cas_directory.go 定义 UserDirectory 的 CAS 实现（生产语义）。
//
// 背景：用户目录查询（管理面选人/姓名反查）应以 CAS 为事实源——本地 users
// 表只是登录建档快照（首次登录时从 CAS 换票写入），CAS 侧改名/新增/组织
// 调整后快照不会实时更新（早期还导入过 mysql17888...-alice 这类脏身份）。
// 本实现每次查询实时调 CAS /users（见 internal/casclient.QueryUsers），并透传
// 组织归属 OrgID（CAS 目录组织架构信息，供 authz 组织成员同步/继承判定）。
package repo

import (
	"context"

	"nemo/services/authz/internal/casclient"
)

// casDirectoryClient 是 cas_directory 依赖的最小 CAS 目录查询能力（svc 装配
// 时注入 *casclient.Client 或替身；独立接口避免 repo 反向依赖 cas 具体类型）。
type casDirectoryClient interface {
	QueryUsers(ctx context.Context, usernames []string, keyword string, page, pageSize int) (casclient.CasDirectoryResp, error)
}

// casDirectory 是 UserDirectory 的 CAS 实时实现：List/GetBySubjects 均委托
// CAS 目录查询。CAS 不可达错误原样上抛（logic 层映射 503/500），目录查询
// 非鉴权判定面，不做 fail-closed 降级。
type casDirectory struct {
	client casDirectoryClient
}

// NewCasDirectory 构造 CAS 目录实现。
func NewCasDirectory(client casDirectoryClient) UserDirectory {
	return &casDirectory{client: client}
}

// List 按 keyword 模糊 + 分页查询 CAS 目录；userId 输出 CAS username
// （IdpSubject 口径），Name 取 CAS displayName（权威展示名）。
func (d *casDirectory) List(ctx context.Context, q ListUsersQuery) ([]User, int, error) {
	resp, err := d.client.QueryUsers(ctx, nil, q.Keyword, q.Page, q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]User, 0, len(resp.Items))
	for _, u := range resp.Items {
		out = append(out, casUserToRepo(u))
	}
	return out, resp.Total, nil
}

// GetBySubjects 按 CAS username 批量反查；未命中主体静默跳过（CAS 侧过滤），
// 与本地实现的"未命中跳过"契约一致。
func (d *casDirectory) GetBySubjects(ctx context.Context, subjects []string) ([]User, error) {
	if len(subjects) == 0 {
		return nil, nil
	}
	resp, err := d.client.QueryUsers(ctx, subjects, "", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(resp.Items))
	for _, u := range resp.Items {
		out = append(out, casUserToRepo(u))
	}
	return out, nil
}

// casUserToRepo 把 CAS 目录条目映射为 repo.User：userId 口径 = CAS username
// （与登录建档的 IdpSubject 一致），展示名/邮箱取 CAS 权威值，组织归属透传。
func casUserToRepo(u casclient.CasDirectoryUser) User {
	return User{
		IdpSubject: u.Username,
		Name:       u.DisplayName,
		Email:      u.Mail,
		OrgID:      u.OrgID,
		OrgName:    u.OrgName,
	}
}
