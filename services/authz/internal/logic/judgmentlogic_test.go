// judgmentlogic_test.go 覆盖判定面四个 logic：Check（200{false} 语义、
// 引擎故障 503 fail-closed）、ListObjects（同上 + 未注册类型走引擎错误路径）、
// MePermissions（去角色化改造：引擎实时查询矩阵——直接授予/组织继承/通配/
// 撤权即时生效）、WriteRelations（幂等重复写不报错 + 审计 operator=sub、
// service 缺省 unknown）。
package logic

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	authzkit "nemo/kit/authz"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// userCtx 构造带指定 sub 与 roles 的 ctx（生产由 authzkit.Identity 中间件注入）。
func userCtx(sub string, roles []string) context.Context {
	return authzkit.WithIdentity(context.Background(), sub, roles)
}

// TestCheckAllowedAndDenied 判定语义：有关系 → 200 {allowed:true}；
// 无关系（明确无权）→ 同样 200 不报错 {allowed:false}——"无权"不是错误，
// 业务层 403 由 kit client / 前端转译。
func TestCheckAllowedAndDenied(t *testing.T) {
	sc := newTestSvcCtx(t)

	rootID := createOrgViaLogic(t, sc, "总部", "")
	if _, err := NewAddMemberLogic(adminCtx(), sc).AddMember(&types.MemberReq{
		OrgId:  rootID,
		UserId: "u1",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}

	// u1 是成员 → allowed=true。
	resp, err := NewCheckLogic(userCtx("u1", nil), sc).Check(&types.CheckReq{
		Object:   "org:" + rootID,
		Relation: "member",
	})
	if err != nil {
		t.Fatalf("u1 Check: %v", err)
	}
	if !resp.Allowed {
		t.Errorf("u1 对组织 member 判定应 allowed=true")
	}

	// u2 无关系 → 明确 false = 200 {allowed:false}，不返回错误。
	resp, err = NewCheckLogic(userCtx("u2", nil), sc).Check(&types.CheckReq{
		Object:   "org:" + rootID,
		Relation: "member",
	})
	if err != nil {
		t.Fatalf("u2 Check（明确无权应为 200 不报错）: %v", err)
	}
	if resp.Allowed {
		t.Errorf("u2 对组织 member 判定应 allowed=false")
	}
}

// TestCheckValidation 必填自校验：Object/Relation 空 → 400；身份缺失 → 401 fail-closed。
func TestCheckValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := userCtx("u1", nil)

	// Object 空 → 400。
	if _, err := NewCheckLogic(ctx, sc).Check(&types.CheckReq{Relation: "member"}); err == nil {
		t.Fatalf("Object 空应报错")
	} else if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("Object 空应 400, 实际 %v", err)
	}
	// Relation 空 → 400。
	if _, err := NewCheckLogic(ctx, sc).Check(&types.CheckReq{Object: "org:o1"}); err == nil {
		t.Fatalf("Relation 空应报错")
	} else if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("Relation 空应 400, 实际 %v", err)
	}
	// 身份缺失 → 401 fail-closed。
	_, err := NewCheckLogic(context.Background(), sc).Check(&types.CheckReq{
		Object: "org:o1", Relation: "member",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 401 {
		t.Errorf("身份缺失应 401, 实际 %v", err)
	}
}

// TestCheckEngineFailure503 引擎故障（停掉引擎端口模拟）→ 503 AUTHZ_ENGINE_ERROR
// fail-closed，绝不把引擎不可达判成"无权"。
func TestCheckEngineFailure503(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Engine.Stop() // 关掉引擎端口模拟引擎故障

	_, err := NewCheckLogic(userCtx("u1", nil), sc).Check(&types.CheckReq{
		Object: "org:o1", Relation: "member",
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 503 || e.Code != "AUTHZ_ENGINE_ERROR" {
		t.Errorf("引擎故障应 503 AUTHZ_ENGINE_ERROR, 实际 %v", err)
	}
}

// TestListObjectsListsAndEmpty ListObjects：返回 user 可见对象 id 数组；
// 无可见对象时返回空数组（非 null）。
func TestListObjectsListsAndEmpty(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := userCtx("u1", nil)

	if err := sc.Engine.WriteTuples(context.Background(), []engine.Tuple{
		{User: "user:u1", Relation: "viewer", Object: "order:o1"},
		{User: "user:u1", Relation: "viewer", Object: "order:o2"},
	}); err != nil {
		t.Fatalf("预置 viewer 元组: %v", err)
	}

	resp, err := NewListObjectsLogic(ctx, sc).ListObjects(&types.ListObjectsReq{
		Type: "order", Relation: "can_view",
	})
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	// 引擎返回顺序不保证，按集合比较。
	want := map[string]bool{"order:o1": true, "order:o2": true}
	if len(resp.Objects) != len(want) {
		t.Fatalf("ListObjects = %v, 期望 %v", resp.Objects, want)
	}
	for _, o := range resp.Objects {
		if !want[o] {
			t.Errorf("ListObjects 含意外对象 %q", o)
		}
	}

	// 无可见对象 → 空数组（非 null）。
	resp2, err := NewListObjectsLogic(userCtx("u2", nil), sc).ListObjects(&types.ListObjectsReq{
		Type: "order", Relation: "can_view",
	})
	if err != nil {
		t.Fatalf("ListObjects(无权限用户): %v", err)
	}
	if resp2.Objects == nil || len(resp2.Objects) != 0 {
		t.Errorf("无可见对象应返回空数组, 实际 %v", resp2.Objects)
	}
}

// TestListObjectsValidationAndEngineFailure 校验与引擎故障路径：
// Type/Relation 空 → 400；身份缺失 → 401；引擎故障 → 503；
// 未注册资源类型（模型中不存在）经实测引擎返回 error → 按引擎错误 503
// fail-closed（决策：不区分"类型不存在"与"引擎故障"，一律拒绝，见 logic 注释）。
func TestListObjectsValidationAndEngineFailure(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := userCtx("u1", nil)

	if _, err := NewListObjectsLogic(ctx, sc).ListObjects(&types.ListObjectsReq{Relation: "can_view"}); err == nil {
		t.Fatalf("Type 空应报错")
	} else if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("Type 空应 400, 实际 %v", err)
	}
	if _, err := NewListObjectsLogic(ctx, sc).ListObjects(&types.ListObjectsReq{Type: "order"}); err == nil {
		t.Fatalf("Relation 空应报错")
	} else if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("Relation 空应 400, 实际 %v", err)
	}
	if _, err := NewListObjectsLogic(context.Background(), sc).ListObjects(&types.ListObjectsReq{
		Type: "order", Relation: "can_view",
	}); err == nil {
		t.Fatalf("身份缺失应报错")
	} else if e := kiterrors.From(err); e == nil || e.Status != 401 {
		t.Errorf("身份缺失应 401, 实际 %v", err)
	}

	// 引擎故障 → 503。
	sc2 := newTestSvcCtx(t)
	sc2.Engine.Stop()
	_, err := NewListObjectsLogic(userCtx("u1", nil), sc2).ListObjects(&types.ListObjectsReq{
		Type: "order", Relation: "can_view",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 503 || e.Code != "AUTHZ_ENGINE_ERROR" {
		t.Errorf("引擎故障应 503 AUTHZ_ENGINE_ERROR, 实际 %v", err)
	}

	// 未注册类型：引擎实测报 error（type not found）→ 同走 503 fail-closed。
	sc3 := newTestSvcCtx(t)
	_, err = NewListObjectsLogic(userCtx("u1", nil), sc3).ListObjects(&types.ListObjectsReq{
		Type: "nosuch", Relation: "can_view",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 503 || e.Code != "AUTHZ_ENGINE_ERROR" {
		t.Errorf("未注册类型应走引擎错误 503 fail-closed, 实际 %v", err)
	}
}

// TestMePermissionsEngineRealtime me/permissions 引擎实时查询矩阵（去角色化
// 改造）：直接授予 / 组织成员继承（含 parent 链）/ 通配对象 / 撤权即时生效。
func TestMePermissionsEngineRealtime(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := context.Background()

	// 直接授予：u1 = order:read + order:delete + 通配 *:read（跨服务同动作）。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "user:u1", Relation: "can", Object: "permission:order.read"},
		{User: "user:u1", Relation: "can", Object: "permission:order.delete"},
		{User: "user:u1", Relation: "can", Object: "permission:*.read"},
	}); err != nil {
		t.Fatalf("预置 u1 授权元组: %v", err)
	}
	// 组织授予：rootOrg 的 can = user:read → 成员 u2（含继承链）获得。
	rootID := createOrgViaLogic(t, sc, "总部", "")
	if _, err := NewAddMemberLogic(adminCtx(), sc).AddMember(&types.MemberReq{OrgId: rootID, UserId: "u2"}); err != nil {
		t.Fatalf("加成员: %v", err)
	}
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:" + rootID + "#member", Relation: "can", Object: "permission:user.read"},
	}); err != nil {
		t.Fatalf("预置组织授权元组: %v", err)
	}

	// u1：直接授予 + 通配码，升序去重；roles claim 已不参与（传任意角色结果一致）。
	resp, err := NewMePermissionsLogic(userCtx("u1", []string{"super_admin"}), sc).MePermissions()
	if err != nil {
		t.Fatalf("MePermissions: %v", err)
	}
	want := []string{"*:read", "order:delete", "order:read"}
	if !reflect.DeepEqual(resp.Perms, want) {
		t.Errorf("Perms = %v, 期望 %v", resp.Perms, want)
	}

	// u2：经组织成员继承（org#member 主体形态，引擎原生展开）。
	resp2, err := NewMePermissionsLogic(userCtx("u2", nil), sc).MePermissions()
	if err != nil {
		t.Fatalf("MePermissions(u2): %v", err)
	}
	if want2 := []string{"user:read"}; !reflect.DeepEqual(resp2.Perms, want2) {
		t.Errorf("Perms(u2) = %v, 期望 %v", resp2.Perms, want2)
	}

	// 撤权即时生效：删掉 u1 全部直接授予后立即为空（无需重新登录/重签 token）。
	if err := sc.Engine.DeleteTuples(ctx, []engine.Tuple{
		{User: "user:u1", Relation: "can", Object: "permission:order.read"},
		{User: "user:u1", Relation: "can", Object: "permission:order.delete"},
		{User: "user:u1", Relation: "can", Object: "permission:*.read"},
	}); err != nil {
		t.Fatalf("撤销 u1 授权: %v", err)
	}
	resp3, err := NewMePermissionsLogic(userCtx("u1", []string{"super_admin"}), sc).MePermissions()
	if err != nil {
		t.Fatalf("MePermissions(撤权后): %v", err)
	}
	if resp3.Perms == nil || len(resp3.Perms) != 0 {
		t.Errorf("撤权后应返回空数组, 实际 %v", resp3.Perms)
	}

	// 身份缺失 → 401。
	if _, err := NewMePermissionsLogic(context.Background(), sc).MePermissions(); err == nil {
		t.Fatalf("身份缺失应报错")
	} else if e := kiterrors.From(err); e == nil || e.Status != 401 {
		t.Errorf("身份缺失应 401, 实际 %v", err)
	}
}

// TestMePermissionsEngineFailure503 引擎故障（停掉引擎模拟）→ 503
// AUTHZ_ENGINE_ERROR fail-closed（去角色化改造：权限集合来自引擎实时查询）。
func TestMePermissionsEngineFailure503(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Engine.Stop()

	_, err := NewMePermissionsLogic(userCtx("u1", nil), sc).MePermissions()
	e := kiterrors.From(err)
	if e == nil || e.Status != 503 || e.Code != "AUTHZ_ENGINE_ERROR" {
		t.Errorf("引擎故障应 503 AUTHZ_ENGINE_ERROR, 实际 %v", err)
	}
}

// TestWriteRelationsIdempotentWithAudit 关系写接口：幂等写（重复提交不报错
// 不重复落审计）、审计 operator=sub、X-Service-Name 缺省 "unknown" 落快照。
func TestWriteRelationsIdempotentWithAudit(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := userCtx("svc-bot", []string{"operator"})

	req := &types.WriteRelationsReq{
		Source: "order-svc",
		Tuples: []types.TupleReq{
			{User: "user:u1", Relation: "owner", Object: "order:o1"},
		},
	}
	resp, err := NewWriteRelationsLogic(ctx, sc).WriteRelations(req)
	if err != nil {
		t.Fatalf("WriteRelations: %v", err)
	}
	if resp.Written != 1 {
		t.Errorf("Written = %d, 期望 1", resp.Written)
	}
	// 元组在引擎中生效。
	ok, err := sc.Engine.Check(context.Background(), "user:u1", "can_delete", "order:o1")
	if err != nil || !ok {
		t.Fatalf("owner 元组应使 can_delete 判定 true: ok=%v err=%v", ok, err)
	}

	// 审计：operator=sub，快照含 service=X-Service-Name。
	e := auditOf(t, sc, "write_relations")
	if e.Status != repo.AuditCommitted || e.Operator != "svc-bot" {
		t.Errorf("write_relations 审计 = %+v, 期望 committed 且 operator=svc-bot", e)
	}

	// 重复提交（同一元组）幂等不报错；审计不重复落（幂等跳过也记录一次提交动作，
	// 但引擎侧无重复数据——此处断言不报错即可）。
	if _, err := NewWriteRelationsLogic(ctx, sc).WriteRelations(req); err != nil {
		t.Errorf("重复 WriteRelations 应幂等不报错: %v", err)
	}
	ok, err = sc.Engine.Check(context.Background(), "user:u1", "can_delete", "order:o1")
	if err != nil || !ok {
		t.Errorf("重复写后判定仍应 true: ok=%v err=%v", ok, err)
	}
}

// TestWriteRelationsDefaultServiceAndValidation service 头缺省 unknown；
// 元组缺要素/空列表 → 400；身份缺失 → 401。
func TestWriteRelationsDefaultServiceAndValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := userCtx("svc-bot", nil)

	// Source 空 → service 记 "unknown"（落审计快照）。
	resp, err := NewWriteRelationsLogic(ctx, sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{
			{User: "user:u1", Relation: "owner", Object: "order:o9"},
		},
	})
	if err != nil {
		t.Fatalf("WriteRelations(无 Source): %v", err)
	}
	if resp.Written != 1 {
		t.Errorf("Written = %d, 期望 1", resp.Written)
	}
	e := auditOf(t, sc, "write_relations")
	if e.Operator != "svc-bot" {
		t.Errorf("审计 operator = %q, 期望 svc-bot", e.Operator)
	}

	// 元组缺 object → 400。
	if _, err := NewWriteRelationsLogic(ctx, sc).WriteRelations(&types.WriteRelationsReq{
		Source: "order-svc",
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "owner"}},
	}); err == nil {
		t.Fatalf("元组缺 object 应报错")
	} else if ke := kiterrors.From(err); ke == nil || ke.Status != 400 {
		t.Errorf("元组缺 object 应 400, 实际 %v", err)
	}

	// 空元组列表 → 400。
	if _, err := NewWriteRelationsLogic(ctx, sc).WriteRelations(&types.WriteRelationsReq{Source: "order-svc"}); err == nil {
		t.Fatalf("空元组列表应报错")
	} else if ke := kiterrors.From(err); ke == nil || ke.Status != 400 {
		t.Errorf("空元组列表应 400, 实际 %v", err)
	}

	// 身份缺失 → 401。
	if _, err := NewWriteRelationsLogic(context.Background(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "owner", Object: "order:o1"}},
	}); err == nil {
		t.Fatalf("身份缺失应报错")
	} else if ke := kiterrors.From(err); ke == nil || ke.Status != 401 {
		t.Errorf("身份缺失应 401, 实际 %v", err)
	}
}

