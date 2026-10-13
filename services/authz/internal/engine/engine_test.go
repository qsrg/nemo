// engine_test.go 是 M0 引擎 PoC 冒烟测试：
// 启动内嵌引擎（memory）→ EnsureModel（model.fga）→ WriteTuples 构造四类场景 →
// 五组断言（租户隔离/层级继承/共享/owner 权限/ListObjects）全部经 Handle API 验证。
package engine

import (
	"context"
	"os"
	"slices"
	"testing"
)

// startTestEngine 启动测试引擎并注册 Cleanup，返回 Handle。
func startTestEngine(t *testing.T) *Handle {
	t.Helper()

	h, err := Start(context.Background(), Options{
		HTTPAddr:  "127.0.0.1:0", // 随机端口，避免并行冲突
		StoreName: "poc-test-store",
		Datastore: DatastoreMemory,
	})
	if err != nil {
		t.Fatalf("启动引擎: %v", err)
	}
	t.Cleanup(h.Stop)
	return h
}

// loadModelDSL 读取仓库内 model.fga 全文。
func loadModelDSL(t *testing.T) string {
	t.Helper()

	// 测试文件位于 internal/engine/，模型文件在模块根 services/authz/。
	data, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	return string(data)
}

// seedTuples 构造 brief 要求的四类场景元组。
// parent 元组方向（Task 8 修正）：(parent, parent, child)——父组织的 parent
// 关系指向子组织，与生产路径（createorg/updateorglogic）一致；原 Task 1
// 种子方向反了（child→root），其"层级继承"断言实际经 owner 路径空转通过，
// 本修正后 u1 对 o3（无 owner 路径）的断言才是纯成员链继承的真证据。
func seedTuples() []Tuple {
	return []Tuple{
		// 层级：org:root 是 org:child 的父组织；parent/child 互逆成对
		//（child 供判定面 member from child 聚合，模型 v5）。
		{User: "org:root", Relation: "parent", Object: "org:child"},
		{User: "org:child", Relation: "child", Object: "org:root"},
		// u1 是 org:child 的 member（子树末端成员，经 child 聚合亦为 root 成员）
		{User: "user:u1", Relation: "member", Object: "org:child"},
		// u1 是 order:o1 的 owner（editor/can_delete/can_share 均由此派生）
		{User: "user:u1", Relation: "owner", Object: "order:o1"},
		// u2 无任何关系（负样本，租户隔离）
		// u3 仅是 order:o2 的 viewer（共享场景：可见不可分享），
		// o2 挂在与根链路无关的独立组织 org:iso（u1 经成员链不可见）
		{User: "user:u3", Relation: "viewer", Object: "order:o2"},
		{User: "org:iso", Relation: "org", Object: "order:o2"},
		// o1 挂 org:child（owner 路径可见）；o3 挂 org:root 且无 owner，
		// u1（child 成员聚合为 root 成员）经成员链可见——子→父授权继承真证据
		{User: "org:child", Relation: "org", Object: "order:o1"},
		{User: "org:root", Relation: "org", Object: "order:o3"},
	}
}

