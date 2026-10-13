package logic

import (
	"context"
	"errors"
	"testing"

	kiterrors "nemo/kit/errors"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"
)

// TestUpdateItem 验证更新成功语义：全量覆盖 Name/Desc 并返回转换后的完整记录。
func TestUpdateItem(t *testing.T) {
	ctx := svc.NewTestServiceContext()
	cl := NewCreateItemLogic(context.Background(), ctx)
	created, err := cl.CreateItem(&types.CreateItemReq{Name: "a", Desc: "d1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	l := NewUpdateItemLogic(context.Background(), ctx)
	item, err := l.UpdateItem(&types.UpdateItemReq{Id: created.Id, Name: "b", Desc: "d2"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if item.Id != created.Id || item.Name != "b" || item.Desc != "d2" {
		t.Fatalf("want updated item, got %+v", item)
	}
}

// TestUpdateItemNameRequired 验证 name 空校验：校验先于存在性判断（400 优先于 404）。
func TestUpdateItemNameRequired(t *testing.T) {
	l := NewUpdateItemLogic(context.Background(), svc.NewTestServiceContext())
	_, err := l.UpdateItem(&types.UpdateItemReq{Id: "no-such-id", Name: ""})
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NAME_REQUIRED" {
		t.Fatalf("want SAMPLE_NAME_REQUIRED, got %v", err)
	}
	if kitErr.Status != 400 {
		t.Fatalf("want status 400, got %d", kitErr.Status)
	}
}

// TestUpdateItemNotFound 验证更新未命中语义：404 SAMPLE_NOT_FOUND。
func TestUpdateItemNotFound(t *testing.T) {
	l := NewUpdateItemLogic(context.Background(), svc.NewTestServiceContext())
	_, err := l.UpdateItem(&types.UpdateItemReq{Id: "no-such-id", Name: "b"})
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NOT_FOUND" {
		t.Fatalf("want SAMPLE_NOT_FOUND, got %v", err)
	}
}
