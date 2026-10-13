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

	kiterrors "nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListOrgsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListOrgsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListOrgsLogic 构造业务规则层实例。
func NewListOrgsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListOrgsLogic {
	return &ListOrgsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListOrgs 组织列表：合并两处来源——
//  1. 注册表元数据（API 创建的组织，带名称/父组织）；
//  2. 引擎元组中出现的 org:* 对象（如 bootstrap 直写引擎的 org:root，
//     未走 API 注册表，名称缺省用 ID，parent 从引擎 parent 元组推导）。
//
// 按 ID 升序去重输出；引擎读失败按 503 fail-closed 返回。
func (l *ListOrgsLogic) ListOrgs() (*types.OrgListResp, error) {
	byID := map[string]types.OrgResp{}

	// 来源一：注册表元数据。
	infos, err := l.svcCtx.Orgs.List(l.ctx)
	if err != nil {
		logx.Errorf("组织列表注册表读失败: %v", err)
		return nil, kiterrors.Internal("AUTHZ_INTERNAL", "查询组织列表失败")
	}
	for _, o := range infos {
		byID[o.ID] = types.OrgResp{Id: o.ID, Name: o.Name, ParentId: o.ParentID}
	}

	// 来源二：引擎元组里以 org: 为 object 的组织（全量读后本地过滤）。
	tuples, err := l.svcCtx.Engine.ReadTuples(l.ctx, "", "")
	if err != nil {
		return nil, engineErrTo503(err)
	}
	parentOf := map[string]string{}
	for _, t := range tuples {
		if !strings.HasPrefix(t.Object, "org:") {
			continue
		}
		orgID := strings.TrimPrefix(t.Object, "org:")
		if _, ok := byID[orgID]; !ok {
			// 引擎可见但未注册的组织：名称缺省用 ID。
			byID[orgID] = types.OrgResp{Id: orgID, Name: orgID, ParentId: ""}
		}
		if t.Relation == "parent" && strings.HasPrefix(t.User, "org:") {
			parentOf[orgID] = strings.TrimPrefix(t.User, "org:")
		}
	}
	// 引擎 parent 元组优先于注册表（引擎是关系判定的事实来源）。
	for id, p := range parentOf {
		o := byID[id]
		o.ParentId = p
		byID[id] = o
	}

	items := make([]types.OrgResp, 0, len(byID))
	for _, o := range byID {
		items = append(items, o)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Id < items[j].Id })
	return &types.OrgListResp{Items: items, Total: len(items)}, nil
}
