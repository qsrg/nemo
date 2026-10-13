// permissionlogic_test.go 覆盖权限点授权面（去角色化改造）：
//   - GrantPermissions：授予/幂等/参数校验/org 404/审计 grant_permission/
//     授予后 me/permissions 即时生效（含组织继承）；
//   - RevokePermission：回收/幂等（不存在 Revoked=0）/审计 revoke_permission/
//     撤权即时生效；
//   - ListPermissionGrants：主体回显（user 与 org 两口径）；
//   - PermissionOverview：direct/viaOrgs/roles 三段总览；
//   - MigrateFromRules：规则 × 角色 assignee 元组 → permission 元组，幂等。
package logic

import (
	"context"
	"reflect"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestGrantPermissionsValidation 参数校验矩阵：targetType 非法 / targetId 空 /
// 空权限列表 / 非法权限码（缺冒号、大写、空白）→ 400；org 不存在 → 404；
// 身份缺失 → 401。
func TestGrantPermissionsValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	cases := []struct {
		name string
		req  *types.GrantPermissionsReq
		want int
	}{
		{"targetType 非法", &types.GrantPermissionsReq{TargetType: "team", TargetId: "u1", Permissions: []string{"redis:read"}}, 400},
		{"targetId 空", &types.GrantPermissionsReq{TargetType: "user", Permissions: []string{"redis:read"}}, 400},
		{"空权限列表", &types.GrantPermissionsReq{TargetType: "user", TargetId: "u1"}, 400},
		{"缺冒号", &types.GrantPermissionsReq{TargetType: "user", TargetId: "u1", Permissions: []string{"redis"}}, 400},
		{"大写非法", &types.GrantPermissionsReq{TargetType: "user", TargetId: "u1", Permissions: []string{"Redis:read"}}, 400},
		{"非法段", &types.GrantPermissionsReq{TargetType: "user", TargetId: "u1", Permissions: []string{"re dis:read"}}, 400},
		{"org 不存在", &types.GrantPermissionsReq{TargetType: "org", TargetId: "no-such", Permissions: []string{"redis:read"}}, 404},
	}
	for _, c := range cases {
		_, err := NewGrantPermissionsLogic(ctx, sc).GrantPermissions(c.req)
		e := kiterrors.From(err)
		if e == nil || e.Status != c.want {
			t.Errorf("%s: 期望 %d, 实际 %v", c.name, c.want, err)
		}
	}

	// 身份缺失 → 401。
	if _, err := NewGrantPermissionsLogic(context.Background(), sc).GrantPermissions(
		&types.GrantPermissionsReq{TargetType: "user", TargetId: "u1", Permissions: []string{"redis:read"}},
	); kiterrors.From(err) == nil || kiterrors.From(err).Status != 401 {
		t.Errorf("身份缺失应 401, 实际 %v", err)
	}
}

