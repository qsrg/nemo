// Package reconcile 是审计对账定时任务：扫超时未提交的 pending 审计条目，
// 回查引擎/业务表实际状态并修正——实态已生效补记 committed，未生效标记
// failed（附对账原因，告警日志），杜绝"权限改了但没审计"与"有审计但没
// 生效"两种不可见中间态（permission-design §6.2）。
//
// 修正语义按动作逐一定义（probe 表）：正向动作以"目标对象存在证据"为
// 生效判据，反向动作以"证据消失"为生效判据；未纳入语义表的动作保持
// pending 不动（fail-safe：宁可留待人工/后续版本接管，不误伤）。
// 挂载点为 main 内 Ticker（间隔见 config.ReconcileConf），本包只做单轮
// 对账 RunOnce，可测核心与定时挂载分离。
package reconcile

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// Result 是单轮对账摘要（main 告警日志与测试断言用）。
type Result struct {
	// Committed 本轮补记 committed 的条目数。
	Committed int
	// Failed 本轮标记 failed 的条目数。
	Failed int
	// Skipped 未纳入语义表而保持 pending 的条目数。
	Skipped int
}

// snapshot 是审计条目快照中与对账判据相关的字段（宽松解析，缺失即判据
// 不足按未生效处理）。
type snapshot struct {
	OrgId   string `json:"orgId"`
	UserId  string `json:"userId"`
	RoleKey string `json:"roleKey"`
	// Prefix/Action 与 RoleKey 构成规则三元组业务指纹：rules.id 改自增后，
	// upsert_rule 新建在写库前拿不到 ID，对账改按快照三元组判定规则是否落库。
	Prefix string `json:"prefix"`
	Action string `json:"action"`
}

// probe 判定单个 pending 条目对应的操作是否已实际生效：
// 返回 (applied, error)；error 为回查过程故障（引擎/业务表读失败），本轮
// 对账中止该条目并向上透传（保持 pending，待下轮重试）。
type probe func(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, snap snapshot) (bool, error)

// probes 是动作 → 生效判据的语义表：
//   - 正向动作（建组织/加成员/授角色/建规则/注册类型）：证据存在 = 生效；
//   - 反向动作（删组织/移成员/收角色/删规则）：证据消失 = 生效；
//   - update_org 前后目标对象均有元组，以对象仍有元组为生效判据。
var probes = map[string]probe{
	"create_org":             probeTuplesPresent,
	"update_org":             probeTuplesPresent,
	"delete_org":             probeTuplesAbsent,
	"add_member":             probeMemberPresent,
	"remove_member":          probeMemberAbsent,
	"assign_role":            probeAssigneePresent,
	"revoke_role":            probeAssigneeAbsent,
	"upsert_rule":            probeRulePresent,
	"delete_rule":            probeRuleAbsent,
	"register_resource_type": probeResourceTypePresent,
}

// RunOnce 执行单轮对账：ListPending(olderThan) 逐条按语义表回查实态并
// 修正。olderThan 传负值时全量 pending 入扫（测试用，规避秒精度）。
// 单条回查故障即中止本轮（返回错误）：带着不可靠的引擎视图做修正，
// 比漏一轮更危险。
func RunOnce(ctx context.Context, sc *svc.ServiceContext, olderThan time.Duration) (Result, error) {
	var res Result
	if sc == nil || sc.AuditRepo == nil {
		return res, stderrors.New("reconcile: ServiceContext 未装配")
	}

	entries, err := sc.AuditRepo.ListPending(ctx, olderThan)
	if err != nil {
		return res, err
	}
	for _, e := range entries {
		p, ok := probes[e.Action]
		if !ok {
			// 未纳入语义表的动作：保持 pending，计入跳过并告警（人工跟进）。
			res.Skipped++
			logx.WithContext(ctx).Errorf("对账跳过:未纳入语义表的 pending 条目 id=%s action=%s target=%s", e.ID, e.Action, e.Target)
			continue
		}

		var snap snapshot
		_ = json.Unmarshal([]byte(e.Snapshot), &snap) // 宽松解析：快照残缺按零值判据处理

		applied, err := p(ctx, sc, e, snap)
		if err != nil {
			return res, err
		}
		if applied {
			if err := sc.AuditRepo.MarkCommitted(ctx, e.ID); err != nil {
				return res, err
			}
			res.Committed++
			logx.WithContext(ctx).Infof("对账补记 committed: id=%s action=%s target=%s", e.ID, e.Action, e.Target)
		} else {
			if err := sc.AuditRepo.MarkFailed(ctx, e.ID, "对账修正:回查实态操作未生效"); err != nil {
				return res, err
			}
			res.Failed++
			logx.WithContext(ctx).Errorf("对账告警:pending 超时且实态未生效,已标记 failed id=%s action=%s target=%s", e.ID, e.Action, e.Target)
		}
	}
	return res, nil
}

// probeTuplesPresent 正向组织动作判据:target 对象在引擎中仍有元组。
func probeTuplesPresent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, _ snapshot) (bool, error) {
	tuples, err := sc.Engine.ReadTuples(ctx, e.Target, "")
	if err != nil {
		return false, err
	}
	return len(tuples) > 0, nil
}

