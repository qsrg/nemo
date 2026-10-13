package logic

import (
	"context"
	"errors"
	"testing"

	kiterrors "nemo/kit/errors"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"
)

// TestDeleteItem 验证删除成功语义：删除后不可再查得（404）。
func TestDeleteItem(t *testing.T) {
	ctx := svc.NewTestServiceContext()
	cl := NewCreateItemLogic(context.Background(), ctx)
	created, err := cl.CreateItem(&types.CreateItemReq{Name: "a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	dl := NewDeleteItemLogic(context.Background(), ctx)
	if err := dl.DeleteItem(&types.ItemIdReq{Id: created.Id}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	gl := NewGetItemLogic(context.Background(), ctx)
	_, err = gl.GetItem(&types.ItemIdReq{Id: created.Id})
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NOT_FOUND" {
		t.Fatalf("want SAMPLE_NOT_FOUND after delete, got %v", err)
	}
}

// TestDeleteItemNotFound 验证删除未命中语义：404 SAMPLE_NOT_FOUND。
func TestDeleteItemNotFound(t *testing.T) {
	l := NewDeleteItemLogic(context.Background(), svc.NewTestServiceContext())
	err := l.DeleteItem(&types.ItemIdReq{Id: "no-such-id"})
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NOT_FOUND" {
		t.Fatalf("want SAMPLE_NOT_FOUND, got %v", err)
	}
}
