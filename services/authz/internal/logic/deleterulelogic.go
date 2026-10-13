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
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	stderrors "errors"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteRuleLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteRuleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteRuleLogic 构造业务规则层实例。
func NewDeleteRuleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteRuleLogic {
	return &DeleteRuleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteRule 删除规则：
//   - 校验（写路径前置，失败不落审计）：操作人必在；ID 须为正整数；规则必须
//     已存在（不存在 404——删除属于参数态校验，在审计先行写路径之前完成）；
//   - 写路径（审计先行）：业务表 Delete；repo 失败 → MarkFailed + 503。
//
// 主键改造：rules.id 为 int64 自增主键，API 入参字符串入口转 int64。
func (l *DeleteRuleLogic) DeleteRule(req *types.RuleIdReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}

	id, err := strconv.ParseInt(strings.TrimSpace(req.Id), 10, 64)
	if err != nil || id <= 0 {
		return errors.BadRequest("AUTHZ_RULE_INVALID", "规则 ID 非法")
	}
	rules, err := l.svcCtx.RuleRepo.List(l.ctx)
	if err != nil {
		return engineErrTo503(err)
	}
	exists := false
	for _, r := range rules {
		if r.ID == id {
			exists = true
			break
		}
	}
	if !exists {
		return errors.NotFound("AUTHZ_RULE_NOT_FOUND", "规则不存在")
	}

	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_rule", "rule:"+strconv.FormatInt(id, 10), req, func() error {
		if err := l.svcCtx.RuleRepo.Delete(l.ctx, id); err != nil {
			// 并发下已被删除视为已达成（幂等），其余错误按写路径失败处理。
			if stderrors.Is(err, repo.ErrNotFound) {
				return nil
			}
			return err
		}
		return nil
	})
}
