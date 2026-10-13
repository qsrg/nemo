// abaclogic_test.go 覆盖 ABAC allow 正向门槛（方案 A 扩展）与通用判定面叠加：
//   - 资源粒度门槛（resourceAttr 形式）：仅设置了该属性的资源受限，
//     属性值满足才放行，缺属性/不满足均被挡；
//   - 整型门槛（字面量形式）：该资源类型全部资源受限，含管理码持有者；
//   - deny 优先于 allow：同时命中仍拒绝；
//   - 通用判定面 /v1/authz/check 自动叠加（后续业务域零接线）。
package logic

import (
	"context"
	"testing"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestABACAllowGateResourceScoped 资源粒度门槛：allow 策略取
// resourceAttr 形式（user.bizLine eq resource.allowedBizLine）——仅设置了
// allowedBizLine 属性的资源受限；属性匹配的用户可见、不匹配与缺属性的
// 用户均不可见；未设置该属性的资源不受门槛约束。
func TestABACAllowGateResourceScoped(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	t1 := createTopicViaLogic(t, sc, "trade-events")
	t2 := createTopicViaLogic(t, sc, "public-events")
	o1, o2 := "kafka_topic:"+t1.Id, "kafka_topic:"+t2.Id

	// t1 上门槛：allowedBizLine=trade；t2 不设。
	if err := sc.AttributeRepo.Set(context.Background(), repo.Attribute{Owner: o1, AttrKey: "allowedBizLine", AttrValue: "trade"}); err != nil {
		t.Fatalf("set resource attr: %v", err)
	}
	// 主体属性：u1=trade（过门槛）、u2=pay（不过）、u3 不配属性。
	for _, s := range []struct{ id, biz string }{{"u1", "trade"}, {"u2", "pay"}} {
		if err := sc.AttributeRepo.Set(context.Background(), repo.Attribute{Owner: "user:" + s.id, AttrKey: "bizLine", AttrValue: s.biz}); err != nil {
			t.Fatalf("set subject attr: %v", err)
		}
	}
	// 三个用户都授 t1/t2 的 viewer。
	for _, u := range []string{"u1", "u2", "u3"} {
		for _, o := range []string{o1, o2} {
			if _, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
				Tuples: []types.TupleReq{{User: "user:" + u, Relation: "viewer", Object: o}},
			}); err != nil {
				t.Fatalf("授 viewer %s %s: %v", u, o, err)
			}
		}
	}

	// allow 门槛策略（resourceAttr 形式）。
	if _, err := sc.PolicyRepo.Create(context.Background(), repo.Policy{
		Name: "业务线门槛", ResourceType: "kafka_topic", Effect: repo.EffectAllow,
		SubjectAttr: "bizLine", Op: repo.OpEq, ResourceAttr: "allowedBizLine", Enabled: true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	totalAs := func(sub string) int {
		t.Helper()
		resp, err := NewListObjectsLogic(ctxAs(sub, nil), sc).ListObjects(&types.ListObjectsReq{Type: "kafka_topic", Relation: "can_view"})
		if err != nil {
			t.Fatalf("%s 列表: %v", sub, err)
		}
		return len(resp.Objects)
	}
	if n := totalAs("u1"); n != 2 {
		t.Errorf("u1(trade) 应见 2 条（t1 过门槛 + t2 无门槛）: %d", n)
	}
	if n := totalAs("u2"); n != 1 {
		t.Errorf("u2(pay) 应仅见 t2（t1 被门槛挡下）: %d", n)
	}
	if n := totalAs("u3"); n != 1 {
		t.Errorf("u3(缺 bizLine) 应仅见 t2（缺属性过不了门槛）: %d", n)
	}
}

// TestABACAllowGateTypeWide 整型门槛：allow 策略取字面量形式
// （user.bizLine eq "trade"，resourceType=topic）——该类型全部资源受限，
// 管理码持有者同样过不了门槛（高敏感隔离优先于页面级授权）。
func TestABACAllowGateTypeWide(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	createTopicViaLogic(t, sc, "walled-topic")

	if _, err := sc.PolicyRepo.Create(context.Background(), repo.Policy{
		Name: "仅 trade 可见", ResourceType: "kafka_topic", Effect: repo.EffectAllow,
		SubjectAttr: "bizLine", Op: repo.OpEq, Value: "trade", Enabled: true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// admin（管理码全权持有者）未配 bizLine → 整型门槛挡下。
	resp, err := NewListObjectsLogic(adminCtx(), sc).ListObjects(&types.ListObjectsReq{Type: "kafka_topic", Relation: "can_view"})
	if err != nil {
		t.Fatalf("admin 列表: %v", err)
	}
	if len(resp.Objects) != 0 {
		t.Errorf("整型门槛对管理码持有者同样生效，admin 应见 0 条: %d", len(resp.Objects))
	}

	// 编辑链路同样被门槛拦截：owner（创建人）编辑自己的 topic 也 403——
	// 改测通用判定面 Check(can_edit) 的 ABAC 叠加（kafka 服务编辑判定
	// 即经 authz check 完成，语义等价）。
	topic := createTopicViaLogic(t, sc, "owner-topic")
	resp2, err := NewCheckLogic(adminCtx(), sc).Check(&types.CheckReq{Object: "kafka_topic:" + topic.Id, Relation: "can_edit"})
	if err != nil {
		t.Fatalf("check can_edit: %v", err)
	}
	if resp2.Allowed {
		t.Error("owner 编辑应被整型门槛拒绝（Check 返回 false）")
	}
}

// TestABACDenyOverridesAllow deny 优先于 allow：主体命中 allow 的同时命中
// deny，仍拒绝——deny 是硬约束，allow 门槛通过不能抵消。
func TestABACDenyOverridesAllow(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "dual-policy")
	object := "kafka_topic:" + topic.Id

	// u1：securityLevel=2（低等级）、bizLine=trade（业务线达标）。
	for _, a := range []repo.Attribute{
		{Owner: "user:u1", AttrKey: "securityLevel", AttrValue: "2"},
		{Owner: "user:u1", AttrKey: "bizLine", AttrValue: "trade"},
		{Owner: object, AttrKey: "securityLevel", AttrValue: "4"},
	} {
		if err := sc.AttributeRepo.Set(context.Background(), a); err != nil {
			t.Fatalf("set attr: %v", err)
		}
	}
	if _, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "viewer", Object: object}},
	}); err != nil {
		t.Fatalf("授 viewer: %v", err)
	}

	// allow（门槛通过）+ deny（等级不足）同时存在。
	if _, err := sc.PolicyRepo.Create(context.Background(), repo.Policy{
		Name: "trade 门槛", ResourceType: "kafka_topic", Effect: repo.EffectAllow,
		SubjectAttr: "bizLine", Op: repo.OpEq, Value: "trade", Enabled: true,
	}); err != nil {
		t.Fatalf("create allow policy: %v", err)
	}
	if _, err := sc.PolicyRepo.Create(context.Background(), repo.Policy{
		Name: "等级限制", ResourceType: "kafka_topic", Effect: repo.EffectDeny,
		SubjectAttr: "securityLevel", Op: repo.OpLt, ResourceAttr: "securityLevel", Enabled: true,
	}); err != nil {
		t.Fatalf("create deny policy: %v", err)
	}

	resp3, err := NewListObjectsLogic(ctxAs("u1", nil), sc).ListObjects(&types.ListObjectsReq{Type: "kafka_topic", Relation: "can_view"})
	if err != nil {
		t.Fatalf("u1 列表: %v", err)
	}
	if len(resp3.Objects) != 0 {
		t.Errorf("deny 应优先于 allow（u1 过门槛但等级不足）: %d", len(resp3.Objects))
	}
}

