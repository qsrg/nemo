// resourcetypelogic_test.go 覆盖资源类型派生同步域（菜单声明驱动，替代原
// 独立注册 logic）:
//   - 派生全流程:资源驱动菜单(隐含 can_view)+ 资源级按钮(relation) →
//     SyncResourceTypesFromMenus → 注册表落库、与基础模型合并重建、引擎新
//     模型版本落库,且新类型 Check 真实生效;
//   - 合并确定性:新增另一类型后,版本 DSL 同时含全部已派生类型(重建式合并);
//   - 幂等:同声明重复 sync 不重建模型;
//   - 保留存量管理码:派生更新不清空类型级名单策略;
//   - 菜单侧校验:页面级按钮声明资源类型/资源级按钮缺资源类型/relation 出
//     白名单 → 400;
//   - ListResourceTypes 从 DslBlock 反解 Actions 回显。
package logic

import (
	"strings"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestSyncResourceTypesFromMenusFlow 派生全流程:经菜单声明 report
// （菜单隐含 can_view + 按钮 can_share）后——
//   - 注册表落库,actions = [can_view, can_share]（白名单固定顺序）;
//   - 版本 DSL 同时含基础类型与派生类型;
//   - 新类型的 can_* 出口经 Check 真实生效（写 owner 元组后判定）。
func TestSyncResourceTypesFromMenusFlow(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Config.Engine.ModelFile = "../../model.fga"
	ctx := adminCtx()

	registerTypeViaMenus(t, sc, "report", []string{"can_share", "can_view"}, "") // 乱序声明,验证规范化

	// 注册表:一条,actions 升序 [can_view can_share]。
	rts, err := sc.ResourceTypeRepo.List(ctx)
	if err != nil || len(rts) != 1 {
		t.Fatalf("注册表 = %+v (err=%v), 期望 1 条", rts, err)
	}
	if got := rts[0].DslBlock; !strings.Contains(got, "type report") || !strings.Contains(got, "define can_share: sharer") {
		t.Errorf("report 块缺 can_share 定义:\n%s", got)
	}

	// 新模型版本落库且含 report 块。
	mv, err := sc.ModelVersionRepo.Latest(ctx)
	if err != nil {
		t.Fatalf("取最新模型版本: %v", err)
	}
	if !strings.Contains(mv.Dsl, "type report") || !strings.Contains(mv.Dsl, "define can_share: sharer") {
		t.Errorf("版本 DSL 缺 report 块:\n%s", mv.Dsl)
	}

	// 新类型判定真实生效:owner → sharer/can_share、member_view/can_view。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "user:admin", Relation: "owner", Object: "report:r1"},
	}); err != nil {
		t.Fatalf("写 owner 元组: %v", err)
	}
	for _, rel := range []string{"can_view", "can_share"} {
		ok, err := sc.Engine.Check(ctx, "user:admin", rel, "report:r1")
		if err != nil {
			t.Fatalf("Check(report can_* %s): %v", rel, err)
		}
		if !ok {
			t.Errorf("Check(user:admin,%s,report:r1) = false, 期望 true(owner 链)", rel)
		}
	}

	// 第二类型派生:重建式合并,版本 DSL 同时含 report 与 billing。
	registerTypeViaMenus(t, sc, "billing", []string{"can_view", "can_edit", "can_delete", "can_share"}, "")
	mv, err = sc.ModelVersionRepo.Latest(ctx)
	if err != nil {
		t.Fatalf("取最新模型版本: %v", err)
	}
	if !strings.Contains(mv.Dsl, "type report") || !strings.Contains(mv.Dsl, "type billing") {
		t.Errorf("第二类型派生后版本 DSL 应同时含两个类型:\n%s", mv.Dsl)
	}
	// 基础类型仍在(合并不丢基础模型)。
	for _, baseType := range []string{"type user", "type org", "type platform_role", "type order"} {
		if !strings.Contains(mv.Dsl, baseType) {
			t.Errorf("派生后版本 DSL 丢失基础类型 %s:\n%s", baseType, mv.Dsl)
		}
	}

	// 注册表两条。
	rts, err = sc.ResourceTypeRepo.List(ctx)
	if err != nil || len(rts) != 2 {
		t.Errorf("注册表 = %+v (err=%v), 期望 2 条", rts, err)
	}
}

