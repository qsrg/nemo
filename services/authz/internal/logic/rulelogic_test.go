// rulelogic_test.go 覆盖规则表域 logic:UpsertRule 成功/必填与白名单校验/
// 审计先行、DeleteRule 幂等删除与 404、ListRules 列表回显。
package logic

import (
	"context"
	"strconv"
	"testing"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestUpsertRuleSuccess 新建与按 ID 更新:落业务表、审计 pending→committed、
// operator=claims.sub;更新后动作码覆盖原值。
func TestUpsertRuleSuccess(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	resp, err := NewUpsertRuleLogic(ctx, sc).UpsertRule(&types.UpsertRuleReq{
		RoleKey: "operator",
		Prefix:  "order:",
		Action:  "manage",
	})
	if err != nil {
		t.Fatalf("新建规则: %v", err)
	}
	if resp.Id == "" || resp.RoleKey != "operator" || resp.Prefix != "order:" || resp.Action != "manage" {
		t.Errorf("新建规则返回 = %+v", resp)
	}

	rules, err := sc.RuleRepo.List(ctx)
	if err != nil {
		t.Fatalf("List 规则: %v", err)
	}
	if len(rules) != 1 || strconv.FormatInt(rules[0].ID, 10) != resp.Id || rules[0].Action != "manage" {
		t.Errorf("业务表规则 = %+v, 期望 id=%s action=manage", rules, resp.Id)
	}

	// 同 ID 再提交 → 更新（动作码覆盖）。
	updated, err := NewUpsertRuleLogic(ctx, sc).UpsertRule(&types.UpsertRuleReq{
		Id:      resp.Id,
		RoleKey: "operator",
		Prefix:  "order:",
		Action:  "read",
	})
	if err != nil {
		t.Fatalf("更新规则: %v", err)
	}
	if updated.Id != resp.Id || updated.Action != "read" {
		t.Errorf("更新规则返回 = %+v, 期望 id=%s action=read", updated, resp.Id)
	}
	rules, _ = sc.RuleRepo.List(ctx)
	if len(rules) != 1 || rules[0].Action != "read" {
		t.Errorf("更新后业务表规则 = %+v, 期望仍 1 条且 action=read", rules)
	}

	if e := auditOf(t, sc, "upsert_rule"); e.Status != repo.AuditCommitted || e.Operator != "admin" {
		t.Errorf("upsert_rule 审计 = %+v, 期望 committed/operator=admin", e)
	}
}

// TestUpsertRuleValidation 生成器语义下请求体全 optional,由本层自校验:
// 角色未定义 → 400 AUTHZ_ROLE_UNKNOWN;前缀/动作码非法 → 400
// AUTHZ_RULE_INVALID;无身份 → 401;校验失败不产生审计条目。
func TestUpsertRuleValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	cases := []struct {
		name     string
		req      types.UpsertRuleReq
		wantCode string
	}{
		{"未知角色", types.UpsertRuleReq{RoleKey: "guest", Prefix: "order:"}, "AUTHZ_ROLE_UNKNOWN"},
		{"空角色", types.UpsertRuleReq{Prefix: "order:"}, "AUTHZ_ROLE_UNKNOWN"},
		{"空前缀", types.UpsertRuleReq{RoleKey: "operator"}, "AUTHZ_RULE_INVALID"},
		{"前缀含空白", types.UpsertRuleReq{RoleKey: "operator", Prefix: "order :"}, "AUTHZ_RULE_INVALID"},
		{"大写动作码", types.UpsertRuleReq{RoleKey: "operator", Prefix: "order:", Action: "Manage"}, "AUTHZ_RULE_INVALID"},
		{"非法动作码", types.UpsertRuleReq{RoleKey: "operator", Prefix: "order:", Action: "read!"}, "AUTHZ_RULE_INVALID"},
	}
	for _, tc := range cases {
		_, err := NewUpsertRuleLogic(ctx, sc).UpsertRule(&tc.req)
		e := kiterrors.From(err)
		if e == nil || e.Status != 400 || e.Code != tc.wantCode {
			t.Errorf("%s: 应 400 %s, 实际 %v", tc.name, tc.wantCode, err)
		}
	}

	_, err := NewUpsertRuleLogic(context.Background(), sc).UpsertRule(&types.UpsertRuleReq{
		RoleKey: "operator", Prefix: "order:",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 401 {
		t.Errorf("无身份应 401, 实际 %v", err)
	}

	if n := auditCount(t, sc); n != 0 {
		t.Errorf("校验失败不应产生审计条目, 实际 %d 条", n)
	}
}

// TestDeleteRule 删除规则:业务表移除、审计 committed;重复删除 → 404
// AUTHZ_RULE_NOT_FOUND 且不产生第二条审计。
func TestDeleteRule(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	resp, err := NewUpsertRuleLogic(ctx, sc).UpsertRule(&types.UpsertRuleReq{
		RoleKey: "readonly", Prefix: "report:",
	})
	if err != nil {
		t.Fatalf("新建规则: %v", err)
	}

	if err := NewDeleteRuleLogic(ctx, sc).DeleteRule(&types.RuleIdReq{Id: resp.Id}); err != nil {
		t.Fatalf("删除规则: %v", err)
	}
	rules, _ := sc.RuleRepo.List(ctx)
	if len(rules) != 0 {
		t.Errorf("删除后业务表应空, 实际 %v", rules)
	}

	// 重复删除 → 404(参数态校验在写路径之前,不落审计)。
	err = NewDeleteRuleLogic(ctx, sc).DeleteRule(&types.RuleIdReq{Id: resp.Id})
	if e := kiterrors.From(err); e == nil || e.Status != 404 || e.Code != "AUTHZ_RULE_NOT_FOUND" {
		t.Errorf("重复删除应 404 AUTHZ_RULE_NOT_FOUND, 实际 %v", err)
	}
	// 审计共 2 条：新建时的 upsert_rule + 首次删除的 delete_rule。
	if n := auditCount(t, sc); n != 2 {
		t.Errorf("审计条目 = %d, 期望 2(仅首次删除落审计)", n)
	}
	if e := auditOf(t, sc, "delete_rule"); e.Status != repo.AuditCommitted {
		t.Errorf("delete_rule 审计状态 = %q, 期望 committed", e.Status)
	}
}

// TestListRules 列表回显:条数与字段一致。
func TestListRules(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	for _, r := range []types.UpsertRuleReq{
		{RoleKey: "operator", Prefix: "order:", Action: "manage"},
		{RoleKey: "readonly", Prefix: "report:"},
	} {
		if _, err := NewUpsertRuleLogic(ctx, sc).UpsertRule(&r); err != nil {
			t.Fatalf("新建规则 %+v: %v", r, err)
		}
	}

	resp, err := NewListRulesLogic(ctx, sc).ListRules()
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("ListRules = %+v, 期望 2 条", resp)
	}
	byID := map[string]types.RuleResp{}
	for _, it := range resp.Items {
		byID[it.Id] = it
	}
	for _, want := range []types.RuleResp{
		{RoleKey: "operator", Prefix: "order:", Action: "manage"},
		{RoleKey: "readonly", Prefix: "report:", Action: ""},
	} {
		found := false
		for _, it := range byID {
			if it.RoleKey == want.RoleKey && it.Prefix == want.Prefix && it.Action == want.Action {
				found = true
			}
		}
		if !found {
			t.Errorf("列表缺少规则 %+v: %v", want, resp.Items)
		}
	}
}
