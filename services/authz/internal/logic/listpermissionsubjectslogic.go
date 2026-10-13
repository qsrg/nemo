// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"sort"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListPermissionSubjectsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListPermissionSubjectsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListPermissionSubjectsLogic 构造业务规则层实例。
func NewListPermissionSubjectsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPermissionSubjectsLogic {
	return &ListPermissionSubjectsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListPermissionSubjects 反向查询某权限码的直接授权主体（管理面审计用）：
//   - permission 必填且合法（400 AUTHZ_PERMISSION_INVALID），支持通配码
//     （如 *:read，对象为 permission:*.read）；
//   - ReadTuples(permObject(code), "can") 读 can 元组，主体归一化：
//     user:{id} → {kind:user, id}；org:{id}#member → {kind:org, id}；
//     其余形态（异常元组）跳过；
//   - 仅返回直接授权主体——组织主体经 org#member userset 继承的成员不在
//     此展开（前端对 org 主体可另行展开组织成员），避免反向视图体积膨胀；
//   - 无授权主体 → 200 空数组（非 null）；引擎失败 → 503 fail-closed。
func (l *ListPermissionSubjectsLogic) ListPermissionSubjects(req *types.ListPermissionSubjectsReq) (*types.ListPermissionSubjectsResp, error) {
	code := strings.TrimSpace(req.Permission)
	if !validPermissionCode(code) {
		return nil, errors.BadRequest("AUTHZ_PERMISSION_INVALID", "权限码不合法")
	}

	tuples, err := l.svcCtx.Engine.ReadTuples(l.ctx, permObject(code), "can")
	if err != nil {
		l.Logger.Errorf("反查授权主体失败 permission=%s: %v", code, err)
		return nil, errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
	}

	subjects := make([]types.PermissionSubject, 0, len(tuples))
	for _, t := range tuples {
		switch {
		case strings.HasPrefix(t.User, "user:"):
			subjects = append(subjects, types.PermissionSubject{Kind: "user", Id: strings.TrimPrefix(t.User, "user:")})
		case strings.HasPrefix(t.User, "org:") && strings.HasSuffix(t.User, "#member"):
			id := strings.TrimPrefix(strings.TrimSuffix(t.User, "#member"), "org:")
			subjects = append(subjects, types.PermissionSubject{Kind: "org", Id: id})
		}
	}
	// 稳定排序：user 在前、org 在后，同类型按 id 升序（展示与断言稳定）。
	sort.SliceStable(subjects, func(i, j int) bool {
		wi, wj := 0, 0
		if subjects[i].Kind == "org" {
			wi = 1
		}
		if subjects[j].Kind == "org" {
			wj = 1
		}
		if wi != wj {
			return wi < wj
		}
		return subjects[i].Id < subjects[j].Id
	})

	return &types.ListPermissionSubjectsResp{Subjects: subjects}, nil
}
