// orglogic_test.go 覆盖组织域 logic:创建成功/必填校验/环检测(409)/
// 未存在(404)/改名与换父/删除/列表合并(引擎可见 + 注册表)。
package logic

import (
	"context"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestCreateOrgSuccess 根组织与子组织创建:parent 元组落引擎、
// 审计 pending→committed、operator=claims.sub。
func TestCreateOrgSuccess(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	root, err := NewCreateOrgLogic(ctx, sc).CreateOrg(&types.CreateOrgReq{Name: "总部"})
	if err != nil {
		t.Fatalf("建根组织: %v", err)
	}
	if root.ParentId != "" {
		t.Errorf("根组织 parentId 应为空, 实际 %q", root.ParentId)
	}

	child, err := NewCreateOrgLogic(ctx, sc).CreateOrg(&types.CreateOrgReq{
		Name:     "华东分公司",
		ParentId: root.Id,
	})
	if err != nil {
		t.Fatalf("建子组织: %v", err)
	}
	if child.ParentId != root.Id {
		t.Errorf("子组织 parentId = %q, 期望 %q", child.ParentId, root.Id)
	}

	// 引擎里应有 child 的 parent 元组。
	got, err := sc.Engine.ReadTuples(ctx, "org:"+child.Id, "parent")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	if len(got) != 1 || got[0].User != "org:"+root.Id {
		t.Errorf("parent 元组 = %v, 期望 user=org:%s", got, root.Id)
	}

	// 两条 create_org 审计均 committed,operator 为 claims.sub。
	for range 2 {
		e := auditOf(t, sc, "create_org")
		if e.Status != repo.AuditCommitted {
			t.Errorf("create_org 审计状态 = %q, 期望 committed", e.Status)
		}
		if e.Operator != "admin" {
			t.Errorf("审计 operator = %q, 期望 admin(claims.sub)", e.Operator)
		}
	}
	if auditCount(t, sc) != 2 {
		t.Errorf("审计条目 = %d, 期望 2", auditCount(t, sc))
	}
}

// TestCreateOrgValidation 生成器语义下请求体全 optional:
// name 空 → 400 AUTHZ_ORG_INVALID;无身份 → 401。
func TestCreateOrgValidation(t *testing.T) {
	sc := newTestSvcCtx(t)

	_, err := NewCreateOrgLogic(adminCtx(), sc).CreateOrg(&types.CreateOrgReq{Name: "  "})
	e := kiterrors.From(err)
	if e == nil || e.Status != 400 || e.Code != "AUTHZ_ORG_INVALID" {
		t.Errorf("空 name 应 400 AUTHZ_ORG_INVALID, 实际 %v", err)
	}

	_, err = NewCreateOrgLogic(context.Background(), sc).CreateOrg(&types.CreateOrgReq{Name: "x"})
	e = kiterrors.From(err)
	if e == nil || e.Status != 401 {
		t.Errorf("无身份应 401, 实际 %v", err)
	}

	if auditCount(t, sc) != 0 {
		t.Errorf("校验失败不应产生审计条目, 实际 %d 条", auditCount(t, sc))
	}
}

// TestUpdateOrgCycle 换父环检测:root 的 parent 改为其子孙 → 409 AUTHZ_ORG_CYCLE;
// 自指为父同样 409。
func TestUpdateOrgCycle(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	childID := createOrgViaLogic(t, sc, "华东分公司", rootID)

	// root 的 parent 指向自己的子孙 child → 环。
	_, err := NewUpdateOrgLogic(ctx, sc).UpdateOrg(&types.UpdateOrgReq{
		Id:       rootID,
		ParentId: childID,
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 409 || e.Code != "AUTHZ_ORG_CYCLE" {
		t.Errorf("环应 409 AUTHZ_ORG_CYCLE, 实际 %v", err)
	}

	// 自指为父 → 环。
	_, err = NewUpdateOrgLogic(ctx, sc).UpdateOrg(&types.UpdateOrgReq{
		Id:       childID,
		ParentId: childID,
	})
	e = kiterrors.From(err)
	if e == nil || e.Status != 409 || e.Code != "AUTHZ_ORG_CYCLE" {
		t.Errorf("自指应 409 AUTHZ_ORG_CYCLE, 实际 %v", err)
	}

	// 环拒绝后引擎状态不应被破坏:child 的 parent 仍是 root。
	got, err := sc.Engine.ReadTuples(ctx, "org:"+childID, "parent")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	if len(got) != 1 || got[0].User != "org:"+rootID {
		t.Errorf("环拒绝后 parent 元组被破坏: %v", got)
	}
}

// TestUpdateOrgNotFound 更新不存在的组织 → 404 AUTHZ_ORG_NOT_FOUND。
func TestUpdateOrgNotFound(t *testing.T) {
	sc := newTestSvcCtx(t)

	_, err := NewUpdateOrgLogic(adminCtx(), sc).UpdateOrg(&types.UpdateOrgReq{
		Id:   "org-does-not-exist",
		Name: "新名",
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 404 || e.Code != "AUTHZ_ORG_NOT_FOUND" {
		t.Errorf("不存在组织应 404 AUTHZ_ORG_NOT_FOUND, 实际 %v", err)
	}
}

// TestUpdateOrgRenameAndReparent 改名 + 换父:旧 parent 元组删除、新元组写入,
// 返回值与注册表一致。
func TestUpdateOrgRenameAndReparent(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	otherID := createOrgViaLogic(t, sc, "华南总部", "")
	childID := createOrgViaLogic(t, sc, "华东分公司", rootID)

	resp, err := NewUpdateOrgLogic(ctx, sc).UpdateOrg(&types.UpdateOrgReq{
		Id:       childID,
		Name:     "华东大区",
		ParentId: otherID,
	})
	if err != nil {
		t.Fatalf("更新组织: %v", err)
	}
	if resp.Name != "华东大区" || resp.ParentId != otherID {
		t.Errorf("更新返回 = %+v, 期望 name=华东大区 parent=%s", resp, otherID)
	}

	// 旧 parent 元组已删、新 parent 元组在位。
	got, err := sc.Engine.ReadTuples(ctx, "org:"+childID, "parent")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	if len(got) != 1 || got[0].User != "org:"+otherID {
		t.Errorf("换父后 parent 元组 = %v, 期望 user=org:%s", got, otherID)
	}

	// 改名后 ListOrgs 反映新名。
	listResp, err := NewListOrgsLogic(ctx, sc).ListOrgs()
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	for _, o := range listResp.Items {
		if o.Id == childID && o.Name != "华东大区" {
			t.Errorf("ListOrgs 组织名 = %q, 期望 华东大区", o.Name)
		}
	}

	// 审计 committed。
	if e := auditOf(t, sc, "update_org"); e.Status != repo.AuditCommitted {
		t.Errorf("update_org 审计状态 = %q, 期望 committed", e.Status)
	}
}

// TestDeleteOrg 删除组织:parent 与 member 元组一并清理、注册表移除、审计 committed。
func TestDeleteOrg(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	childID := createOrgViaLogic(t, sc, "华东分公司", rootID)
	if _, err := NewAddMemberLogic(ctx, sc).AddMember(&types.MemberReq{
		OrgId:  childID,
		UserId: "u1",
	}); err != nil {
		t.Fatalf("加成员: %v", err)
	}

	if err := NewDeleteOrgLogic(ctx, sc).DeleteOrg(&types.OrgIdReq{Id: childID}); err != nil {
		t.Fatalf("删除组织: %v", err)
	}

	// 引擎中该组织的全部元组应清空。
	got, err := sc.Engine.ReadTuples(ctx, "org:"+childID, "")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("删除后 org 元组应为空, 实际 %v", got)
	}

	// 再删 → 404。
	err = NewDeleteOrgLogic(ctx, sc).DeleteOrg(&types.OrgIdReq{Id: childID})
	if e := kiterrors.From(err); e == nil || e.Status != 404 {
		t.Errorf("重复删除应 404, 实际 %v", err)
	}

	if e := auditOf(t, sc, "delete_org"); e.Status != repo.AuditCommitted {
		t.Errorf("delete_org 审计状态 = %q, 期望 committed", e.Status)
	}
}

// TestListOrgsMergesEngineAndRegistry 引擎元组可见的组织(如 bootstrap 的
// org:root,未走 API 注册表)也出现在列表中;API 建的组织带注册表名称。
func TestListOrgsMergesEngineAndRegistry(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	// 模拟 bootstrap:直接向引擎写 org:root 的成员元组(不经 API)。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "user:admin", Relation: "member", Object: "org:root"},
	}); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}

	childID := createOrgViaLogic(t, sc, "华东分公司", "root")

	resp, err := NewListOrgsLogic(ctx, sc).ListOrgs()
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("组织总数 = %d, 期望 2(root + child): %v", resp.Total, resp.Items)
	}
	byID := map[string]types.OrgResp{}
	for _, o := range resp.Items {
		byID[o.Id] = o
	}
	// root 来自引擎(名称缺省为 ID),child 来自 API(带名称与父子关系)。
	if _, ok := byID["root"]; !ok {
		t.Errorf("引擎可见的 org:root 应出现在列表: %v", resp.Items)
	}
	if c, ok := byID[childID]; !ok || c.Name != "华东分公司" || c.ParentId != "root" {
		t.Errorf("child 组织信息 = %+v, 期望 name=华东分公司 parent=root", c)
	}
}

