// memory_instance_test.go 是 InstanceRepo 内存实现的单元测试：覆盖 CRUD、
// 重名唯一性，以及 Page 可见性谓词的关键语义（全量 / ID 集合下推 / 空集合
// = 空页而非全量——与 MySQL 实现的 WHERE 1 = 0 分支对齐）。
package repo

import (
	"context"
	"errors"
	"testing"
)

// seedInstances 预置 3 条实例（自增 ID 1..3 对应 cache-a/cache-b/cache-c），
// 返回仓库实例供各用例复用。
func seedInstances(t *testing.T) InstanceRepo {
	t.Helper()
	r := NewMemoryInstanceRepo()
	for _, n := range []string{"cache-a", "cache-b", "cache-c"} {
		if _, err := r.Create(context.Background(), Instance{Name: n, Addr: "127.0.0.1:6379", CreatedBy: "admin"}); err != nil {
			t.Fatalf("预置实例 %s 失败: %v", n, err)
		}
	}
	return r
}

// TestCreateAndGet 验证自增 ID、重名 Create 报 ErrDuplicate、Get/GetByName
// 未命中返回 ErrNotFound。
func TestCreateAndGet(t *testing.T) {
	r := seedInstances(t)
	ctx := context.Background()

	got, err := r.GetByName(ctx, "cache-b")
	if err != nil || got.ID != 2 {
		t.Fatalf("GetByName cache-b = (%+v, %v)，期望 ID=2 无错", got, err)
	}
	if _, err := r.Create(ctx, Instance{Name: "cache-b"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("重名 Create 期望 ErrDuplicate，得到 %v", err)
	}
	if _, err := r.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get 未命中期望 ErrNotFound，得到 %v", err)
	}
}

// TestPageVisibility 验证 Page 三种谓词语义与分页切片：
// All 全量；IDs 集合升序过滤；非 All 且 IDs 为空 = 空页（普通用户无任何
// 关系时列表为空，而非全量泄露）；OFFSET 切页正确。
func TestPageVisibility(t *testing.T) {
	r := seedInstances(t)
	ctx := context.Background()

	if items, total, err := r.Page(ctx, InstancePageFilter{All: true}); err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("All=true 期望 3 条 total=3，得到 (%d 条, total=%d, %v)", len(items), total, err)
	}

	items, total, err := r.Page(ctx, InstancePageFilter{IDs: []int64{3, 1}})
	if err != nil || total != 2 || len(items) != 2 || items[0].ID != 1 || items[1].ID != 3 {
		t.Fatalf("IDs=[3,1] 期望升序 [1,3] total=2，得到 (%+v, total=%d, %v)", items, total, err)
	}

	items, total, err = r.Page(ctx, InstancePageFilter{})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("空 IDs 期望空页 total=0，得到 (%+v, total=%d, %v)", items, total, err)
	}

	items, total, err = r.Page(ctx, InstancePageFilter{All: true, Page: 2, PageSize: 2})
	if err != nil || total != 3 || len(items) != 1 || items[0].ID != 3 {
		t.Fatalf("Page=2 PageSize=2 期望仅 [3] total=3，得到 (%+v, total=%d, %v)", items, total, err)
	}
}

// TestUpdatePreservesImmutable 验证 Update 保留 CreatedBy（业务表 created_by
// 不随更新漂移，与 MySQL UPDATE 不碰 created_* 列同语义）、落 UpdateBy、
// 改成已有名报 ErrDuplicate。
func TestUpdatePreservesImmutable(t *testing.T) {
	r := seedInstances(t)
	ctx := context.Background()

	prev, err := r.GetByName(ctx, "cache-a")
	if err != nil {
		t.Fatalf("取 cache-a 失败: %v", err)
	}
	// 改名撞已有 cache-b → ErrDuplicate。
	dup := prev
	dup.Name = "cache-b"
	if _, err := r.Update(ctx, dup); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("改名撞 cache-b 期望 ErrDuplicate，得到 %v", err)
	}

	prev.Name = "cache-a2"
	prev.UpdateBy = "zhangsan"
	prev.CreatedBy = "hacker"
	upd, err := r.Update(ctx, prev)
	if err != nil {
		t.Fatalf("合法 Update 失败: %v", err)
	}
	if upd.CreatedBy != "admin" {
		t.Fatalf("CreatedBy 被覆盖，期望保留 admin，得到 %s", upd.CreatedBy)
	}
	if upd.UpdateBy != "zhangsan" {
		t.Fatalf("UpdateBy 未落，期望 zhangsan，得到 %s", upd.UpdateBy)
	}
}

// TestDelete 验证删除后 Get 未命中、重复 Delete 报 ErrNotFound
// （logic 层据此做"资源不存在幂等返成功"分支）。
func TestDelete(t *testing.T) {
	r := seedInstances(t)
	ctx := context.Background()

	if err := r.Delete(ctx, 2); err != nil {
		t.Fatalf("Delete 2 失败: %v", err)
	}
	if _, err := r.Get(ctx, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后 Get 期望 ErrNotFound，得到 %v", err)
	}
	if err := r.Delete(ctx, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("重复 Delete 期望 ErrNotFound，得到 %v", err)
	}
}
