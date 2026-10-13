// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"strconv"

	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"nemo/kit/errors"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListRulesLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListRulesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListRulesLogic 构造业务规则层实例。
func NewListRulesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRulesLogic {
	return &ListRulesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListRules 列出全部规则（RoleRepo.List 已按 ID 升序保证输出确定性）。
func (l *ListRulesLogic) ListRules() (*types.RuleListResp, error) {
	rules, err := l.svcCtx.RuleRepo.List(l.ctx)
	if err != nil {
		return nil, errors.Internal("AUTHZ_INTERNAL", "查询规则失败")
	}
	items := make([]types.RuleResp, 0, len(rules))
	for _, r := range rules {
		items = append(items, types.RuleResp{
			Id:      strconv.FormatInt(r.ID, 10),
			RoleKey: r.RoleKey,
			Prefix:  r.Prefix,
			Action:  r.Action,
		})
	}
	return &types.RuleListResp{Items: items, Total: len(items)}, nil
}