// TestWriteRelationsWhitelist 写白名单护栏（终审 C1 负/正样本）：
// ① 普通用户（及超管——白名单是接口语义）写 platform_role assignee → 403；
// ② 写 org member → 403；③ 合法业务资源共享（order viewer / 已注册类型 sharer）
// → 200；④ 出口关系与未注册类型 → 403。拒绝发生在写路径前置，不落审计。
func TestWriteRelationsWhitelist(t *testing.T) {
	sc := newTestSvcCtx(t)
	plain := userCtx("attacker", nil)
	admin := userCtx("admin", []string{"super_admin"})

	// ① 自授平台角色：普通用户与超管都被拒（保留类型禁写）。
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"普通用户", plain},
		{"超管", admin},
	} {
		_, err := NewWriteRelationsLogic(tc.ctx, sc).WriteRelations(&types.WriteRelationsReq{
			Tuples: []types.TupleReq{{User: "user:me", Relation: "assignee", Object: "platform_role:super_admin"}},
		})
		e := kiterrors.From(err)
		if e.Status != http.StatusForbidden || e.Code != "AUTHZ_RELATION_FORBIDDEN" {
			t.Errorf("%s 写 platform_role assignee 应 403 AUTHZ_RELATION_FORBIDDEN: %v", tc.name, err)
		}
	}

	// ② 写组织管理面关系（org member）→ 403。
	_, err := NewWriteRelationsLogic(plain, sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:me", Relation: "member", Object: "org:root"}},
	})
	e := kiterrors.From(err)
	if e.Status != http.StatusForbidden || e.Code != "AUTHZ_RELATION_FORBIDDEN" {
		t.Errorf("写 org member 应 403 AUTHZ_RELATION_FORBIDDEN: %v", err)
	}

	// ③a 合法业务资源共享：order 的 viewer 关系 → 200（既有链路不回归）。
	resp, err := NewWriteRelationsLogic(plain, sc).WriteRelations(&types.WriteRelationsReq{
		Source: "order-svc",
		Tuples: []types.TupleReq{{User: "user:a", Relation: "viewer", Object: "order:o1"}},
	})
	if err != nil || resp.Written != 1 {
		t.Fatalf("order viewer 写入应 200: resp=%v err=%v", resp, err)
	}
	// ③b 已注册资源类型的 sharer → 200（经菜单派生路径：资源驱动菜单/按钮
	// 声明类型 → sync 落库 + 模型合并 + EnsureModel）。名单策略（grantpolicy）：
	// 派生保留存量 ManageCode，持有者可写该类型任意资源名单。
	// ModelFile 相对工作目录（测试 cwd 为 internal/logic，与既有注册测试同约定）。
	sc.Config.Engine.ModelFile = "../../model.fga"
	registerTypeViaMenus(t, sc, "report", []string{"can_view", "can_share"}, "report-tools:manage")
	grantPerm(t, sc, "attacker", "report-tools:manage")
	if _, err := NewWriteRelationsLogic(plain, sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:a", Relation: "sharer", Object: "report:r1"}},
	}); err != nil {
		t.Fatalf("已注册类型 sharer 写入应 200: %v", err)
	}
	// ③c 无管理码/无关系的普通用户写注册类型名单 → 403 AUTHZ_DENIED
	//（关系过写白名单，但资源管理权不满足——grantpolicy 校验生效）。
	if _, err := NewWriteRelationsLogic(userCtx("no-rights", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:a", Relation: "sharer", Object: "report:r2"}},
	}); err == nil {
		t.Fatal("无名单管理权的用户写注册类型应 403")
	} else if fe := kiterrors.From(err); fe.Status != http.StatusForbidden || fe.Code != "AUTHZ_DENIED" {
		t.Errorf("应 403 AUTHZ_DENIED: %v", err)
	}

	// ④ 出口关系（can_view 不可直接写）与未注册类型 → 403。
	// 负样本统一用独立操作人 denied-bot，便于断言"拒绝不落审计"。
	denied := userCtx("denied-bot", nil)
	for _, tc := range []types.TupleReq{
		{User: "user:a", Relation: "can_view", Object: "order:o1"},
		{User: "user:a", Relation: "owner", Object: "nosuchtype:o1"},
		{User: "user:a", Relation: "owner", Object: "user:b"},
	} {
		_, err := NewWriteRelationsLogic(denied, sc).WriteRelations(&types.WriteRelationsReq{Tuples: []types.TupleReq{tc}})
		if fe := kiterrors.From(err); fe.Status != http.StatusForbidden || fe.Code != "AUTHZ_RELATION_FORBIDDEN" {
			t.Errorf("元组 %+v 应 403 AUTHZ_RELATION_FORBIDDEN: %v", tc, err)
		}
	}

	// 拒绝发生在写路径前置：denied-bot 全程无审计落库（positive 落库不影响）。
	if got := auditOfOpt(t, sc, "denied-bot"); got != "" {
		t.Errorf("白名单拒绝不应落审计: %s", got)
	}
}

// auditOfOpt 按操作人取首条审计动作，无条目返回空串（供负样本断言）。
func auditOfOpt(t *testing.T, sc *svc.ServiceContext, operator string) string {
	t.Helper()
	entries, _, err := sc.AuditRepo.Query(context.Background(), repo.AuditFilter{Operator: operator})
	if err != nil {
		t.Fatalf("查询审计: %v", err)
	}
	if len(entries) == 0 {
		return ""
	}
	return entries[0].Action
}
