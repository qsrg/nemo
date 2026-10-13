// grantpolicy_test.go 覆盖通用资源授权名单的类型级策略（菜单派生驱动）：
// 资源类型声明 ManageCode 后，名单的查看/写/撤销校验零改码生效——
// 管理码持有者全权、owner 可管理（**不含 editor**，编辑属性不放大为名单
// 管理权）、can_view 可查看、无关者 403。
package logic

import (
	"testing"

	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestGenericGrantPolicy 通用名单策略端到端：派生 report 类型（can_view +
// can_edit + 名单管理码），验证写/查/撤销三类接口的权限矩阵。
func TestGenericGrantPolicy(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Config.Engine.ModelFile = "../../model.fga" // sync 走合并基础模型，路径同 resourcetypelogic_test

	// 派生 report：菜单隐含 can_view + 按钮 can_edit，管理码预置存量
	// （派生 sync 保留存量值，见 resourcetypelogic_test 保留用例）。
	registerTypeViaMenus(t, sc, "report", []string{"can_view", "can_edit"}, "report-tools:manage")

	// u9 持名单管理码 → 对该类型任意资源全权。
	grantPerm(t, sc, "u9", "report-tools:manage")
	object := "report:r1"

	// 1) 管理码持有者写 owner 元组（资源创建归属由管理码承担）。
	if _, err := NewWriteRelationsLogic(ctxAs("u9", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:alice", Relation: "owner", Object: object}},
	}); err != nil {
		t.Fatalf("管理码写 owner: %v", err)
	}

	// 2) 无关用户写 viewer 元组 → 403（无管理码、无关系）。
	_, err := NewWriteRelationsLogic(ctxAs("mallory", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:mallory", Relation: "viewer", Object: object}},
	})
	if e := kiterrors.From(err); e == nil || e.Status != 403 {
		t.Fatalf("无关用户写名单应 403: %v", err)
	}

	// 3) 管理码持有者查看名单 OK；can_view 持有者（viewer）也可查看。
	listGrants := func(sub string) error {
		_, lerr := NewListResourceGrantsLogic(ctxAs(sub, nil), sc).ListResourceGrants(&types.ListResourceGrantsReq{Object: object})
		return lerr
	}
	if err := listGrants("u9"); err != nil {
		t.Fatalf("管理码查看名单: %v", err)
	}
	if _, err := NewWriteRelationsLogic(ctxAs("u9", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:bob", Relation: "viewer", Object: object}},
	}); err != nil {
		t.Fatalf("管理码写 viewer: %v", err)
	}
	if err := listGrants("bob"); err != nil {
		t.Fatalf("viewer(can_view) 查看名单: %v", err)
	}
	if err := listGrants("mallory"); err == nil {
		t.Fatal("无关用户查看名单应被拒")
	}

	// 4) owner（alice）写 editor 名单 + 撤销——资源负责人路径。
	if _, err := NewWriteRelationsLogic(ctxAs("alice", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:carol", Relation: "editor", Object: object}},
	}); err != nil {
		t.Fatalf("owner 写 editor: %v", err)
	}
	// editor 持有者不得管理名单（编辑属性不放大为名单管理权，防自授 owner 提权）。
	if _, err := NewWriteRelationsLogic(ctxAs("carol", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:carol", Relation: "owner", Object: object}},
	}); err == nil {
		t.Fatal("editor 不应能写 owner 名单（提权防护）")
	}
	if err := NewDeleteResourceGrantLogic(ctxAs("alice", nil), sc).DeleteResourceGrant(&types.DeleteResourceGrantReq{
		Object: object, User: "user:carol", Relation: "editor",
	}); err != nil {
		t.Fatalf("owner 撤销名单: %v", err)
	}
	// carol 被撤销后无任何关系 → 无权管理名单（关系即权限，撤即生效）。
	if err := NewDeleteResourceGrantLogic(ctxAs("carol", nil), sc).DeleteResourceGrant(&types.DeleteResourceGrantReq{
		Object: object, User: "user:bob", Relation: "viewer",
	}); err == nil {
		t.Fatal("被撤销者不应再能管理名单")
	}

	// 5) 未注册类型（order 之外走 writableObject 已拒；order 保留原语义）：
	// order 的写不校验资源管理权（业务服务批量写路径）。
	if _, err := NewWriteRelationsLogic(ctxAs("anyone", nil), sc).WriteRelations(&types.WriteRelationsReq{
		Tuples: []types.TupleReq{{User: "user:u1", Relation: "viewer", Object: "order:o1"}},
	}); err != nil {
		t.Fatalf("order 写保留原语义（不校验资源管理权）: %v", err)
	}
}
