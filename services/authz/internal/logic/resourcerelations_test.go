// resourcerelations_test.go 覆盖通用资源授权 relations API（判定面
// /v1/authz/relations 收敛后）：
//   - WriteRelations 对 topic 对象的管理权校验（owner/editor/manage 码），
//     防止任意登录用户自授名单越权；
//   - ListResourceGrants 查看校验（manage 码全量 / can_view）；
//   - DeleteResourceGrant 撤销校验（topicCanEdit）+ 幂等删除。
package logic

import (
	"context"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestWriteRelationsTopicManageCheck 通用写接口对 topic 对象的名单写要求
// 管理权：admin（预置 kafka-topics:manage）可写 viewer；无管理权的 u9 写
// 同一元组 403 AUTHZ_DENIED（防自授越权）。
func TestWriteRelationsTopicManageCheck(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "rel-topic")

	// admin 写 viewer 成功（幂等）。
	wl := NewWriteRelationsLogic(adminCtx(), sc)
	_, err := wl.WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "viewer", Object: "kafka_topic:" + topic.Id}},
	})
	if err != nil {
		t.Fatalf("admin 写 viewer 应成功: %v", err)
	}

	// u9（无 manage 码、非 owner/editor）写同关系 → 403。
	_, err = NewWriteRelationsLogic(ctxAs("u9", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u2", Relation: "viewer", Object: "kafka_topic:" + topic.Id}},
	})
	if err == nil {
		t.Fatal("u9 写 topic 名单应被拒")
	}
	if e := kiterrors.From(err); e == nil || e.Status != 403 || e.Code != "AUTHZ_DENIED" {
		t.Fatalf("应 403 AUTHZ_DENIED: %v", err)
	}
}

// TestListResourceGrantsViewCheck 通用列表接口查看校验：admin（manage 码）
// 可列全部元组；未获 view 的 u9 403；授予 viewer 后可列。
func TestListResourceGrantsViewCheck(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "list-topic")

	// 预置 editor 元组（admin 管理权下写）。
	if _, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:zhangsan", Relation: "editor", Object: "kafka_topic:" + topic.Id}},
	}); err != nil {
		t.Fatalf("预置 editor: %v", err)
	}

	// admin 列表：manage 码全量可见。
	list, err := NewListResourceGrantsLogic(adminCtx(), sc).ListResourceGrants(&types.ListResourceGrantsReq{
		Object: "kafka_topic:" + topic.Id,
	})
	if err != nil {
		t.Fatalf("admin 列表: %v", err)
	}
	if list.Total == 0 {
		t.Fatal("admin 列表不应为空")
	}
	found := false
	for _, it := range list.Items {
		if it.User == "user:zhangsan" && it.Relation == "editor" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应含 zhangsan editor 元组: %+v", list.Items)
	}

	// u9 无 view → 403。
	if _, err := NewListResourceGrantsLogic(ctxAs("u9", nil), sc).ListResourceGrants(&types.ListResourceGrantsReq{
		Object: "kafka_topic:" + topic.Id,
	}); err == nil {
		t.Fatal("u9 无查看权应被拒")
	} else if e := kiterrors.From(err); e == nil || e.Status != 403 || e.Code != "AUTHZ_DENIED" {
		t.Fatalf("应 403 AUTHZ_DENIED: %v", err)
	}

	// 授予 u9 viewer 后可列。
	if _, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u9", Relation: "viewer", Object: "kafka_topic:" + topic.Id}},
	}); err != nil {
		t.Fatalf("授予 u9 viewer: %v", err)
	}
	if _, err := NewListResourceGrantsLogic(ctxAs("u9", nil), sc).ListResourceGrants(&types.ListResourceGrantsReq{
		Object: "kafka_topic:" + topic.Id,
	}); err != nil {
		t.Fatalf("u9 获 viewer 后应可列: %v", err)
	}
}

