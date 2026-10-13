// menulogic_test.go 覆盖菜单注册表（菜单模型简化，docs/menu-simplify-design.md）：
// 管理面 CRUD（权限码唯一 409/未命中 404/层级深度/按钮行约束）、me/menus
// 过滤语义（空码全员可见、精确码、*:动作 通配、enabled 位）、菜单停用改码
// 删除时的孤儿授权清理（含页面级按钮级联）。全部走真 memory 引擎装配
// （newTestSvcCtx）与内存 repo。
//
// IaC 改造：代码内置 seed 已移除，菜单事实源为管理面 API。测试用
// seedTestMenus 辅助直接预置菜单行（行身份 = 自增 id，父子 = parent_id），
// 不依赖任何 seed 逻辑。
package logic

import (
	"context"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"

	authzkit "nemo/kit/authz"
	kiterrors "nemo/kit/errors"
	"nemo/services/authz/internal/svc"
)

// ctxAs 构造带指定身份的 ctx（roles 决定权限归并与 AdminRole 语义）。
func ctxAs(sub string, roles []string) context.Context {
	return authzkit.WithIdentity(context.Background(), sub, roles)
}

// grantPerm 直接授予用户权限点 can 元组（去角色化改造：权限 = 人 × 权限点）。
func grantPerm(t *testing.T, sc *svc.ServiceContext, userID, code string) {
	t.Helper()
	if err := sc.Engine.WriteTuples(context.Background(), []engine.Tuple{
		{User: "user:" + userID, Relation: "can", Object: permObject(code)},
	}); err != nil {
		t.Fatalf("授予权限 %s → %s: %v", userID, code, err)
	}
}

// tupleCount 读取某权限码的 can 授予元组条数（孤儿清理断言用）。
func tupleCount(t *testing.T, sc *svc.ServiceContext, code string) int {
	t.Helper()
	tuples, err := sc.Engine.ReadTuples(context.Background(), permObject(code), "can")
	if err != nil {
		t.Fatalf("读 %s 授予元组: %v", code, err)
	}
	return len(tuples)
}

// seedTestMenus 预置测试所需菜单行（模拟声明下发后的注册表状态）：行身份 =
// 自增 id，父子经 parent_id 串树。返回 path → 行 id 映射（用例按 path 断言）。
// 含一行空码菜单（/home，方案 B：空 = 全员可见）。
func seedTestMenus(t *testing.T, sc *svc.ServiceContext) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	ids := make(map[string]int64, 10)
	put := func(m repo.Menu) {
		t.Helper()
		created, err := sc.MenuRepo.Create(ctx, m)
		if err != nil {
			t.Fatalf("预置菜单 %s: %v", m.Title, err)
		}
		ids[created.Path] = created.ID
	}
	put(repo.Menu{Type: "menu", Title: "控制台", Path: "/home", Sort: 1, Enabled: true})
	put(repo.Menu{Type: "menu", Title: "实例管理", Path: "/redis", Sort: 10, PermissionCode: "redis:read", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "集群概览", Path: "/kafka", Sort: 50, PermissionCode: "kafka:read", Enabled: true})
	put(repo.Menu{Type: "menu", ParentID: ids["/kafka"], Title: "Topic 管理", Path: "/kafka/topics", Sort: 60, PermissionCode: "kafka-topics:read", ResourceType: "kafka_topic", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "人员授权", Path: "/authz/grants", Sort: 170, PermissionCode: "authz-grants:read", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "部门与成员", Path: "/authz/orgs", Sort: 180, PermissionCode: "authz-orgs:read", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "菜单管理", Path: "/authz/menus", Sort: 190, PermissionCode: "authz-menus:read", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "审计", Path: "/authz/audit", Sort: 200, PermissionCode: "authz-audit:read", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "资源共享", Path: "/authz/shares", Sort: 210, PermissionCode: "authz-shares:read", Enabled: true})
	put(repo.Menu{Type: "menu", Title: "资源类型", Path: "/authz/types", Sort: 220, PermissionCode: "authz-types:read", Enabled: true})
	return ids
}

