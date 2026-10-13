// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器返回。

package logic

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpsertRuleLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type UpsertRuleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpsertRuleLogic 构造业务规则层实例。
func NewUpsertRuleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpsertRuleLogic {
	return &UpsertRuleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// actionPattern 是动作分组标签的合法形态（permission-design §5.2：前缀 ×
// 动作标签，标签小写字母开头的小写字母/数字/下划线）。
var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// UpsertRule 新建/更新规则（角色 × 前缀 × 动作分组标签）：
//   - 校验（写路径前置，失败不落审计）：操作人必在；RoleKey 必须是内置
//     平台角色（白名单外 400）；Prefix 必填且不得含空白；Action 可空
//     （空 = 服务级授权），非空时须为合法动作标签；
//   - 写路径（审计先行）：业务表 Upsert（ID=0 新建由 DB 自增生成，带 ID
//     更新）；repo 失败 → MarkFailed + 503。
//
// 主键改造：rules.id 为 DB 自增主键，新建在写库前 ID 未知。审计 target：
//   - 更新（请求带数字 ID）：rule:<id>；
//   - 新建：rule:<roleKey>|<prefix>|<action> 三元组业务指纹（三元组业务唯一，
//     与对账 probeRulePresent 的 snapshot 三元组判据一致）。
func (l *UpsertRuleLogic) UpsertRule(req *types.UpsertRuleReq) (*types.RuleResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	roleKey := strings.TrimSpace(req.RoleKey)
	if roleName(roleKey) == "" {
		return nil, errors.BadRequest("AUTHZ_ROLE_UNKNOWN", "角色未定义")
	}
	prefix := strings.TrimSpace(req.Prefix)
	if prefix == "" || strings.ContainsAny(prefix, " \t\r\n") {
		return nil, errors.BadRequest("AUTHZ_RULE_INVALID", "前缀不能为空且不得含空白")
	}
	action := strings.TrimSpace(req.Action)
	if action != "" && !actionPattern.MatchString(action) {
		return nil, errors.BadRequest("AUTHZ_RULE_INVALID", "动作标签须为小写字母开头的小写字母/数字/下划线")
	}

	// ID：请求带数字 ID 即按其更新（upsert 语义）；空 = 新建（DB 自增）。
	// 审计 target 在写路径前确定：更新用 rule:<id>，新建用三元组业务指纹。
	var ruleID int64
	var auditTarget string
	if idStr := strings.TrimSpace(req.Id); idStr != "" {
		parsed, perr := strconv.ParseInt(idStr, 10, 64)
		if perr != nil || parsed <= 0 {
			return nil, errors.BadRequest("AUTHZ_RULE_INVALID", "规则 ID 非法")
		}
		ruleID = parsed
		auditTarget = "rule:" + idStr
	} else {
		auditTarget = "rule:" + roleKey + "|" + prefix + "|" + action
	}

	rule := repo.Rule{ID: ruleID, RoleKey: roleKey, Prefix: prefix, Action: action}
	// 审计字段（AGENTS.md 规范）：新建填 create_by，更新填 update_by，均由操作人填充。
	if ruleID == 0 {
		rule.CreateBy = operator
	}
	rule.UpdateBy = operator
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "upsert_rule", auditTarget, req, func() error {
		saved, werr := l.svcCtx.RuleRepo.Upsert(l.ctx, rule)
		if werr != nil {
			return werr
		}
		ruleID = saved.ID // 新建时回填 DB 自增 ID，供响应返回。
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &types.RuleResp{Id: strconv.FormatInt(ruleID, 10), RoleKey: roleKey, Prefix: prefix, Action: action}, nil
}
