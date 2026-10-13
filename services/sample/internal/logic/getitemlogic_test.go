package logic

import (
	"context"
	"errors"
	"testing"

	kiterrors "nemo/kit/errors"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"
)

// TestGetItem 验证查询成功语义：repo 存储模型正确转换为 API 出参模型（types↔repo 转换在 logic）。
func TestGetItem(t *testing.T) {
	ctx := svc.NewTestServiceContext()
	cl := NewCreateItemLogic(context.Background(), ctx)
	created, err := cl.CreateItem(&types.CreateItemReq{Name: "a", Desc: "d1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	l := NewGetItemLogic(context.Background(), ctx)
	item, err := l.GetItem(&types.ItemIdReq{Id: created.Id})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item.Id != created.Id || item.Name != "a" || item.Desc != "d1" {
		t.Fatalf("want %+v, got %+v", created, item)
	}
}

// TestGetItemNotFound 验证未命中语义：404 SAMPLE_NOT_FOUND。
func TestGetItemNotFound(t *testing.T) {
	l := NewGetItemLogic(context.Background(), svc.NewTestServiceContext())
	_, err := l.GetItem(&types.ItemIdReq{Id: "missing"})
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NOT_FOUND" {
		t.Fatalf("want SAMPLE_NOT_FOUND, got %v", err)
	}
	if kitErr.Status != 404 {
		t.Fatalf("want status 404, got %d", kitErr.Status)
	}
}
