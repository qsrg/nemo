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
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// MigrateFromRulesLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type MigrateFromRulesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMigrateFromRulesLogic 构造业务规则层实例。
func NewMigrateFromRulesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MigrateFromRulesLogic {
	return &MigrateFromRulesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MigrateFromRules 规则表存量一次性迁移为 permission 元组（去角色化改造，
// 幂等可重放）：
//  1. 读规则表全部规则——码 = 服务级规则（Action 空）产出 `<服务>:read`
//     缺省码，动作级产出 `<服务>:<动作>`（服务名 = Prefix 去尾部冒号）；
//  2. 读引擎 platform_role 全部 assignee 元组（user 与 org#member 两形态，
//     含 B 阶段改造 2.1 的角色授予组织）；
//  3. 对每个 (角色主体, 码) 组合写 permission:{code}#can@{subject}——
//     user:xxx 原样映射，org:xxx#member 原样映射（组织成员继承语义不变）；
//     同一元组去重后一次 WriteTuples 提交，重复迁移由读后写幂等保证；
//  4. 审计先行（action=migrate_rules，快照含规则数与提交元组数）。
//
// 迁移后规则表停用（me/permissions 已改引擎实时查询，不再消费规则表）；
// 规则/角色管理 API 保留（.api 已标注 deprecated）。规则表读失败 / 引擎
// 读或写失败 → 503 fail-closed。规则为空时不动引擎，仅落审计（Written=0）。
func (l *MigrateFromRulesLogic) MigrateFromRules() (*types.MigrateRulesResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	rules, err := l.svcCtx.RuleRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("规则表读取失败: %v", err)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "权限规则读取失败，迁移被拒绝（fail-closed）")
	}

	// platform_role 全部 assignee 元组。OpenFGA Read 不支持"仅按 relation 过滤"
	//（无 object 的 relation 过滤非法，实测引擎报错），故全量读回后本地过滤。
	assignees, err := l.svcCtx.Engine.ReadTuples(l.ctx, "", "")
	if err != nil {
		l.Logger.Errorf("读平台角色元组失败: %v", err)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "授权引擎不可用，迁移被拒绝（fail-closed）")
	}
	filtered := make([]engine.Tuple, 0, len(assignees))
	for _, t := range assignees {
		if t.Relation == "assignee" && strings.HasPrefix(t.Object, "platform_role:") {
			filtered = append(filtered, t)
		}
	}
	assignees = filtered

	// 码 → 角色集合：规则归并（同角色多规则产出多码）。
	codesByRole := make(map[string][]string)
	seenPair := make(map[string]bool)
	ruleCount := 0
	for _, r := range rules {
		svcName := strings.TrimSuffix(r.Prefix, ":")
		if svcName == "" {
			continue
		}
		action := r.Action
		if action == "" {
			action = "read" // 服务级规则产出缺省码 `<服务>:read`
		}
		code := svcName + ":" + action
		if !seenPair[r.RoleKey+"\x00"+code] {
			seenPair[r.RoleKey+"\x00"+code] = true
			codesByRole[r.RoleKey] = append(codesByRole[r.RoleKey], code)
		}
		ruleCount++
	}

	// (主体, 码) → permission 元组（跨角色去重）。
	tupleSeen := make(map[string]bool)
	tuples := make([]engine.Tuple, 0)
	for _, t := range assignees {
		if t.Relation != "assignee" {
			continue
		}
		roleKey := strings.TrimPrefix(t.Object, "platform_role:")
		codes, ok := codesByRole[roleKey]
		if !ok {
			continue
		}
		// 主体形态校验：只迁移 user:/org:#member 两种（其余形态非法，跳过并记日志）。
		if !strings.HasPrefix(t.User, "user:") && !strings.HasPrefix(t.User, "org:") {
			logx.WithContext(l.ctx).Errorf("迁移跳过非法主体: %s", t.User)
			continue
		}
		for _, code := range codes {
			key := t.User + "\x00" + code
			if tupleSeen[key] {
				continue
			}
			tupleSeen[key] = true
			tuples = append(tuples, engine.Tuple{
				User:     t.User,
				Relation: "can",
				Object:   permObject(code),
			})
		}
	}
	sort.Slice(tuples, func(i, j int) bool { return tuples[i].User+tuples[i].Object < tuples[j].User+tuples[j].Object })

	snapshot := map[string]any{"rules": ruleCount, "written": len(tuples)}
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "migrate_rules", "permissions", snapshot, func() error {
		if len(tuples) == 0 {
			return nil // 规则为空 / 无可映射主体：不动引擎，审计照记
		}
		return l.svcCtx.Engine.WriteTuples(l.ctx, tuples)
	})
	if err != nil {
		return nil, err
	}

	return &types.MigrateRulesResp{Written: len(tuples), Rules: ruleCount}, nil
}