// TestDeleteResourceGrantManageAndIdempotent 通用撤销接口：admin 撤销成功、
// 重复撤销幂等（不存在同成功）；无管理权的 u9 撤销 403。
func TestDeleteResourceGrantManageAndIdempotent(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "del-topic")
	object := "kafka_topic:" + topic.Id

	if _, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:zhangsan", Relation: "viewer", Object: object}},
	}); err != nil {
		t.Fatalf("预置 viewer: %v", err)
	}

	dl := NewDeleteResourceGrantLogic(adminCtx(), sc)
	if err := dl.DeleteResourceGrant(&types.DeleteResourceGrantReq{
		Object: object, User: "user:zhangsan", Relation: "viewer",
	}); err != nil {
		t.Fatalf("admin 撤销应成功: %v", err)
	}
	// 幂等：重复撤销不报错。
	if err := dl.DeleteResourceGrant(&types.DeleteResourceGrantReq{
		Object: object, User: "user:zhangsan", Relation: "viewer",
	}); err != nil {
		t.Fatalf("重复撤销应幂等成功: %v", err)
	}

	// 元组确已删除。
	tuples, err := sc.Engine.ReadTuples(context.Background(), object, "viewer")
	if err != nil {
		t.Fatalf("读元组: %v", err)
	}
	if len(tuples) != 0 {
		t.Fatalf("撤销后 viewer 元组应为空: %+v", tuples)
	}

	// u9 无管理权撤销 → 403。
	if err := NewDeleteResourceGrantLogic(ctxAs("u9", nil), sc).DeleteResourceGrant(&types.DeleteResourceGrantReq{
		Object: object, User: "user:zhangsan", Relation: "viewer",
	}); err == nil {
		t.Fatal("u9 撤销应被拒")
	} else if e := kiterrors.From(err); e == nil || e.Status != 403 || e.Code != "AUTHZ_DENIED" {
		t.Fatalf("应 403 AUTHZ_DENIED: %v", err)
	}
}

// TestWriteRelationsTopicObjectWritable topic 类型可写白名单：白名单关系
// 之外的关系（如 can_view 出口关系）拒绝——写白名单语义与 order 一致。
func TestWriteRelationsTopicRelationWhitelist(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "whitelist-topic")

	_, err := NewWriteRelationsLogic(adminCtx(), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "can_view", Object: "kafka_topic:" + topic.Id}},
	})
	if err == nil {
		t.Fatal("can_view 不在写白名单应被拒")
	}
	if e := kiterrors.From(err); e == nil || e.Status != 403 || e.Code != "AUTHZ_RELATION_FORBIDDEN" {
		t.Fatalf("应 403 AUTHZ_RELATION_FORBIDDEN: %v", err)
	}
}

// compile 防 engine 未使用（测试内不直接引用）。
var _ = engine.Tuple{}

// TestWriteRelationsCreateBootstrap 创建引导放行分支（2026-10-03 空对象首写）：
// relation 为 owner/org 且对象无 owner 元组时，持注册表 create 码或 manage 码
// 的调用者放行（"创建者即 owner"由调用方服务守卫背书）——自举死锁的权威语义。
// 覆盖：放行（create 码/manage 码）、无码拒绝、非空对象拒绝（防元组预占）、
// org 关系同放行。
func TestWriteRelationsCreateBootstrap(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	// 注册表补 createCode（生产由 006 迁移/部署设置；memory 测试手工设）。
	ctx := context.Background()
	rt, err := sc.ResourceTypeRepo.Get(ctx, "kafka_topic")
	if err != nil {
		t.Fatalf("读注册表: %v", err)
	}
	rt.CreateCode = "kafka-topics:create"
	if err := sc.ResourceTypeRepo.Update(ctx, rt); err != nil {
		t.Fatalf("设 createCode: %v", err)
	}
	// alice 持 create 码；wangwu 无任何码。
	grantPerm(t, sc, "alice", "kafka-topics:create")

	// 1. 空对象首写 owner + create 码 → 放行（元组落库）。
	wl := NewWriteRelationsLogic(ctxAs("alice", nil), sc)
	if _, err := wl.WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u2", Relation: "owner", Object: "kafka_topic:90001"}},
	}); err != nil {
		t.Fatalf("空对象 + create 码应放行: %v", err)
	}

	// 2. 对象已有 owner（u2 认领）→ alice（create 码持有者但非 owner）再写 → 403
	//    （防元组预占：放行仅限空对象首写）。
	_, err = NewWriteRelationsLogic(ctxAs("alice", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:alice", Relation: "owner", Object: "kafka_topic:90001"}},
	})
	if e := kiterrors.From(err); e == nil || e.Status != 403 {
		t.Fatalf("非空对象再写应 403: %v", err)
	}

	// 3. 空对象 + 无任何码（wangwu）→ 403。
	if _, err := NewWriteRelationsLogic(ctxAs("wangwu", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:wangwu", Relation: "owner", Object: "kafka_topic:90002"}},
	}); err == nil {
		t.Fatal("无码写空对象应被拒")
	}

	// 4. org 关系同放行（kafka 创建的部门挂接与 owner 同批）。
	if _, err := NewWriteRelationsLogic(ctxAs("alice", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "org:tech", Relation: "org", Object: "kafka_topic:90003"}},
	}); err != nil {
		t.Fatalf("空对象 org 挂接 + create 码应放行: %v", err)
	}
}

