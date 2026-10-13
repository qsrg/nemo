package repo

import (
	"context"
	"errors"
	"os"
	"testing"
)

// testItemRepo 用同一组断言约束全部实现（内存必跑；MySQL 走 TestMySQLRepo 环境开关）。
func testItemRepo(t *testing.T, r ItemRepo) {
	ctx := context.Background()
	created, err := r.Create(ctx, Item{Name: "a", Desc: "d1"})
	if err != nil || created.ID == 0 {
		t.Fatalf("create: %v", err)
	}
	got, err := r.Get(ctx, created.ID)
	if err != nil || got.Name != "a" {
		t.Fatalf("get: %v", err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list: %v", err)
	}
	updated, err := r.Update(ctx, Item{ID: created.ID, Name: "b"})
	if err != nil || updated.Name != "b" {
		t.Fatalf("update: %v", err)
	}
	// 同值更新（命中但值未变）必须成功，不得误判为 ErrNotFound：锁住
	// "RowsAffected=匹配行数"的驱动语义（见 NewMySQL 的 ClientFoundRows 注释），
	// 防止同一秒内重复同值 PUT 被当作未命中（DATETIME 默认秒精度下改变行数为 0）。
	if _, err := r.Update(ctx, Item{ID: created.ID, Name: "b"}); err != nil {
		t.Fatalf("same-value update on existing row: %v", err)
	}
	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

// testItemRepoNotFound 断言未命中语义在全部实现上一致：Get/Update/Delete 对
// 不存在的 id 均返回 ErrNotFound，logic 层据此统一转 404。
func testItemRepoNotFound(t *testing.T, r ItemRepo) {
	ctx := context.Background()
	if _, err := r.Get(ctx, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: want ErrNotFound, got %v", err)
	}
	if _, err := r.Update(ctx, Item{ID: 0, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}
	if err := r.Delete(ctx, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

// TestMemoryRepo 验证内存实现满足共享断言组（正常流程 + 未命中语义）。
func TestMemoryRepo(t *testing.T) {
	r := NewMemory()
	testItemRepo(t, r)
}

// TestMemoryRepoNotFound 单独验证内存实现的未命中语义。
func TestMemoryRepoNotFound(t *testing.T) {
	testItemRepoNotFound(t, NewMemory())
}

// TestNewMySQLInvalidDSN 验证 DSN 非法或连不上时 NewMySQL 返回 error
// （Ping 校验），不依赖真实 MySQL 实例。
func TestNewMySQLInvalidDSN(t *testing.T) {
	if _, err := NewMySQL("::::not-a-dsn"); err == nil {
		t.Fatal("malformed dsn: want error, got nil")
	}
	// 本机回环保留端口，连接必然被拒绝且快速失败，不产生跨网络请求。
	if _, err := NewMySQL("u:p@tcp(127.0.0.1:1)/nemo?charset=utf8mb4"); err == nil {
		t.Fatal("unreachable dsn: want error, got nil")
	}
}

// TestMySQLRepo 真库集成测试：仅在本机设置了 MYSQL_DSN 时执行，
// 否则跳过，保证无 MySQL 实例的机器上 `go test` 全绿。
func TestMySQLRepo(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN 未设置，跳过 MySQL 集成测试")
	}
	r, err := NewMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// 清场：删除历史残留记录，保证断言组（List 恰 1 条）对可重复执行的
	// 集成测试成立；仅通过 repo 接口操作，不依赖实现细节。
	ctx := context.Background()
	if stale, err := r.List(ctx); err != nil {
		t.Fatalf("list for cleanup: %v", err)
	} else {
		for _, s := range stale {
			if err := r.Delete(ctx, s.ID); err != nil {
				t.Fatalf("cleanup delete %d: %v", s.ID, err)
			}
		}
	}
	testItemRepo(t, r)
	testItemRepoNotFound(t, r)
}
