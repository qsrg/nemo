// roleorglogic_test.go 覆盖平台角色授予组织的扩展语义（B 阶段改造 2）：
// org 目标写 userset 元组 org:{uid}#member、org 不存在 404、targetType 非法
// 400、org.member 的 parent 传递继承链、revoke 与授予对称、user 缺省路径
// 不回归。全部走真 memory 引擎判定（Check/ListObjects 验证）。
package logic

import (
	"testing"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestAssignRoleToOrgDirectMember 授予组织：直接成员 Check 生效，
// 且 list-objects 层面（登录取 roles 的既有通道）能看到该角色。
func TestAssignRoleToOrgDirectMember(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()
	orgID := createOrgViaLogic(t, sc, "总部", "")

	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  orgID,
		UserId: "u1",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}

	resp, err := NewAssignRoleLogic(ctx, sc).AssignRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     orgID,
		TargetType: "org",
	})
	if err != nil {
		t.Fatalf("授予组织角色: %v", err)
	}
	if resp.RoleKey != "operator" || resp.Name == "" {
		t.Errorf("RoleResp = %+v, 期望 roleKey=operator 且展示名非空", resp)
	}

	// 判定面：组织成员（userset org:{id}#member）经模型 v2 获得角色。
	ok, err := sc.Engine.Check(ctx, "user:u1", "assignee", "platform_role:operator")
	if err != nil || !ok {
		t.Fatalf("组织成员 assignee 判定应 true: ok=%v err=%v", ok, err)
	}

	// list-objects 层面：无需重新登录语义之外的动作，角色即可被列出
	//（me/permissions 的 roles claim 即来自该通道）。
	objs, err := sc.Engine.ListObjects(ctx, "user:u1", "assignee", "platform_role")
	if err != nil {
		t.Fatalf("list-objects: %v", err)
	}
	found := false
	for _, o := range objs {
		if o == "platform_role:operator" {
			found = true
		}
	}
	if !found {
		t.Errorf("list-objects 应含 platform_role:operator, 实际 %v", objs)
	}

	// 审计照 writeWithAudit：committed。
	if e := auditOf(t, sc, "assign_role"); e.Status != repo.AuditCommitted {
		t.Errorf("assign_role 审计状态 = %q, 期望 committed", e.Status)
	}
}

// TestAssignRoleToOrgNotFound org 目标不存在 → 404 AUTHZ_ORG_NOT_FOUND，
// 不产生审计条目（参数校验类失败先于审计写路径）。
func TestAssignRoleToOrgNotFound(t *testing.T) {
	sc := newTestSvcCtx(t)

	_, err := NewAssignRoleLogic(adminCtx(), sc).AssignRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     "org-no-such",
		TargetType: "org",
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 404 || e.Code != "AUTHZ_ORG_NOT_FOUND" {
		t.Errorf("org 不存在应 404 AUTHZ_ORG_NOT_FOUND, 实际 %v", err)
	}
	if auditCount(t, sc) != 0 {
		t.Errorf("404 不应产生审计条目, 实际 %d 条", auditCount(t, sc))
	}
}

// TestAssignRoleInvalidTargetType targetType 非法 → 400 AUTHZ_ROLE_INVALID。
func TestAssignRoleInvalidTargetType(t *testing.T) {
	sc := newTestSvcCtx(t)

	_, err := NewAssignRoleLogic(adminCtx(), sc).AssignRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     "u9",
		TargetType: "group",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 400 || e.Code != "AUTHZ_ROLE_INVALID" {
		t.Errorf("非法 targetType 应 400 AUTHZ_ROLE_INVALID, 实际 %v", err)
	}
}