// TestSyncResourceTypesIdempotent 幂等:同声明再跑一次 sync 无变化,不产生
// 新模型版本（版本行数不变）。
func TestSyncResourceTypesIdempotent(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Config.Engine.ModelFile = "../../model.fga"
	ctx := adminCtx()

	registerTypeViaMenus(t, sc, "report", []string{"can_view", "can_edit"}, "")
	before, err := sc.ModelVersionRepo.Latest(ctx)
	if err != nil {
		t.Fatalf("取版本: %v", err)
	}
	if err := SyncResourceTypesFromMenus(ctx, sc); err != nil {
		t.Fatalf("重复 sync: %v", err)
	}
	after, err := sc.ModelVersionRepo.Latest(ctx)
	if err != nil {
		t.Fatalf("取版本: %v", err)
	}
	if before.EnginemodelID != after.EnginemodelID {
		t.Errorf("无变化 sync 不应重建模型: before=%s after=%s", before.EnginemodelID, after.EnginemodelID)
	}
}

// TestSyncResourceTypesKeepManageCode 派生更新保留存量名单管理码:预置
// report(manageCode=report-tools:manage)后菜单声明变更 → sync 更新块但
// 管理码不被清空。
func TestSyncResourceTypesKeepManageCode(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Config.Engine.ModelFile = "../../model.fga"
	ctx := adminCtx()

	registerTypeViaMenus(t, sc, "report", []string{"can_view", "can_edit"}, "report-tools:manage")
	rt, err := sc.ResourceTypeRepo.Get(ctx, "report")
	if err != nil {
		t.Fatalf("取 report: %v", err)
	}
	if rt.ManageCode != "report-tools:manage" {
		t.Errorf("派生同步后管理码被清空: %q", rt.ManageCode)
	}
}

// TestMenuButtonResourceValidation 菜单侧校验（新模型下类型由按钮/菜单声明
// 派生,配置错误在菜单写路径拒绝）:
//   - 页面级按钮声明资源类型 → 400（全局按钮与资源无关）;
//   - 资源级按钮缺 resourceType → 400;
//   - relation 出引擎出口白名单 → 400。
func TestMenuButtonResourceValidation(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Config.Engine.ModelFile = "../../model.fga"
	ctx := adminCtx()

	// 先建父菜单。
	page, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type:  "menu",
		Title: "应用页",
		Path:  "/app",
	})
	if err != nil {
		t.Fatalf("建父菜单: %v", err)
	}

	// 页面级按钮声明资源类型 → 400（全局按钮与资源无关）。
	_, err = NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type:           "button",
		ParentId:       page.Id,
		Title:          "导出",
		PermissionCode: "app:export",
		ResourceType:   "kafka_topic",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("页面级按钮声明资源类型应 400, 实际 %v", err)
	}

	// 资源级按钮缺 resourceType → 400。
	_, err = NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type:           "button",
		ParentId:       page.Id,
		Title:          "冻结",
		PermissionCode: "app:freeze",
		Relation:       "can_edit",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("资源级按钮缺 resourceType 应 400, 实际 %v", err)
	}

	// relation 出白名单 → 400。
	_, err = NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type:           "button",
		ParentId:       page.Id,
		Title:          "起飞",
		PermissionCode: "app:fly",
		ResourceType:   "kafka_topic",
		Relation:       "can_fly",
	})
	if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Errorf("relation 出白名单应 400, 实际 %v", err)
	}
}

// TestListResourceTypes 列表回显:Actions 从 DslBlock 反解、升序输出。
func TestListResourceTypes(t *testing.T) {
	sc := newTestSvcCtx(t)
	sc.Config.Engine.ModelFile = "../../model.fga"
	ctx := adminCtx()

	registerTypeViaMenus(t, sc, "billing", []string{"can_edit", "can_view"}, "")
	registerTypeViaMenus(t, sc, "report", []string{"can_share", "can_view", "can_delete"}, "")

	resp, err := NewListResourceTypesLogic(ctx, sc).ListResourceTypes()
	if err != nil {
		t.Fatalf("ListResourceTypes: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("ListResourceTypes = %+v, 期望 2 条", resp)
	}
	byName := map[string]types.ResourceTypeResp{}
	for _, it := range resp.Items {
		byName[it.Name] = it
	}
	if got := byName["billing"].Actions; len(got) != 2 || got[0] != "can_view" || got[1] != "can_edit" {
		t.Errorf("billing actions = %v, 期望 [can_view can_edit]", got)
	}
	if got := byName["report"].Actions; len(got) != 3 || got[0] != "can_view" || got[1] != "can_delete" || got[2] != "can_share" {
		t.Errorf("report actions = %v, 期望 [can_view can_delete can_share]", got)
	}
}
