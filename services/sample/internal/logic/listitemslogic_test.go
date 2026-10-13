package logic

import (
	"context"
	"testing"

	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"
)

// TestListItemsEmpty 验证空列表语义：Items 为空切片（JSON 序列化为 [] 而非 null），Total 为 0。
func TestListItemsEmpty(t *testing.T) {
	l := NewListItemsLogic(context.Background(), svc.NewTestServiceContext())
	resp, err := l.ListItems()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Items == nil || len(resp.Items) != 0 || resp.Total != 0 {
		t.Fatalf("want empty list, got %+v", resp)
	}
}

// TestListItemsMultiple 验证多记录列表语义：Total 与记录数一致，字段转换正确。
func TestListItemsMultiple(t *testing.T) {
	ctx := svc.NewTestServiceContext()
	cl := NewCreateItemLogic(context.Background(), ctx)
	for _, name := range []string{"a", "b"} {
		if _, err := cl.CreateItem(&types.CreateItemReq{Name: name}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	l := NewListItemsLogic(context.Background(), ctx)
	resp, err := l.ListItems()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("want 2 items, got total=%d len=%d", resp.Total, len(resp.Items))
	}
	for _, it := range resp.Items {
		if it.Id == "" || it.Name == "" {
			t.Fatalf("want converted item, got %+v", it)
		}
	}
}
