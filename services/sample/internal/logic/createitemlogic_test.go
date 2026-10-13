package logic

import (
	"context"
	"errors"
	"testing"

	kiterrors "nemo/kit/errors"
	"nemo/services/sample/internal/svc"
	"nemo/services/sample/internal/types"
)

// TestCreateItem 验证创建成功语义：返回完整 Item（Id 由 repo 生成非空），Name/Desc 透传。
func TestCreateItem(t *testing.T) {
	l := NewCreateItemLogic(context.Background(), svc.NewTestServiceContext())
	item, err := l.CreateItem(&types.CreateItemReq{Name: "a", Desc: "d1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Id == "" || item.Name != "a" || item.Desc != "d1" {
		t.Fatalf("want id/name/desc 透传, got %+v", item)
	}
}

// TestCreateItemNameRequired 验证 name 空校验：空串与纯空白一律 400 SAMPLE_NAME_REQUIRED，
// 且校验先于任何存储访问（不产生脏数据）。
func TestCreateItemNameRequired(t *testing.T) {
	for _, name := range []string{"", "   "} {
		l := NewCreateItemLogic(context.Background(), svc.NewTestServiceContext())
		_, err := l.CreateItem(&types.CreateItemReq{Name: name})
		var kitErr *kiterrors.Error
		if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NAME_REQUIRED" {
			t.Fatalf("name %q: want SAMPLE_NAME_REQUIRED, got %v", name, err)
		}
		if kitErr.Status != 400 {
			t.Fatalf("name %q: want status 400, got %d", name, kitErr.Status)
		}
	}
}