// TestUpdateOrgMultiParentCycle 多父场景防环:parent 是集合语义,B 部同时挂
// X 部与 A 部两条 parent 元组,X 链(X→Y→root)不含 A——单链遍历取任一条
// (如先插入的 X 链)会绕过环检测,必须全量遍历全部 parent 边。
func TestUpdateOrgMultiParentCycle(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	yID := createOrgViaLogic(t, sc, "Y部", rootID)
	xID := createOrgViaLogic(t, sc, "X部", yID)
	aID := createOrgViaLogic(t, sc, "A部", rootID)
	bID := createOrgViaLogic(t, sc, "B部", xID) // B 的第一条 parent 元组:X(不含 A 的链)

	// 直接往引擎补第二条 parent 元组(B←A):A 侧成环。
	// 先插 X 链后插 A 链——单链遍历若只走先插入的 X 链即漏判本环。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:" + aID, Relation: "parent", Object: "org:" + bID},
	}); err != nil {
		t.Fatalf("写第二条 parent 元组: %v", err)
	}

	// A 的 parent 改为 B:B 的祖先沿 A 链含 A 自己 → 409。
	_, err := NewUpdateOrgLogic(ctx, sc).UpdateOrg(&types.UpdateOrgReq{
		Id:       aID,
		ParentId: bID,
	})
	e := kiterrors.From(err)
	if e == nil || e.Status != 409 || e.Code != "AUTHZ_ORG_CYCLE" {
		t.Errorf("多父环应 409 AUTHZ_ORG_CYCLE, 实际 %v", err)
	}

	// 环拒绝后引擎状态不被破坏:B 的 parent 元组仍是 {X, A} 两条。
	got, err := sc.Engine.ReadTuples(ctx, "org:"+bID, "parent")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	users := map[string]bool{}
	for _, tu := range got {
		users[tu.User] = true
	}
	if len(got) != 2 || !users["org:"+aID] || !users["org:"+xID] {
		t.Errorf("环拒绝后 B 的 parent 元组应为 {{X},{A}} 两条, 实际 %v", got)
	}
}

