// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"net/http"
	"strings"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// ListUsersLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListUsersLogic 构造业务规则层实例。
func NewListUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListUsersLogic {
	return &ListUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// listUsersPageSizeMax 是分页查询的每页上限（防止无界拉全表）。
const listUsersPageSizeMax = 100

// ListUsers 用户目录查询（管理面：选人/姓名反查的唯一事实源，B 阶段改造 1.2）：
//   - ids 非空：按裸用户名（JWT sub 口径，即 idp_subject）批量反查，跳过分页，
//     未命中主体静默跳过（前端回退展示裸 ID）；
//   - 否则 keyword 分页：name / idp_subject 子串模糊，page 缺省 1、pageSize
//     缺省 20、上限 100；
//   - 响应 userId 一律输出裸用户名（idp_subject）——内部 User.ID 的 "user:"
//     前缀不外泄（authz 引擎侧由调用方自行拼 "user:"+userId）；
//   - 存储故障 → 500 AUTH_INTERNAL（读路径不做 fail-closed 降级：目录查询
//     非鉴权判定面，错误如实上抛）。
func (l *ListUsersLogic) ListUsers(req *types.ListUsersReq) (*types.UserListResp, error) {
	// ids 批量反查路径：逗号分隔裸用户名，去空去重保序。
	if idsRaw := strings.TrimSpace(req.Ids); idsRaw != "" {
		seen := map[string]bool{}
		subjects := make([]string, 0, 16)
		for _, s := range strings.Split(idsRaw, ",") {
			s = strings.TrimSpace(s)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			subjects = append(subjects, s)
		}
		users, err := l.svcCtx.Directory.GetBySubjects(l.ctx, subjects)
		if err != nil {
			l.Logger.Errorf("用户目录批量反查失败 ids=%q: %v", idsRaw, err)
			return nil, kiterrors.New(http.StatusInternalServerError, "AUTH_INTERNAL", "用户目录查询失败")
		}
		// 命中条目按请求 ids 顺序输出（调用方逐个取名的场景顺序更可预期）。
		bySubject := make(map[string]repo.User, len(users))
		for _, u := range users {
			bySubject[u.IdpSubject] = u
		}
		items := make([]types.DirectoryUserResp, 0, len(subjects))
		for _, s := range subjects {
			if u, ok := bySubject[s]; ok {
				items = append(items, types.DirectoryUserResp{UserId: u.IdpSubject, Name: u.Name, Email: u.Email, OrgId: u.OrgID, OrgName: u.OrgName})
			}
		}
		return &types.UserListResp{Items: items, Total: len(items)}, nil
	}

	// keyword 分页路径：page/pageSize 归一化（下限与上限兜底）。
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > listUsersPageSizeMax {
		pageSize = listUsersPageSizeMax
	}

	users, total, err := l.svcCtx.Directory.List(l.ctx, repo.ListUsersQuery{
		Keyword:  req.Keyword,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		l.Logger.Errorf("用户目录分页查询失败 keyword=%q: %v", req.Keyword, err)
		return nil, kiterrors.New(http.StatusInternalServerError, "AUTH_INTERNAL", "用户目录查询失败")
	}
	items := make([]types.DirectoryUserResp, 0, len(users))
	for _, u := range users {
		items = append(items, types.DirectoryUserResp{UserId: u.IdpSubject, Name: u.Name, Email: u.Email, OrgId: u.OrgID, OrgName: u.OrgName})
	}
	return &types.UserListResp{Items: items, Total: total}, nil
}
