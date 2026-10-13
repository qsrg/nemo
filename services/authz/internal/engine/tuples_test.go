// tuples_test.go 覆盖 Task 5 为 engine 包补充的元组读取/删除能力：
// ReadTuples（按 object/relation 过滤 + 空过滤读全量 + 分页）与
// DeleteTuples（幂等删除：不存在的元组跳过、删除后判定语义翻转）。
package engine

import (
	"context"
	"testing"
)

// TestReadTuplesFilteredAndAll 覆盖 ReadTuples 的三种过滤形态：
// object+relation 双过滤、仅 object 过滤、全量（双空）读取。
func TestReadTuplesFilteredAndAll(t *testing.T) {
	ctx := context.Background()
	h := startTestEngine(t)
	if _, _, err := h.EnsureModel(ctx, loadModelDSL(t)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if err := h.WriteTuples(ctx, []Tuple{
		{User: "org:root", Relation: "parent", Object: "org:child"},
		{User: "user:u1", Relation: "member", Object: "org:child"},
		{User: "user:u2", Relation: "member", Object: "org:root"},
		{User: "user:u3", Relation: "assignee", Object: "platform_role:operator"},
	}); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}

	// 双过滤：org:child 的 parent 元组，只有一条。
	got, err := h.ReadTuples(ctx, "org:child", "parent")
	if err != nil {
		t.Fatalf("ReadTuples(object,relation): %v", err)
	}
	if len(got) != 1 || got[0] != (Tuple{User: "org:root", Relation: "parent", Object: "org:child"}) {
		t.Errorf("双过滤结果 = %v, 期望 [{org:root parent org:child}]", got)
	}

	// 仅 object 过滤：org:root 相关的两条 member 元组。
	got, err = h.ReadTuples(ctx, "org:root", "")
	if err != nil {
		t.Fatalf("ReadTuples(object,): %v", err)
	}
	if len(got) != 1 || got[0].User != "user:u2" {
		t.Errorf("object 过滤结果 = %v, 期望 user:u2 的 member 元组", got)
	}

	// 全量：四条全部可见。
	got, err = h.ReadTuples(ctx, "", "")
	if err != nil {
		t.Fatalf("ReadTuples 全量: %v", err)
	}
	if len(got) != 4 {
		t.Errorf("全量读取 = %d 条, 期望 4: %v", len(got), got)
	}

	// 无匹配：返回空切片非 nil、非错误。
	got, err = h.ReadTuples(ctx, "org:nobody", "")
	if err != nil {
		t.Fatalf("ReadTuples 无匹配: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("无匹配应返回空, 实际 %v", got)
	}
}

// TestReadTuplesPagination 元组数超过单页上限（100）时分页取全。
func TestReadTuplesPagination(t *testing.T) {
	ctx := context.Background()
	h := startTestEngine(t)
	if _, _, err := h.EnsureModel(ctx, loadModelDSL(t)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}

	// 写 105 条 member 元组（单页上限 100，必须翻页）。
	var batch []Tuple
	for i := 0; i < 105; i++ {
		batch = append(batch, Tuple{
			User:     "user:u" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Relation: "member",
			Object:   "org:root",
		})
	}
	if err := h.WriteTuples(ctx, batch); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}

	got, err := h.ReadTuples(ctx, "org:root", "member")
	if err != nil {
		t.Fatalf("ReadTuples 分页: %v", err)
	}
	if len(got) != 105 {
		t.Errorf("分页读取 = %d 条, 期望 105", len(got))
	}
}

// TestDeleteTuplesIdempotent 覆盖 DeleteTuples 幂等语义：
// 删除存在的元组生效（Check 语义翻转）、重复删除不报错、
// 删除从未存在的元组同样安全。
func TestDeleteTuplesIdempotent(t *testing.T) {
	ctx := context.Background()
	h := startTestEngine(t)
	if _, _, err := h.EnsureModel(ctx, loadModelDSL(t)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	member := Tuple{User: "user:u1", Relation: "member", Object: "org:root"}
	if err := h.WriteTuples(ctx, []Tuple{member}); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}

	ok, err := h.Check(ctx, "user:u1", "member", "org:root")
	if err != nil || !ok {
		t.Fatalf("删除前 member 判定应 true: ok=%v err=%v", ok, err)
	}

	// 第一次删除生效。
	if err := h.DeleteTuples(ctx, []Tuple{member}); err != nil {
		t.Fatalf("DeleteTuples: %v", err)
	}
	ok, _ = h.Check(ctx, "user:u1", "member", "org:root")
	if ok {
		t.Errorf("删除后 member 判定应 false")
	}

	// 重复删除与删除不存在元组均幂等不报错。
	if err := h.DeleteTuples(ctx, []Tuple{member}); err != nil {
		t.Errorf("重复删除应幂等: %v", err)
	}
	if err := h.DeleteTuples(ctx, []Tuple{{User: "user:nobody", Relation: "member", Object: "org:root"}}); err != nil {
		t.Errorf("删除不存在元组应幂等: %v", err)
	}

	// 读回验证：空。
	got, err := h.ReadTuples(ctx, "org:root", "member")
	if err != nil {
		t.Fatalf("ReadTuples: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("删除后读取应为空, 实际 %v", got)
	}
}

// TestReadTuplesBeforeEnsureModel 未绑定 Store/Model 时读元组应报错（防误用）。
func TestReadTuplesBeforeEnsureModel(t *testing.T) {
	h := startTestEngine(t)
	if _, err := h.ReadTuples(context.Background(), "", ""); err == nil {
		t.Errorf("未 EnsureModel 时 ReadTuples 应报错")
	}
	if err := h.DeleteTuples(context.Background(), []Tuple{{User: "user:u1", Relation: "member", Object: "org:root"}}); err == nil {
		t.Errorf("未 EnsureModel 时 DeleteTuples 应报错")
	}
}
