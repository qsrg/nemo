// memberlogic_test.go 覆盖成员域 logic:加成员成功(含组织层级继承判定)、
// 必填校验、移除成员幂等、审计 pending→committed。
package logic

import (
	"testing"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestAddMemberSuccess 加成员:member 元组落引擎、上级组织成员沿层级继承、
// 审计 committed 且 operator=claims.sub。
func TestAddMemberSuccess(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	childID := createOrgViaLogic(t, sc, "华东分公司", rootID)

	resp, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  rootID,
		UserId: "u1",
	})
	if err != nil {
		t.Fatalf("加成员: %v", err)
	}
	if resp.OrgId != rootID || resp.UserId != "u1" {
		t.Errorf("MemberResp = %+v, 期望 org=%s user=u1", resp, rootID)
	}

	// 直接成员关系在引擎中生效。
	ok, err := sc.Engine.Check(ctx, "user:u1", "member", "org:"+rootID)
	if err != nil || !ok {
		t.Fatalf("u1 对根组织 member 判定应 true: ok=%v err=%v", ok, err)
	}
	// 模型 v5 语义:子组织成员沿 child 链聚合进父组织成员集合,反向(上级成员
	// 下传为下级成员)不再成立——u1 是根的成员,但不是子组织的成员。
	ok, err = sc.Engine.Check(ctx, "user:u1", "member", "org:"+childID)
	if err != nil || ok {
		t.Fatalf("u1 对子组织 member 判定应 false(上级成员不下传): ok=%v err=%v", ok, err)
	}

	// u1 的 add_member 审计（在补测 u2 之前断言，避免 auditOf 取到后写条目）。
	e := auditOf(t, sc, "add_member")
	if e.Status != repo.AuditCommitted || e.Operator != "admin" {
		t.Errorf("add_member 审计 = %+v, 期望 committed 且 operator=admin", e)
	}
	if e.Target != "org:"+rootID {
		t.Errorf("add_member 审计 target = %q, 期望 org:%s", e.Target, rootID)
	}

	// 对照:u2 加入子组织后,对子组织与根组织 member 均判定 true(子成员聚合)。
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  childID,
		UserId: "u2",
	}); err != nil {
		t.Fatalf("加成员 u2: %v", err)
	}
	ok, err = sc.Engine.Check(ctx, "user:u2", "member", "org:"+childID)
	if err != nil || !ok {
		t.Fatalf("u2 对子组织 member 判定应 true: ok=%v err=%v", ok, err)
	}
	ok, err = sc.Engine.Check(ctx, "user:u2", "member", "org:"+rootID)
	if err != nil || !ok {
		t.Fatalf("u2 对根组织 member 判定应 true(子成员聚合进父): ok=%v err=%v", ok, err)
	}
}

// TestAddMemberValidation userId 空 → 400 AUTHZ_MEMBER_INVALID,不产生审计。
func TestAddMemberValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	rootID := createOrgViaLogic(t, sc, "总部", "")

	_, err := NewAddMemberLogic(adminCtx(), sc).AddMember(&types.MemberReq{
		OrgId:  rootID,
		UserId: " ",
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 400 || e.Code != "AUTHZ_MEMBER_INVALID" {
		t.Errorf("空 userId 应 400 AUTHZ_MEMBER_INVALID, 实际 %v", err)
	}
	if auditCount(t, sc) != 1 { // 只有 create_org 那一条
		t.Errorf("校验失败不应产生审计, 实际 %d 条", auditCount(t, sc))
	}
}

// TestListOrgMembers 列出直接成员:只含本组织的直接 member 元组、
// 其他组织的成员不混入;空组织返回空数组(非 null);不存在组织 404。
func TestListOrgMembers(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	otherID := createOrgViaLogic(t, sc, "华南总部", "")
	emptyID := createOrgViaLogic(t, sc, "空壳部门", "")
	for _, uid := range []string{"u2", "u1"} { // 乱序添加,验证输出排序
		if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
			OrgId:  rootID,
			UserId: uid,
		}); err != nil {
			t.Fatalf("加成员 %s: %v", uid, err)
		}
	}
	// 其他组织的成员不应出现在 root 列表。
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  otherID,
		UserId: "u9",
	}); err != nil {
		t.Fatalf("加成员 other: %v", err)
	}

	resp, err := NewListOrgMembersLogic(ctx, sc).ListOrgMembers(&types.OrgIdReq{Id: rootID})
	if err != nil {
		t.Fatalf("ListOrgMembers: %v", err)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("成员数 = %d, 期望 2: %+v", len(resp.Members), resp.Members)
	}
	for i, want := range []string{"u1", "u2"} { // 升序输出
		if resp.Members[i].UserId != want || resp.Members[i].OrgId != rootID {
			t.Errorf("members[%d] = %+v, 期望 user=%s org=%s", i, resp.Members[i], want, rootID)
		}
	}

	// 空组织:返回空数组(非 null)。
	empty, err := NewListOrgMembersLogic(ctx, sc).ListOrgMembers(&types.OrgIdReq{Id: emptyID})
	if err != nil {
		t.Fatalf("空组织 ListOrgMembers: %v", err)
	}
	if empty.Members == nil || len(empty.Members) != 0 {
		t.Errorf("空组织应返回空数组非 null, 实际 %+v", empty.Members)
	}

	// 不存在的组织:404。
	_, err = NewListOrgMembersLogic(ctx, sc).ListOrgMembers(&types.OrgIdReq{Id: "org-does-not-exist"})
	e := kiterrors.From(err)
	if e == nil || e.Status != 404 || e.Code != "AUTHZ_ORG_NOT_FOUND" {
		t.Errorf("不存在组织应 404 AUTHZ_ORG_NOT_FOUND, 实际 %v", err)
	}
}

// TestRemoveMemberIdempotent 移除成员后判定翻转为 false;重复移除幂等不报错。
func TestRemoveMemberIdempotent(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  rootID,
		UserId: "u1",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}

	if err := NewRemoveMemberLogic(ctx, sc).RemoveMember(&types.MemberIdReq{
		OrgId:  rootID,
		UserId: "u1",
	}); err != nil {
		t.Fatalf("移除成员: %v", err)
	}
	ok, err := sc.Engine.Check(ctx, "user:u1", "member", "org:"+rootID)
	if err != nil || ok {
		t.Errorf("移除后 member 判定应 false: ok=%v err=%v", ok, err)
	}

	// 重复移除不报错(幂等)。
	if err := NewRemoveMemberLogic(ctx, sc).RemoveMember(&types.MemberIdReq{
		OrgId:  rootID,
		UserId: "u1",
	}); err != nil {
		t.Errorf("重复移除应幂等: %v", err)
	}

	if e := auditOf(t, sc, "remove_member"); e.Status != repo.AuditCommitted {
		t.Errorf("remove_member 审计状态 = %q, 期望 committed", e.Status)
	}
}