// TestGrantAndRevokePermissions 授予 → me/permissions 即时可见（含组织继承）；
// 回收 → 即时不可见；幂等（重复授予/回收不存在均安全）；审计动作落库。
func TestGrantAndRevokePermissions(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	// user 直接授予。
	resp, err := NewGrantPermissionsLogic(ctx, sc).GrantPermissions(&types.GrantPermissionsReq{
		TargetType: "user", TargetId: "u1", Permissions: []string{"redis:read", "redis:manage", "redis:read"},
	})
	if err != nil {
		t.Fatalf("授予: %v", err)
	}
	if resp.Granted != 2 { // 重复码去重
		t.Errorf("Granted = %d, 期望 2", resp.Granted)
	}

	// org 授予：成员 u2 经 org#member 继承获得。
	rootID := createOrgViaLogic(t, sc, "总部", "")
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{OrgId: rootID, UserId: "u2"}); err != nil {
		t.Fatalf("加成员: %v", err)
	}
	if _, err := NewGrantPermissionsLogic(ctx, sc).GrantPermissions(&types.GrantPermissionsReq{
		TargetType: "org", TargetId: rootID, Permissions: []string{"order:read"},
	}); err != nil {
		t.Fatalf("授予组织: %v", err)
	}

	// 即时生效：不重签 token 直接查。
	for _, tc := range []struct{ sub, code string }{
		{"u1", "redis:read"}, {"u1", "redis:manage"}, {"u2", "order:read"},
	} {
		resp, err := NewMePermissionsLogic(userCtx(tc.sub, nil), sc).MePermissions()
		if err != nil {
			t.Fatalf("MePermissions(%s): %v", tc.sub, err)
		}
		found := false
		for _, p := range resp.Perms {
			if p == tc.code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s 应即时获得 %s, 实际 %v", tc.sub, tc.code, resp.Perms)
		}
	}

	// ListPermissionGrants 回显（user 与 org 两口径，只含直接授予）。
	ug, err := NewListPermissionGrantsLogic(ctx, sc).ListPermissionGrants(&types.ListPermissionGrantsReq{
		TargetType: "user", TargetId: "u1",
	})
	if err != nil {
		t.Fatalf("ListGrants(user): %v", err)
	}
	if want := []string{"redis:manage", "redis:read"}; !reflect.DeepEqual(ug.Codes, want) {
		t.Errorf("user grants = %v, 期望 %v", ug.Codes, want)
	}
	og, err := NewListPermissionGrantsLogic(ctx, sc).ListPermissionGrants(&types.ListPermissionGrantsReq{
		TargetType: "org", TargetId: rootID,
	})
	if err != nil {
		t.Fatalf("ListGrants(org): %v", err)
	}
	if want := []string{"order:read"}; !reflect.DeepEqual(og.Codes, want) {
		t.Errorf("org grants = %v, 期望 %v", og.Codes, want)
	}

	// 幂等：重复授予不报错，元组数不增。
	if _, err := NewGrantPermissionsLogic(ctx, sc).GrantPermissions(&types.GrantPermissionsReq{
		TargetType: "user", TargetId: "u1", Permissions: []string{"redis:read"},
	}); err != nil {
		t.Fatalf("重复授予应幂等: %v", err)
	}

	// 回收：即时不可见；不存在回收 Revoked=0 幂等。
	rr, err := NewRevokePermissionLogic(ctx, sc).RevokePermission(&types.RevokePermissionReq{
		TargetType: "user", TargetId: "u1", Permission: "redis:manage",
	})
	if err != nil || rr.Revoked != 1 {
		t.Fatalf("回收: resp=%v err=%v", rr, err)
	}
	rr2, err := NewRevokePermissionLogic(ctx, sc).RevokePermission(&types.RevokePermissionReq{
		TargetType: "user", TargetId: "u1", Permission: "redis:manage",
	})
	if err != nil || rr2.Revoked != 0 {
		t.Fatalf("重复回收应 Revoked=0: resp=%v err=%v", rr2, err)
	}
	mresp, err := NewMePermissionsLogic(userCtx("u1", nil), sc).MePermissions()
	if err != nil {
		t.Fatalf("MePermissions(撤权后): %v", err)
	}
	for _, p := range mresp.Perms {
		if p == "redis:manage" {
			t.Errorf("撤权后不应仍含 redis:manage: %v", mresp.Perms)
		}
	}

	// 审计：授予/回收动作落 committed。
	for _, action := range []string{"grant_permission", "revoke_permission"} {
		e := auditOf(t, sc, action)
		if e.Status != repo.AuditCommitted || e.Operator != "admin" {
			t.Errorf("审计 %s = %+v, 期望 committed 且 operator=admin", action, e)
		}
	}

	// 引擎故障 → 授予 503 fail-closed（引擎句柄已停）。
	sc.Engine.Stop()
	if _, err := NewGrantPermissionsLogic(ctx, sc).GrantPermissions(&types.GrantPermissionsReq{
		TargetType: "user", TargetId: "u1", Permissions: []string{"redis:read"},
	}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 503 {
		t.Errorf("引擎故障应 503, 实际 %v", err)
	}
}

// TestPermissionOverview 总览三段：direct（直接授予）/ viaOrgs（部门授予，
// orgName 回退）/ roles（过渡期角色展示）。
func TestPermissionOverview(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()
	bg := context.Background()

	// u1：直接授予两码 + 属于 root 组织（组织授予一码）+ 有角色元组。
	if err := sc.Engine.WriteTuples(bg, []engine.Tuple{
		{User: "user:u1", Relation: "can", Object: "permission:redis.read"},
		{User: "user:u1", Relation: "can", Object: "permission:*.read"},
		{User: "user:u1", Relation: "assignee", Object: "platform_role:operator"},
	}); err != nil {
		t.Fatalf("预置元组: %v", err)
	}
	rootID := createOrgViaLogic(t, sc, "总部", "")
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{OrgId: rootID, UserId: "u1"}); err != nil {
		t.Fatalf("加成员: %v", err)
	}
	if err := sc.Engine.WriteTuples(bg, []engine.Tuple{
		{User: "org:" + rootID + "#member", Relation: "can", Object: "permission:order.read"},
	}); err != nil {
		t.Fatalf("预置组织授权: %v", err)
	}

	ov, err := NewPermissionOverviewLogic(ctx, sc).PermissionOverview(&types.PermissionOverviewReq{UserId: "u1"})
	if err != nil {
		t.Fatalf("PermissionOverview: %v", err)
	}
	if want := []string{"*:read", "redis:read"}; !reflect.DeepEqual(ov.Direct, want) {
		t.Errorf("direct = %v, 期望 %v", ov.Direct, want)
	}
	if len(ov.ViaOrgs) != 1 || ov.ViaOrgs[0].OrgId != rootID || ov.ViaOrgs[0].OrgName != "总部" {
		t.Errorf("viaOrgs = %+v, 期望含组织 %s（总部）", ov.ViaOrgs, rootID)
	}
	if want := []string{"order:read"}; !reflect.DeepEqual(ov.ViaOrgs[0].Codes, want) {
		t.Errorf("viaOrgs codes = %v, 期望 %v", ov.ViaOrgs[0].Codes, want)
	}
	if want := []string{"operator"}; !reflect.DeepEqual(ov.Roles, want) {
		t.Errorf("roles = %v, 期望 %v", ov.Roles, want)
	}

	// userId 缺失 → 400。
	if _, err := NewPermissionOverviewLogic(ctx, sc).PermissionOverview(&types.PermissionOverviewReq{}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("userId 空应 400, 实际 %v", err)
	}
}