// TestAssignRoleToOrgInheritedChain 继承链（模型 v5 语义：member from child，
// 子组织成员沿 child 关系聚合进父组织成员集合）——授予任一组织时，该组织
// 子树内（直属 + 全部子孙）成员生效；授权叶子不影响祖先/兄弟部门成员。
func TestAssignRoleToOrgInheritedChain(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	grand := createOrgViaLogic(t, sc, "集团", "")
	parent := createOrgViaLogic(t, sc, "总部", grand)
	child := createOrgViaLogic(t, sc, "研发部", parent)

	// u2 只挂在集团（child 的祖父级）。
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  grand,
		UserId: "u2",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}
	// u3 只挂在研发部（child 自身），作为子树成员对照。
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  child,
		UserId: "u3",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}

	// 授予最下级组织 child：子树成员 u3 生效；上级成员 u2 不生效
	//（授权叶子不影响祖先部门成员——v5 语义反转的核心断言）。
	if _, err := NewAssignRoleLogic(ctx, sc).AssignRole(&types.AssignRoleReq{
		RoleKey:    "readonly",
		UserId:     child,
		TargetType: "org",
	}); err != nil {
		t.Fatalf("授予组织角色: %v", err)
	}
	ok, err := sc.Engine.Check(ctx, "user:u3", "assignee", "platform_role:readonly")
	if err != nil || !ok {
		t.Fatalf("授予组织对自身成员 assignee 判定应 true: ok=%v err=%v", ok, err)
	}
	ok, err = sc.Engine.Check(ctx, "user:u2", "assignee", "platform_role:readonly")
	if err != nil || ok {
		t.Fatalf("授权叶子不应使祖先部门成员生效: ok=%v err=%v", ok, err)
	}

	// 授予最上级组织 grand：整个子树成员（u2 直属 + u3 经 child→parent→grand
	// 两跳聚合）均生效。
	if _, err := NewAssignRoleLogic(ctx, sc).AssignRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     grand,
		TargetType: "org",
	}); err != nil {
		t.Fatalf("授予集团角色: %v", err)
	}
	ok, err = sc.Engine.Check(ctx, "user:u2", "assignee", "platform_role:operator")
	if err != nil || !ok {
		t.Fatalf("集团直属成员应获得集团角色: ok=%v err=%v", ok, err)
	}
	ok, err = sc.Engine.Check(ctx, "user:u3", "assignee", "platform_role:operator")
	if err != nil || !ok {
		t.Fatalf("子部门成员应经聚合获得集团角色: ok=%v err=%v", ok, err)
	}
}

// TestRevokeRoleOrgSymmetric 回收与授予对称：org 目标撤销后判定翻转，
// 重复撤销幂等；targetType 缺省仍删 user 元组（user 路径不回归）。
func TestRevokeRoleOrgSymmetric(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()
	orgID := createOrgViaLogic(t, sc, "总部", "")

	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  orgID,
		UserId: "u1",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}
	if _, err := NewAssignRoleLogic(ctx, sc).AssignRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     orgID,
		TargetType: "org",
	}); err != nil {
		t.Fatalf("授予组织角色: %v", err)
	}

	// 缺省 targetType 撤销只删 user 元组，org 元组仍在（对称性验证的前提）。
	if _, err := NewRevokeRoleLogic(ctx, sc).RevokeRole(&types.AssignRoleReq{
		RoleKey: "operator",
		UserId:  "u9",
	}); err != nil {
		t.Fatalf("缺省撤销: %v", err)
	}
	ok, err := sc.Engine.Check(ctx, "user:u1", "assignee", "platform_role:operator")
	if err != nil || !ok {
		t.Fatalf("缺省 targetType 撤销不应影响 org 授予: ok=%v err=%v", ok, err)
	}

	// org 目标撤销：成员判定翻转；重复撤销幂等不报错。
	if _, err := NewRevokeRoleLogic(ctx, sc).RevokeRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     orgID,
		TargetType: "org",
	}); err != nil {
		t.Fatalf("org 撤销: %v", err)
	}
	ok, err = sc.Engine.Check(ctx, "user:u1", "assignee", "platform_role:operator")
	if err != nil || ok {
		t.Errorf("org 撤销后成员 assignee 判定应 false: ok=%v err=%v", ok, err)
	}
	if _, err := NewRevokeRoleLogic(ctx, sc).RevokeRole(&types.AssignRoleReq{
		RoleKey:    "operator",
		UserId:     orgID,
		TargetType: "org",
	}); err != nil {
		t.Errorf("重复 org 撤销应幂等: %v", err)
	}

	if e := auditOf(t, sc, "revoke_role"); e.Status != repo.AuditCommitted {
		t.Errorf("revoke_role 审计状态 = %q, 期望 committed", e.Status)
	}
}