// TestEnginePoC 覆盖 brief Step 5 的五组断言。
func TestEnginePoC(t *testing.T) {
	ctx := context.Background()
	h := startTestEngine(t)

	// EnsureModel：DSL 装配（含幂等二次调用验证）。
	storeID, modelID, err := h.EnsureModel(ctx, loadModelDSL(t))
	if err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if storeID == "" || modelID == "" {
		t.Fatalf("EnsureModel 返回空 ID: storeID=%q modelID=%q", storeID, modelID)
	}
	storeID2, _, err := h.EnsureModel(ctx, loadModelDSL(t))
	if err != nil {
		t.Fatalf("EnsureModel 二次调用: %v", err)
	}
	if storeID2 != storeID {
		t.Errorf("EnsureModel 幂等失败：二次调用 StoreID 变化 %s -> %s", storeID, storeID2)
	}

	// 写元组构造场景（含幂等二次写入验证）。
	tuples := seedTuples()
	if err := h.WriteTuples(ctx, tuples); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}
	if err := h.WriteTuples(ctx, tuples); err != nil {
		t.Errorf("WriteTuples 幂等失败（二次写入报错）: %v", err)
	}

	// ① 租户隔离：u2 无任何关系，对 o1/o2 的 can_view 均 false。
	for _, obj := range []string{"order:o1", "order:o2"} {
		allowed, err := h.Check(ctx, "user:u2", "can_view", obj)
		if err != nil {
			t.Fatalf("Check(u2, can_view, %s): %v", obj, err)
		}
		if allowed {
			t.Errorf("① 租户隔离失败：u2 对 %s can_view 应为 false", obj)
		}
	}

	// ② 层级继承（模型 v5 子→父聚合）：u1 是 org:child 的 member，聚合为
	// org:root 成员；o3 无 owner 路径且挂 root，可见性只能来自成员链（真继承证据）。
	for _, obj := range []string{"order:o1", "order:o3"} {
		allowed, err := h.Check(ctx, "user:u1", "can_view", obj)
		if err != nil {
			t.Fatalf("Check(u1, can_view, %s): %v", obj, err)
		}
		if !allowed {
			t.Errorf("② 层级继承失败：u1 对 %s can_view 应为 true（member from child 聚合链）", obj)
		}
	}
	// u1 与 org:iso 无任何链路，对 o2 不可见（组织级隔离）。
	allowed, err := h.Check(ctx, "user:u1", "can_view", "order:o2")
	if err != nil {
		t.Fatalf("Check(u1, can_view, order:o2): %v", err)
	}
	if allowed {
		t.Errorf("② 组织级隔离失败：u1 对 order:o2 can_view 应为 false")
	}

	// ③ 共享：u3 是 o2 的 viewer，可见但不可分享。
	allowed, err = h.Check(ctx, "user:u3", "can_view", "order:o2")
	if err != nil {
		t.Fatalf("Check(u3, can_view, order:o2): %v", err)
	}
	if !allowed {
		t.Errorf("③ 共享失败：u3 对 order:o2 can_view 应为 true")
	}
	allowed, err = h.Check(ctx, "user:u3", "can_share", "order:o2")
	if err != nil {
		t.Fatalf("Check(u3, can_share, order:o2): %v", err)
	}
	if allowed {
		t.Errorf("③ 共享失败：u3 对 order:o2 can_share 应为 false")
	}

	// ④ owner 权限：u1 owner(order:o1)，can_edit/can_delete/can_share 均为 true。
	for _, rel := range []string{"can_edit", "can_delete", "can_share"} {
		allowed, err := h.Check(ctx, "user:u1", rel, "order:o1")
		if err != nil {
			t.Fatalf("Check(u1, %s, order:o1): %v", rel, err)
		}
		if !allowed {
			t.Errorf("④ owner 权限失败：u1 对 order:o1 %s 应为 true", rel)
		}
	}

	// ⑤ ListObjects：u1 的可看订单含 o1（owner+成员链）与 o3（纯成员链），
	// 不含 u3 专属且组织隔离的 o2。
	objects, err := h.ListObjects(ctx, "user:u1", "can_view", "order")
	if err != nil {
		t.Fatalf("ListObjects(u1, can_view, order): %v", err)
	}
	for _, want := range []string{"order:o1", "order:o3"} {
		if !slices.Contains(objects, want) {
			t.Errorf("⑤ ListObjects 失败：u1 可见列表应含 %s，实际 %v", want, objects)
		}
	}
	if slices.Contains(objects, "order:o2") {
		t.Errorf("⑤ ListObjects 失败：u1 可见列表不应含 order:o2，实际 %v", objects)
	}
}