// TestWriteRelationsCreateBootstrapManage 管理码通道：注册类型仅声明
// manageCode（无 createCode）时，管理码持有者写空对象首 owner 放行
// （如 zk_cluster 场景——创建入口仅管理码）。预置块多含一个 can_edit 使
// 派生块与之有差异——触发 sync 重建引擎模型（无变化时 EnsureModel 跳过，
// 模型将缺类型——生产注册路径 Create 必带 changed=true 无此问题）。
func TestWriteRelationsCreateBootstrapManage(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerTypeViaMenus(t, sc, "zk_cluster", []string{"can_view", "can_edit"}, "zookeeper:manage")
	grantPerm(t, sc, "zhangsan", "zookeeper:manage")

	if _, err := NewWriteRelationsLogic(ctxAs("zhangsan", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u3", Relation: "owner", Object: "zk_cluster:80001"}},
	}); err != nil {
		t.Fatalf("manage 码写空对象首 owner 应放行: %v", err)
	}
}

// TestManageCodeGrantsOnRegisteredType 收敛专用分支后的注册策略等价性：
// 注册类型（kafka_topic）声明 manageCode 后，管理码持有者（非 owner）
// 拥有名单管理全权（写 viewer/撤销），无码者 403——与"列表管理码"语义
// 等价，验证收敛未破坏管理码路径。
func TestManageCodeGrantsOnRegisteredType(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc)
	topic := createTopicViaLogic(t, sc, "mc-topic")
	object := "kafka_topic:" + topic.Id

	// u9 持管理码（非 owner）→ 可写 viewer、可撤销。
	grantPerm(t, sc, "u9", "kafka-topics:manage")
	if _, err := NewWriteRelationsLogic(ctxAs("u9", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:zhangsan", Relation: "viewer", Object: object}},
	}); err != nil {
		t.Fatalf("管理码持有者写名单应成功: %v", err)
	}
	if err := NewDeleteResourceGrantLogic(ctxAs("u9", nil), sc).DeleteResourceGrant(&types.DeleteResourceGrantReq{
		Object: object, User: "user:zhangsan", Relation: "viewer",
	}); err != nil {
		t.Fatalf("管理码持有者撤销应成功: %v", err)
	}

	// wangwu 无码（非 owner）→ 写名单 403。
	if _, err := NewWriteRelationsLogic(ctxAs("wangwu", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:zhangsan", Relation: "viewer", Object: object}},
	}); err == nil {
		t.Fatal("无管理权写名单应被拒")
	} else if e := kiterrors.From(err); e == nil || e.Status != 403 || e.Code != "AUTHZ_DENIED" {
		t.Fatalf("应 403 AUTHZ_DENIED: %v", err)
	}
}