// TestCheckLogicABACOverlay 通用判定面叠加：/v1/authz/check 在 ReBAC 判定
// 通过后过 ABAC——后续业务域经 kit client 走通用 Check 即自动生效，
// 无需逐处接线。
func TestCheckLogicABACOverlay(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "via-check")
	object := "kafka_topic:" + topic.Id

	if _, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "viewer", Object: object}},
	}); err != nil {
		t.Fatalf("授 viewer: %v", err)
	}
	if err := sc.AttributeRepo.Set(context.Background(), repo.Attribute{Owner: "user:u1", AttrKey: "bizLine", AttrValue: "pay"}); err != nil {
		t.Fatalf("set subject attr: %v", err)
	}

	check := func(sub string) bool {
		t.Helper()
		resp, err := NewCheckLogic(ctxAs(sub, nil), sc).Check(&types.CheckReq{Object: object, Relation: "can_view"})
		if err != nil {
			t.Fatalf("check(%s): %v", sub, err)
		}
		return resp.Allowed
	}

	// 无策略：viewer 经 ReBAC 放行。
	if !check("u1") {
		t.Fatal("无策略时 viewer 应 allowed=true")
	}

	// allow 门槛（仅 trade）→ u1(pay) 被通用判定面挡下。
	if _, err := sc.PolicyRepo.Create(context.Background(), repo.Policy{
		Name: "仅 trade", ResourceType: "kafka_topic", Effect: repo.EffectAllow,
		SubjectAttr: "bizLine", Op: repo.OpEq, Value: "trade", Enabled: true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if check("u1") {
		t.Fatal("allow 门槛应经通用判定面拦截 u1(pay)")
	}
}

// TestPolicyLogicEffectValidation 策略写接口的 effect 白名单：
// allow 合法、未知值 400。
func TestPolicyLogicEffectValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)

	created, err := NewCreatePolicyLogic(adminCtx(), sc).CreatePolicy(&types.CreatePolicyReq{
		Name: "trade 门槛", ResourceType: "kafka_topic", Effect: "allow",
		SubjectAttr: "bizLine", Op: "eq", Value: "trade", Enabled: false,
	})
	if err != nil {
		t.Fatalf("创建 allow 策略应合法: %v", err)
	}
	if created.Policy.Effect != repo.EffectAllow {
		t.Errorf("effect 应归一为 allow: %q", created.Policy.Effect)
	}

	_, err = NewCreatePolicyLogic(adminCtx(), sc).CreatePolicy(&types.CreatePolicyReq{
		Name: "非法效果", Effect: "permit", SubjectAttr: "bizLine", Op: "eq", Value: "trade",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 400 || e.Code != "AUTHZ_POLICY_INVALID" {
		t.Errorf("非法 effect 应 400 AUTHZ_POLICY_INVALID: %v", err)
	}
}