// TestUpdateOrgReparentUsesEngineTruth 重启漂移后换父以引擎现值为删除依据:
// 引擎存在注册表不知道的 parent 元组(模拟内存注册表重启丢失)时,换父必须
// 把引擎里全部旧 parent 元组删净——按注册表删除会漏删,制造多父残留。
func TestUpdateOrgReparentUsesEngineTruth(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	rootID := createOrgViaLogic(t, sc, "总部", "")
	otherID := createOrgViaLogic(t, sc, "华南总部", rootID)
	childID := createOrgViaLogic(t, sc, "华东分公司", rootID)
	newID := createOrgViaLogic(t, sc, "西南总部", rootID)

	// 模拟重启漂移:直接往引擎写注册表不知道的第二条 parent 元组(←other)。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:" + otherID, Relation: "parent", Object: "org:" + childID},
	}); err != nil {
		t.Fatalf("写漂移 parent 元组: %v", err)
	}

	// 换父到 new。
	if _, err := NewUpdateOrgLogic(ctx, sc).UpdateOrg(&types.UpdateOrgReq{
		Id:       childID,
		ParentId: newID,
	}); err != nil {
		t.Fatalf("换父: %v", err)
	}

	// 引擎里 child 的 parent 元组必须只剩 new 一条(root 与 other 均被删净)。
	got, err := sc.Engine.ReadTuples(ctx, "org:"+childID, "parent")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	if len(got) != 1 || got[0].User != "org:"+newID {
		t.Errorf("换父后应无多父残留(仅 %s), 实际 %v", newID, got)
	}
}