// TestMeMenusFiltering me/menus 过滤语义：空码全员可见、精确码可见、
// 未授权服务不可见、disabled 行不下发、*:read 通配放行全部
// （去角色化改造：授权经引擎 permission 元组实时查询）。
func TestMeMenusFiltering(t *testing.T) {
	sc := newTestSvcCtx(t)
	// kafka:topics 菜单声明 resource_type=kafka_topic，资源级解锁查询需类型
	// 已注册（未注册时 ListObjects fail-closed，菜单列表不可用）。
	registerKafkaTopic(t, sc)
	// 预置菜单行（替代原 seed：声明下发后的注册表状态）。
	ids := seedTestMenus(t, sc)

	// viewer 用户只授 authz 部分菜单（菜单级码，验证菜单级隔离）与 redis 精确码 redis:read。
	grantPerm(t, sc, "u1", "authz-grants:read")
	grantPerm(t, sc, "u1", "authz-orgs:read")
	grantPerm(t, sc, "u1", "authz-audit:read")
	grantPerm(t, sc, "u1", "redis:read")

	resp, err := NewMeMenusLogic(ctxAs("u1", []string{"viewer"}), sc).MeMenus()
	if err != nil {
		t.Fatalf("me/menus: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range resp.Items {
		seen[m.Path] = true
		switch m.PermissionCode {
		case "", "authz-grants:read", "authz-orgs:read", "authz-audit:read", "redis:read":
		default:
			t.Errorf("不应下发未授权菜单 %s(%s)", m.Path, m.PermissionCode)
		}
	}
	if !seen["/authz/orgs"] || !seen["/authz/audit"] {
		t.Errorf("已授菜单级码的子页应可见: %+v", resp.Items)
	}
	if seen["/authz/menus"] || seen["/authz/shares"] || seen["/authz/types"] {
		t.Errorf("未授菜单级码的子页不应可见（菜单级隔离）: %+v", resp.Items)
	}
	if !seen["/redis"] {
		t.Errorf("精确码 redis:read 应可见")
	}
	if !seen["/home"] {
		t.Errorf("空码菜单应全员可见（不过滤）: %+v", resp.Items)
	}
	if seen["/kafka"] || seen["/kafka/topics"] {
		t.Errorf("未授权的 kafka 不应可见")
	}
	if len(resp.Items) == 0 {
		t.Fatal("可见菜单不应为空")
	}

	// disabled 行不下发：停用 /authz/audit（按 id 定位，PUT 全量覆盖）。
	if _, err := NewUpdateMenuLogic(adminCtx(), sc).UpdateMenu(&types.UpdateMenuReq{
		Id: ids["/authz/audit"], Title: "审计", Path: "/authz/audit",
		PermissionCode: "authz-audit:read", Enabled: false,
	}); err != nil {
		t.Fatalf("停用 /authz/audit: %v", err)
	}
	resp, err = NewMeMenusLogic(ctxAs("u1", []string{"viewer"}), sc).MeMenus()
	if err != nil {
		t.Fatalf("me/menus(2): %v", err)
	}
	for _, m := range resp.Items {
		if m.Path == "/authz/audit" {
			t.Errorf("disabled 行不应下发")
		}
	}

	// 通配：*:read（permission:*:read 通配对象）放行全部服务菜单。
	grantPerm(t, sc, "u2", "*:read")
	resp, err = NewMeMenusLogic(ctxAs("u2", []string{"super_admin"}), sc).MeMenus()
	if err != nil {
		t.Fatalf("me/menus(通配): %v", err)
	}
	foundKafka, foundRedis := false, false
	for _, m := range resp.Items {
		if m.Path == "/kafka/topics" {
			foundKafka = true
		}
		if m.Path == "/redis" {
			foundRedis = true
		}
	}
	if !foundKafka || !foundRedis {
		t.Errorf("*:read 通配应放行 /kafka/topics 与 /redis: %d 行", len(resp.Items))
	}
}

// TestPermGrantedMatrix permGranted 与前端 hasPermission 判定矩阵对齐
// （packages/shared permission.test.ts 同源用例）。
func TestPermGrantedMatrix(t *testing.T) {
	cases := []struct {
		code  string
		perms []string
		want  bool
	}{
		{"authz:read", []string{"authz:read"}, true},               // 精确码
		{"authz-grants:read", []string{"authz-grants:read"}, true}, // 菜单级精确码
		{"authz-grants:read", []string{"*:read"}, true},            // 菜单级码被 *:动作 通配
		{"authz-grants:read", []string{"authz:read"}, false},       // 旧服务级码不再匹配菜单级码
		{"redis:read", []string{"*:read"}, true},                   // *:动作
		{"redis:read", []string{"redis:*"}, true},                  // 服务:*（跨动作）
		{"redis:read", []string{"*:*"}, true},                      // 全通配
		{"redis:read", []string{"*"}, true},                        // 全通配（裸）
		{"redis:read", []string{"redis"}, true},                    // 服务级码
		{"redis:read", []string{"authz:read"}, false},              // 无关码
		{"redis:read", []string{"redis:manage"}, false},            // read 不被其他动作兜底
		{"authz:read", []string{"authz:manage", "*:write"}, false}, // 非同动作通配
		{"redis:read", nil, false},                                 // 空集
		{"redis:read", []string{"*:manage", "redis:read"}, true},   // 短路精确码
	}
	for _, c := range cases {
		if got := permGranted(c.code, c.perms); got != c.want {
			t.Errorf("permGranted(%q, %v) = %v, 期望 %v", c.code, c.perms, got, c.want)
		}
	}
}

// TestMenuCRUD 管理面 CRUD：新建（id 自增/空码合法）/权限码重复 409/必填
// 400/type 锚点不可改/未命中 404/更新覆盖/删除；三级菜单层级校验与按钮行
// 约束（parentId 必填、页面级与资源级按钮码均必填）。
func TestMenuCRUD(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	created, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Title: "演示页", Path: "/demo",
		Sort: 999, PermissionCode: "demo:read",
	})
	if err != nil {
		t.Fatalf("新建: %v", err)
	}
	if created.Id == 0 || !created.Enabled {
		t.Errorf("新建行应回填自增 id 且默认 enabled=true: %+v", created)
	}

	// 空码菜单合法（方案 B：空 = 全员可见）。
	free, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Title: "自由页", Path: "/free",
	})
	if err != nil || free.PermissionCode != "" {
		t.Errorf("空码菜单应合法: %v %+v", err, free)
	}

	// 权限码全表唯一：重复 409（键名已删，唯一性落在权限码上）。
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Title: "重复", Path: "/demo2", PermissionCode: "demo:read",
	}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 409 {
		t.Errorf("权限码重复应 409, 实际 %v", err)
	}

	// 必填缺失 400：菜单行缺 path（导航入口）。
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Title: "x"}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("缺 path 应 400, 实际 %v", err)
	}

	// 三级菜单层级：顶级 → 一级 → 二级 OK；二级再挂子级 400。
	top, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Title: "顶级", Path: "/t"})
	if err != nil {
		t.Fatalf("建顶级菜单: %v", err)
	}
	mid, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Title: "一级", Path: "/t/m", ParentId: top.Id})
	if err != nil {
		t.Fatalf("建一级菜单: %v", err)
	}
	leaf, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Title: "二级", Path: "/t/m/l", ParentId: mid.Id})
	if err != nil {
		t.Fatalf("建二级菜单: %v", err)
	}
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Title: "三级", Path: "/t/m/l/x", ParentId: leaf.Id}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("三级菜单再挂子级应 400, 实际 %v", err)
	}

	// 按钮行：parentId 必填 400；页面级/资源级按钮码均必填 400。
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Type: "button", Title: "b"}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("按钮缺 parentId 应 400, 实际 %v", err)
	}
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{Type: "button", ParentId: top.Id, Title: "页面按钮"}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("页面级按钮缺码应 400, 实际 %v", err)
	}
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type: "button", ParentId: top.Id, Title: "冻结",
		Relation: "can_edit", ResourceType: "kafka_topic",
	}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("资源级按钮缺码应 400, 实际 %v", err)
	}
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type: "button", ParentId: top.Id, Title: "新建", PermissionCode: "demo-t:create",
	}); err != nil {
		t.Fatalf("建页面级按钮: %v", err)
	}
	resBtn, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type: "button", ParentId: top.Id, Title: "冻结",
		PermissionCode: "demo-t:freeze", Relation: "can_edit", ResourceType: "kafka_topic",
	})
	if err != nil || resBtn.PermissionCode != "demo-t:freeze" {
		t.Errorf("资源级按钮应带码落库（动作分派标识）: %v %+v", err, resBtn)
	}

	// 更新：全量覆盖（id 定位）；type 锚点不可改；未命中 404。
	up, err := NewUpdateMenuLogic(ctx, sc).UpdateMenu(&types.UpdateMenuReq{
		Id: created.Id, Title: "改名", Path: "/demo/new", PermissionCode: "demo:read", Enabled: false,
	})
	if err != nil || up.Title != "改名" || up.Enabled {
		t.Fatalf("更新: %v %+v", err, up)
	}
	if _, err := NewUpdateMenuLogic(ctx, sc).UpdateMenu(&types.UpdateMenuReq{
		Id: created.Id, Type: "button", Title: "改名", Path: "/demo/new", Enabled: false,
	}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 400 {
		t.Errorf("type 不可修改应 400, 实际 %v", err)
	}
	if _, err := NewUpdateMenuLogic(ctx, sc).UpdateMenu(&types.UpdateMenuReq{
		Id: 424242, Title: "x", Path: "/x", Enabled: true,
	}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 404 {
		t.Errorf("更新未命中应 404, 实际 %v", err)
	}
	if err := NewDeleteMenuLogic(ctx, sc).DeleteMenu(&types.MenuIdReq{Id: 424242}); kiterrors.From(err) == nil || kiterrors.From(err).Status != 404 {
		t.Errorf("删除未命中应 404, 实际 %v", err)
	}

	// 删除后不再存在。
	if err := NewDeleteMenuLogic(ctx, sc).DeleteMenu(&types.MenuIdReq{Id: created.Id}); err != nil {
		t.Fatalf("删除: %v", err)
	}
	list, _ := sc.MenuRepo.List(context.Background())
	for _, m := range list {
		if m.ID == created.Id {
			t.Errorf("删除后不应存在")
		}
	}
}

// TestMenuOrphanGrantCleanup 孤儿授权防护（键名删除 + 权限码唯一后的新
// 场景）：停用清理 read/manage；派生 manage 码被其他启用菜单（同 base 不同
// 码）引用时不误清；改码清理旧码；删除级联清理页面级按钮码。
func TestMenuOrphanGrantCleanup(t *testing.T) {
	sc := newTestSvcCtx(t)
	ctx := adminCtx()

	mustMenu := func(req *types.CreateMenuReq) *types.MenuResp {
		t.Helper()
		resp, err := NewCreateMenuLogic(ctx, sc).CreateMenu(req)
		if err != nil {
			t.Fatalf("建菜单 %s: %v", req.Title, err)
		}
		return resp
	}
	// 两个菜单同 base（svc-a）不同码：read 行 + freeze 行，派生共享
	// svc-a:manage（同一码空间下码本身不可重复，共享落在派生 manage 上）。
	m1 := mustMenu(&types.CreateMenuReq{Title: "服务a", Path: "/svc-a", Sort: 999, PermissionCode: "svc-a:read"})
	m2 := mustMenu(&types.CreateMenuReq{Title: "服务a冻结", Path: "/svc-a/freeze", Sort: 999, PermissionCode: "svc-a:freeze"})
	grantPerm(t, sc, "u1", "svc-a:read")
	grantPerm(t, sc, "u1", "svc-a:manage")
	grantPerm(t, sc, "u1", "svc-a:freeze")

	// 停用 m1（保留码）：read 清理；manage/freeze 仍被启用 m2（同 base
	// 派生 manage、自身 freeze）引用 → 保留（不误清）。
	if _, err := NewUpdateMenuLogic(ctx, sc).UpdateMenu(&types.UpdateMenuReq{
		Id: m1.Id, Title: "服务a", Path: "/svc-a", PermissionCode: "svc-a:read", Enabled: false,
	}); err != nil {
		t.Fatalf("停用 m1: %v", err)
	}
	if n := tupleCount(t, sc, "svc-a:read"); n != 0 {
		t.Errorf("停用后 read 码应清理, 实际 %d 条", n)
	}
	if n := tupleCount(t, sc, "svc-a:manage"); n != 1 {
		t.Errorf("manage 码仍被 m2 派生引用不应清理, 实际 %d 条", n)
	}
	if n := tupleCount(t, sc, "svc-a:freeze"); n != 1 {
		t.Errorf("freeze 码仍被 m2 引用不应清理, 实际 %d 条", n)
	}

	// 重新启用 + 再授予，然后改码：旧 read 清理；manage 因 m2 仍派生引用
	// 不误清；新码随菜单生效。
	if _, err := NewUpdateMenuLogic(ctx, sc).UpdateMenu(&types.UpdateMenuReq{
		Id: m1.Id, Title: "服务a", Path: "/svc-a", PermissionCode: "svc-a:read", Enabled: true,
	}); err != nil {
		t.Fatalf("重新启用 m1: %v", err)
	}
	grantPerm(t, sc, "u1", "svc-a:read")
	grantPerm(t, sc, "u1", "svc-a:manage")
	if _, err := NewUpdateMenuLogic(ctx, sc).UpdateMenu(&types.UpdateMenuReq{
		Id: m1.Id, Title: "服务a", Path: "/svc-a", PermissionCode: "svc-a2:read", Enabled: true,
	}); err != nil {
		t.Fatalf("改码 m1: %v", err)
	}
	if n := tupleCount(t, sc, "svc-a:read"); n != 0 {
		t.Errorf("改码后旧 read 码应清理, 实际 %d 条", n)
	}
	if n := tupleCount(t, sc, "svc-a:manage"); n != 1 {
		t.Errorf("manage 码仍被 m2 派生引用不应清理, 实际 %d 条", n)
	}

	// 页面级按钮挂在 m1 下（码 = 授权码），删除 m1 级联按钮：
	// 按钮码 + m1 的码清理；m2 的 freeze/manage 不误清。
	if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type: "button", ParentId: m1.Id, Title: "服务a新建", PermissionCode: "svc-a:create",
	}); err != nil {
		t.Fatalf("建页面级按钮: %v", err)
	}
	grantPerm(t, sc, "u1", "svc-a:create")
	if err := NewDeleteMenuLogic(ctx, sc).DeleteMenu(&types.MenuIdReq{Id: m1.Id}); err != nil {
		t.Fatalf("删除 m1: %v", err)
	}
	for _, c := range []string{"svc-a:create", "svc-a2:read"} {
		if n := tupleCount(t, sc, c); n != 0 {
			t.Errorf("删除后孤儿码 %s 应清理, 实际 %d 条", c, n)
		}
	}
	if n := tupleCount(t, sc, "svc-a:freeze"); n != 1 {
		t.Errorf("m2 仍启用, freeze 码不应清理, 实际 %d 条", n)
	}
	if n := tupleCount(t, sc, "svc-a:manage"); n != 1 {
		t.Errorf("manage 码仍被 m2 派生引用不应清理, 实际 %d 条", n)
	}

	// 删除 m2——freeze/manage 最后一个使用者移除，两码元组清理（孤儿防护）。
	if err := NewDeleteMenuLogic(ctx, sc).DeleteMenu(&types.MenuIdReq{Id: m2.Id}); err != nil {
		t.Fatalf("删除 m2: %v", err)
	}
	for _, c := range []string{"svc-a:freeze", "svc-a:manage"} {
		if n := tupleCount(t, sc, c); n != 0 {
			t.Errorf("删除 m2 后孤儿码 %s 应清理, 实际 %d 条", c, n)
		}
	}
}