// probeTuplesAbsent 反向组织动作判据:target 对象在引擎中已无元组。
func probeTuplesAbsent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, _ snapshot) (bool, error) {
	tuples, err := sc.Engine.ReadTuples(ctx, e.Target, "")
	if err != nil {
		return false, err
	}
	return len(tuples) == 0, nil
}

// hasMemberTuple 判定 target(org:<id>) 上是否存在 user:<uid> 的 member 元组。
func hasMemberTuple(ctx context.Context, sc *svc.ServiceContext, target, uid string) (bool, error) {
	tuples, err := sc.Engine.ReadTuples(ctx, target, "member")
	if err != nil {
		return false, err
	}
	for _, t := range tuples {
		if t.User == "user:"+uid {
			return true, nil
		}
	}
	return false, nil
}

// probeMemberPresent 加成员判据:成员元组存在（快照缺 userId 按未生效处理）。
func probeMemberPresent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, snap snapshot) (bool, error) {
	if snap.UserId == "" {
		return false, nil
	}
	return hasMemberTuple(ctx, sc, e.Target, snap.UserId)
}

// probeMemberAbsent 移成员判据:成员元组消失（快照缺 userId 按未生效处理）。
func probeMemberAbsent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, snap snapshot) (bool, error) {
	if snap.UserId == "" {
		return false, nil
	}
	ok, err := hasMemberTuple(ctx, sc, e.Target, snap.UserId)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// hasAssigneeTuple 判定 target(platform_role:<key>) 上是否存在 user:<uid>
// 的 assignee 元组。
func hasAssigneeTuple(ctx context.Context, sc *svc.ServiceContext, target, uid string) (bool, error) {
	tuples, err := sc.Engine.ReadTuples(ctx, target, "assignee")
	if err != nil {
		return false, err
	}
	for _, t := range tuples {
		if t.User == "user:"+uid {
			return true, nil
		}
	}
	return false, nil
}

// probeAssigneePresent 授角色判据:assignee 元组存在（快照缺 userId 按未生效处理）。
func probeAssigneePresent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, snap snapshot) (bool, error) {
	if snap.UserId == "" {
		return false, nil
	}
	return hasAssigneeTuple(ctx, sc, e.Target, snap.UserId)
}

// probeAssigneeAbsent 收角色判据:assignee 元组消失（快照缺 userId 按未生效处理）。
func probeAssigneeAbsent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, snap snapshot) (bool, error) {
	if snap.UserId == "" {
		return false, nil
	}
	ok, err := hasAssigneeTuple(ctx, sc, e.Target, snap.UserId)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// probeRulePresent 建/改规则判据：规则已在业务表。
// rules.id 改 DB 自增后，upsert 新建在审计落库时拿不到 ID，故优先按快照
// 三元组（roleKey+prefix+action，业务唯一）判定；快照缺 roleKey（异常/
// 历史条目）时回退按 target rule:<数字id> 判定。
func probeRulePresent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, snap snapshot) (bool, error) {
	if snap.RoleKey != "" {
		return ruleExistsByTriple(ctx, sc, snap.RoleKey, snap.Prefix, snap.Action)
	}
	return ruleExistsByTargetID(ctx, sc, e.Target)
}

// probeRuleAbsent 删规则判据:target(rule:<数字id>) 的规则已不在业务表。
func probeRuleAbsent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, _ snapshot) (bool, error) {
	ok, err := ruleExistsByTargetID(ctx, sc, e.Target)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// ruleExistsByTriple 按规则三元组（角色 × 前缀 × 动作，action 空 = 服务级，
// 空串也参与全等匹配）查业务表是否存在（规则表量级小，List 全量扫描足够）。
func ruleExistsByTriple(ctx context.Context, sc *svc.ServiceContext, roleKey, prefix, action string) (bool, error) {
	rules, err := sc.RuleRepo.List(ctx)
	if err != nil {
		return false, err
	}
	for _, r := range rules {
		if r.RoleKey == roleKey && r.Prefix == prefix && r.Action == action {
			return true, nil
		}
	}
	return false, nil
}

// ruleExistsByTargetID 按 target(rule:<数字id>) 查业务表规则是否存在；
// target 中 id 非法（非数字）按不存在处理（历史字符串 ID 条目随清表重建已失效）。
func ruleExistsByTargetID(ctx context.Context, sc *svc.ServiceContext, target string) (bool, error) {
	id, err := strconv.ParseInt(trimPrefix(target, "rule:"), 10, 64)
	if err != nil || id <= 0 {
		return false, nil
	}
	rules, err := sc.RuleRepo.List(ctx)
	if err != nil {
		return false, err
	}
	for _, r := range rules {
		if r.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// probeResourceTypePresent 注册类型判据:target(type:<名>) 已在注册表。
func probeResourceTypePresent(ctx context.Context, sc *svc.ServiceContext, e repo.AuditEntry, _ snapshot) (bool, error) {
	name := trimPrefix(e.Target, "type:")
	_, err := sc.ResourceTypeRepo.Get(ctx, name)
	if stderrors.Is(err, repo.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// trimPrefix 去除 target 的语义前缀（无前缀时原样返回）。
func trimPrefix(s, prefix string) string {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