// TestMigrateFromRules 迁移矩阵：规则（服务级→`服务:read`，动作级→`服务:动作`）
// × 角色 assignee 元组（user 与 org#member 两形态）→ permission 元组；幂等。
func TestMigrateFromRules(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()
	bg := context.Background()

	// 规则：operator × order:（服务级）+ order:delete；readonly × user:read。
	// ID 留 0 由 repo 自增生成（迁移只读三元组，不依赖具体 ID 值）。
	for _, r := range []repo.Rule{
		{RoleKey: "operator", Prefix: "order:"},
		{RoleKey: "operator", Prefix: "order:", Action: "delete"},
		{RoleKey: "readonly", Prefix: "user:", Action: "read"},
	} {
		if _, err := sc.RuleRepo.Upsert(bg, r); err != nil {
			t.Fatalf("预置规则 %+v: %v", r, err)
		}
	}
	// 角色授予：operator → u1（user 形态）与 root 组织（org#member 形态，成员 u2）；
	// readonly → u3。
	rootID := createOrgViaLogic(t, sc, "总部", "")
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{OrgId: rootID, UserId: "u2"}); err != nil {
		t.Fatalf("加成员: %v", err)
	}
	if err := sc.Engine.WriteTuples(bg, []engine.Tuple{
		{User: "user:u1", Relation: "assignee", Object: "platform_role:operator"},
		{User: "org:" + rootID + "#member", Relation: "assignee", Object: "platform_role:operator"},
		{User: "user:u3", Relation: "assignee", Object: "platform_role:readonly"},
	}); err != nil {
		t.Fatalf("预置角色元组: %v", err)
	}

	mr, err := NewMigrateFromRulesLogic(ctx, sc).MigrateFromRules()
	if err != nil {
		t.Fatalf("迁移: %v", err)
	}
	if mr.Rules != 3 {
		t.Errorf("Rules = %d, 期望 3", mr.Rules)
	}
	// 期望元组：u1×{order:read, order:delete} + root组织×{order:read, order:delete} + u3×{user:read} = 5。
	if mr.Written != 5 {
		t.Errorf("Written = %d, 期望 5", mr.Written)
	}

	// 迁移结果经判定面即时可见。
	for _, tc := range []struct {
		sub  string
		code string
	}{
		{"u1", "order:read"}, {"u1", "order:delete"}, {"u2", "order:read"}, {"u3", "user:read"},
	} {
		resp, err := NewMePermissionsLogic(userCtx(tc.sub, nil), sc).MePermissions()
		if err != nil {
			t.Fatalf("MePermissions(%s): %v", tc.sub, err)
		}
		found := false
		for _, p := range resp.Perms {
			if p == tc.code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s 迁移后应获得 %s, 实际 %v", tc.sub, tc.code, resp.Perms)
		}
	}

	// 幂等：重复迁移 Written 仍 5（读后写跳过已存在），引擎不重复。
	mr2, err := NewMigrateFromRulesLogic(ctx, sc).MigrateFromRules()
	if err != nil {
		t.Fatalf("重复迁移: %v", err)
	}
	if mr2.Written != 5 {
		t.Errorf("重复迁移 Written = %d, 期望 5（提交口径不变）", mr2.Written)
	}

	// 审计：migrate_rules committed。
	e := auditOf(t, sc, "migrate_rules")
	if e.Status != repo.AuditCommitted || e.Operator != "admin" {
		t.Errorf("migrate_rules 审计 = %+v, 期望 committed 且 operator=admin", e)
	}

	// 空规则表：Written=0 不动引擎（新上下文验证）。
	sc2 := newTestSvcCtx(t)
	mr3, err := NewMigrateFromRulesLogic(ctx, sc2).MigrateFromRules()
	if err != nil || mr3.Written != 0 || mr3.Rules != 0 {
		t.Errorf("空规则迁移应 Written=0 Rules=0: resp=%v err=%v", mr3, err)
	}
}
