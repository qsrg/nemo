// rolelogic_test.go 覆盖平台角色域 logic:赋角色成功、白名单外角色 400、
// 必填校验、回收幂等、角色列表、引擎不可用 503 + 审计 failed。
package logic

import (
	"testing"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestAssignRoleSuccess 赋内置角色:assignee 元组落引擎、审计 committed。
func TestAssignRoleSuccess(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	resp, err := NewAssignRoleLogic(ctx, sc).AssignRole(&types.AssignRoleReq{
		RoleKey: "operator",
		UserId:  "u9",
	})
	if err != nil {
		t.Fatalf("赋角色: %v", err)
	}
	if resp.RoleKey != "operator" || resp.Name == "" {
		t.Errorf("RoleResp = %+v, 期望 roleKey=operator 且展示名非空", resp)
	}

	ok, err := sc.Engine.Check(ctx, "user:u9", "assignee", "platform_role:operator")
	if err != nil || !ok {
		t.Fatalf("assignee 判定应 true: ok=%v err=%v", ok, err)
	}

	e := auditOf(t, sc, "assign_role")
	if e.Status != repo.AuditCommitted || e.Operator != "admin" || e.Target != "platform_role:operator" {
		t.Errorf("assign_role 审计 = %+v, 期望 committed/admin/platform_role:operator", e)
	}
}

// TestAssignRoleUnknown 白名单外角色 → 400 AUTHZ_ROLE_UNKNOWN,不产生审计。
func TestAssignRoleUnknown(t *testing.T) {
	sc := newTestSvcCtx(t)

	_, err := NewAssignRoleLogic(adminCtx(), sc).AssignRole(&types.AssignRoleReq{
		RoleKey: "hacker",
		UserId:  "u9",
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 400 || e.Code != "AUTHZ_ROLE_UNKNOWN" {
		t.Errorf("未注册角色应 400 AUTHZ_ROLE_UNKNOWN, 实际 %v", err)
	}
	if auditCount(t, sc) != 0 {
		t.Errorf("白名单拒绝不应产生审计条目, 实际 %d 条", auditCount(t, sc))
	}
}

// TestAssignRoleValidation roleKey/userId 空 → 400 AUTHZ_ROLE_INVALID。
func TestAssignRoleValidation(t *testing.T) {
	sc := newTestSvcCtx(t)

	for _, req := range []types.AssignRoleReq{
		{RoleKey: "", UserId: "u9"},
		{RoleKey: "operator", UserId: ""},
	} {
		_, err := NewAssignRoleLogic(adminCtx(), sc).AssignRole(&req)
		if e := kiterrors.From(err); e == nil || e.Status != 400 || e.Code != "AUTHZ_ROLE_INVALID" {
			t.Errorf("必填缺失应 400 AUTHZ_ROLE_INVALID, req=%+v 实际 %v", req, err)
		}
	}
}

// TestRevokeRoleIdempotent 回收角色后判定翻转;重复回收幂等不报错。
func TestRevokeRoleIdempotent(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	if _, err := NewAssignRoleLogic(ctx, sc).AssignRole(&types.AssignRoleReq{
		RoleKey: "readonly",
		UserId:  "u9",
	}); err != nil {
		t.Fatalf("赋角色: %v", err)
	}

	resp, err := NewRevokeRoleLogic(ctx, sc).RevokeRole(&types.AssignRoleReq{
		RoleKey: "readonly",
		UserId:  "u9",
	})
	if err != nil {
		t.Fatalf("回收角色: %v", err)
	}
	if resp.RoleKey != "readonly" {
		t.Errorf("RevokeRole 返回 = %+v, 期望 roleKey=readonly", resp)
	}
	ok, err := sc.Engine.Check(ctx, "user:u9", "assignee", "platform_role:readonly")
	if err != nil || ok {
		t.Errorf("回收后 assignee 判定应 false: ok=%v err=%v", ok, err)
	}

	// 重复回收不报错(幂等)。
	if _, err := NewRevokeRoleLogic(ctx, sc).RevokeRole(&types.AssignRoleReq{
		RoleKey: "readonly",
		UserId:  "u9",
	}); err != nil {
		t.Errorf("重复回收应幂等: %v", err)
	}

	if e := auditOf(t, sc, "revoke_role"); e.Status != repo.AuditCommitted {
		t.Errorf("revoke_role 审计状态 = %q, 期望 committed", e.Status)
	}
}

// TestListRoles 内置白名单两角色全部可见（super_admin 已移除，去角色化
// 后超管鉴权统一走 permission:*.* 权限码）。
func TestListRoles(t *testing.T) {
	sc := newTestSvcCtx(t)

	resp, err := NewListRolesLogic(adminCtx(), sc).ListRoles()
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("角色数 = %d, 期望 2: %v", resp.Total, resp.Items)
	}
	keys := map[string]bool{}
	for _, r := range resp.Items {
		keys[r.RoleKey] = true
		if r.Name == "" {
			t.Errorf("角色 %q 缺展示名", r.RoleKey)
		}
	}
	for _, want := range []string{"operator", "readonly"} {
		if !keys[want] {
			t.Errorf("角色列表缺 %q: %v", want, resp.Items)
		}
	}
}

// TestEngineUnavailableMarksAuditFailed 引擎不可用:写路径 503
// AUTHZ_SERVICE_UNAVAILABLE,审计条目标记 failed(附原因)。
func TestEngineUnavailableMarksAuditFailed(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()
	rootID := createOrgViaLogic(t, sc, "总部", "")

	// 停掉引擎模拟引擎故障(Stop 幂等,t.Cleanup 再调安全)。
	sc.Engine.Stop()

	_, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  rootID,
		UserId: "u1",
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 503 || e.Code != "AUTHZ_SERVICE_UNAVAILABLE" {
		t.Errorf("引擎故障应 503 AUTHZ_SERVICE_UNAVAILABLE, 实际 %v", err)
	}

	// 对应审计条目应为 failed。
	entry := auditOf(t, sc, "add_member")
	if entry.Status != repo.AuditFailed {
		t.Errorf("引擎故障后审计状态 = %q, 期望 failed", entry.Status)
	}
}