// TestSplitResourceTypes 覆盖资源类型多值拆分：逗号分隔、trim、去空、去重、
// 顺序保持；空串返回 nil（纯菜单），单值等价于长度 1 列表（存量兼容）。
func TestSplitResourceTypes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"空串", "", nil},
		{"纯逗号", ",,", nil},
		{"单值", "kafka_topic", []string{"kafka_topic"}},
		{"多值", "kafka_topic,kafka_consumer", []string{"kafka_topic", "kafka_consumer"}},
		{"带空格", " kafka_topic , kafka_consumer ", []string{"kafka_topic", "kafka_consumer"}},
		{"去空与去重", "a,,b,a", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitResourceTypes(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: splitResourceTypes(%q)=%v 期望 %v", c.name, c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: splitResourceTypes(%q)[%d]=%q 期望 %q", c.name, c.in, i, got[i], c.want[i])
			}
		}
	}
	if _, err := validateResourceTypes("kafka_topic,kafka_consumer"); err != nil {
		t.Errorf("多值校验不应报错: %v", err)
	}
	if _, err := validateResourceTypes("kafka_topic, BadType!"); err == nil {
		t.Errorf("非法类型名应报错")
	}
	if _, err := validateResourceTypes(""); err != nil {
		t.Errorf("空串应合法: %v", err)
	}
}