// TestHandlePing 验证就绪探针语义：启动后 Ping 成功，Stop 之后 Ping 报错。
func TestHandlePing(t *testing.T) {
	h := startTestEngine(t)

	// 启动状态下端口可达，Ping 应返回 nil。
	if err := h.Ping(); err != nil {
		t.Errorf("启动后 Ping 应成功: %v", err)
	}

	// Stop 之后端口关闭，Ping 应返回错误（readyz 依赖此语义返回 503）。
	h.Stop()
	if err := h.Ping(); err == nil {
		t.Errorf("Stop 后 Ping 应失败，实际成功")
	}
}

// TestHandleConcurrentEnsureAndCheck 终审 I1 并发保护：EnsureModel
// （RegisterResourceType 模型合并路径会反复调用）与 Check 并发执行，
// storeID/modelID 的读写经 RWMutex 保护——本测试须在 -race 下通过。
func TestHandleConcurrentEnsureAndCheck(t *testing.T) {
	ctx := context.Background()
	h := startTestEngine(t)

	dsl := loadModelDSL(t)
	storeID, _, err := h.EnsureModel(ctx, dsl)
	if err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if err := h.WriteTuples(ctx, []Tuple{{User: "user:u1", Relation: "owner", Object: "order:o1"}}); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}

	done := make(chan struct{})
	// 写者：反复 EnsureModel（幂等，模拟资源类型注册的模型重建）。
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			if _, _, err := h.EnsureModel(ctx, dsl); err != nil {
				t.Errorf("并发 EnsureModel: %v", err)
				return
			}
		}
	}()
	// 读者：并发 Check / StoreID / ModelID。
	for i := 0; i < 4; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				if _, err := h.Check(ctx, "user:u1", "can_delete", "order:o1"); err != nil {
					t.Errorf("并发 Check: %v", err)
					return
				}
				_ = h.StoreID()
				_ = h.ModelID()
			}
		}()
	}
	<-done
	if h.StoreID() != storeID || h.ModelID() == "" {
		t.Errorf("绑定 ID 应稳定: storeID=%q modelID=%q", h.StoreID(), h.ModelID())
	}
}

// TestHandleBatchCheck 批量判定集成验证（OpenFGA 服务端 BatchCheck）：
// 一次 RPC 判定多组 object/relation，结果与请求下标一一对应；
// item 级"明确无权"返回 false 不报错，空批返回空切片。
func TestHandleBatchCheck(t *testing.T) {
	ctx := context.Background()
	h := startTestEngine(t)

	if _, _, err := h.EnsureModel(ctx, loadModelDSL(t)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if err := h.WriteTuples(ctx, seedTuples()); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}

	// u1：owner of o1（can_view/can_delete 派生为真）、iso 组织隔离不可见 o2、
	// 经 child 成员链聚合可见 o3（与 TestEnginePoC 的继承断言同口径）。
	results, err := h.BatchCheck(ctx, "user:u1", []BatchCheckItem{
		{Object: "order:o1", Relation: "can_view"},
		{Object: "order:o1", Relation: "can_delete"},
		{Object: "order:o2", Relation: "can_view"},
		{Object: "order:o3", Relation: "can_view"},
	})
	if err != nil {
		t.Fatalf("BatchCheck: %v", err)
	}
	want := []bool{true, true, false, true}
	if len(results) != len(want) {
		t.Fatalf("结果数 %d != 期望 %d: %v", len(results), len(want), results)
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("results[%d] = %v, 期望 %v", i, results[i], want[i])
		}
	}

	// 空批次：返回空切片且不报错。
	empty, err := h.BatchCheck(ctx, "user:u1", nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("空批次应返回空切片且无错误: results=%v err=%v", empty, err)
	}

	// u2 无任何关系：明确无权 = false，整体不报错（与 Check 语义一致）。
	denied, err := h.BatchCheck(ctx, "user:u2", []BatchCheckItem{
		{Object: "order:o1", Relation: "can_view"},
	})
	if err != nil {
		t.Fatalf("BatchCheck(u2): %v", err)
	}
	if len(denied) != 1 || denied[0] {
		t.Errorf("u2 应明确无权: %v", denied)
	}
}
