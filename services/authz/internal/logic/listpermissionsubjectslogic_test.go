// listpermissionsubjectslogic_test.go 覆盖反向查询（管理面审计用）：
//   - 反查：user 直接授予 + org#member 主体两形态均返回，归一化为
//     {kind, id}，user 在前 org 在后、同类型按 id 升序；
//   - 通配码可查（*:read 对象 permission:*.read）；
//   - 参数校验：空/非法码 → 400；
//   - 无授权主体 → 200 空数组；引擎故障 → 503 fail-closed。
package logic

import (
	"context"
	"reflect"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestListPermissionSubjects 反查直接授权主体：user 与 org#member 两形态。
func TestListPermissionSubjects(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()
	bg := context.Background()

	// 预置：u1 直接授予 redis:read；u2 直接授予 redis:manage；
	// root 组织授予 authz-grants:manage（org#member 主体）+ 通配 *:read 给 u3。
	if err := sc.Engine.WriteTuples(bg, []engine.Tuple{
		{User: "user:u1", Relation: "can", Object: "permission:redis.read"},
		{User: "user:u2", Relation: "can", Object: "permission:redis.manage"},
		{User: "user:u3", Relation: "can", Object: "permission:*.read"},
	}); err != nil {
		t.Fatalf("预置 user 元组: %v", err)
	}
	rootID := createOrgViaLogic(t, sc, "总部", "")
	if err := sc.Engine.WriteTuples(bg, []engine.Tuple{
		{User: "org:" + rootID + "#member", Relation: "can", Object: "permission:authz-grants.manage"},
	}); err != nil {
		t.Fatalf("预置 org 元组: %v", err)
	}

	// redis:read 仅 u1。
	rr, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: "redis:read"})
	if err != nil {
		t.Fatalf("反查 redis:read: %v", err)
	}
	if want := []types.PermissionSubject{{Kind: "user", Id: "u1"}}; !reflect.DeepEqual(rr.Subjects, want) {
		t.Errorf("redis:read subjects = %v, 期望 %v", rr.Subjects, want)
	}

	// authz-grants:manage：root 组织主体（org 在 user 之后）。
	og, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: "authz-grants:manage"})
	if err != nil {
		t.Fatalf("反查 authz-grants:manage: %v", err)
	}
	if want := []types.PermissionSubject{{Kind: "org", Id: rootID}}; !reflect.DeepEqual(og.Subjects, want) {
		t.Errorf("authz-grants:manage subjects = %v, 期望 %v", og.Subjects, want)
	}

	// 通配码 *:read → u3（对象 permission:*.read）。
	wg, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: "*:read"})
	if err != nil {
		t.Fatalf("反查 *:read: %v", err)
	}
	if want := []types.PermissionSubject{{Kind: "user", Id: "u3"}}; !reflect.DeepEqual(wg.Subjects, want) {
		t.Errorf("*:read subjects = %v, 期望 %v", wg.Subjects, want)
	}

	// 多主体混合：redis:manage 再补一个 org 授权，验证排序（user 前 org 后）。
	if err := sc.Engine.WriteTuples(bg, []engine.Tuple{
		{User: "org:" + rootID + "#member", Relation: "can", Object: "permission:redis.manage"},
	}); err != nil {
		t.Fatalf("预置 redis.manage org 元组: %v", err)
	}
	mr, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: "redis:manage"})
	if err != nil {
		t.Fatalf("反查 redis:manage(混合): %v", err)
	}
	if want := []types.PermissionSubject{
		{Kind: "user", Id: "u2"},
		{Kind: "org", Id: rootID},
	}; !reflect.DeepEqual(mr.Subjects, want) {
		t.Errorf("redis:manage subjects = %v, 期望 %v", mr.Subjects, want)
	}

	// 无授权主体 → 200 空数组（非 null）。
	er, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: "no-such:read"})
	if err != nil || er.Subjects == nil || len(er.Subjects) != 0 {
		t.Errorf("无主体应空数组: resp=%v err=%v", er, err)
	}

	// 参数校验：空 / 非法码 → 400。
	for _, c := range []string{"", "redis", "Redis:read", "re dis:read"} {
		if _, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: c}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
			t.Errorf("非法码 %q 应 400, 实际 %v", c, err)
		}
	}

	// 引擎故障 → 503 fail-closed。
	sc.Engine.Stop()
	if _, err := NewListPermissionSubjectsLogic(ctx, sc).ListPermissionSubjects(&types.ListPermissionSubjectsReq{Permission: "redis:read"}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 503 {
		t.Errorf("引擎故障应 503, 实际 %v", err)
	}
}
