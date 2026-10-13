package repo

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

// testRuleRepo 用同一组断言约束 RuleRepo 的全部实现（内存必跑；MySQL 走
// TestMySQLRepos 环境开关）。
func testRuleRepo(t *testing.T, r RuleRepo) {
	ctx := context.Background()

	// 新建：ID=0 由实现自增生成，字段原样落库（Action 空 = 服务级授权）。
	up, err := r.Upsert(ctx, Rule{RoleKey: "admin", Prefix: "order:", Action: ""})
	if err != nil {
		t.Fatalf("upsert(新建): %v", err)
	}
	if up.ID == 0 {
		t.Fatal("upsert(新建) 应生成正整数 ID")
	}
	if up.RoleKey != "admin" || up.Prefix != "order:" || up.Action != "" {
		t.Fatalf("upsert(新建) 字段应原样落库: %+v", up)
	}

	// List 应恰好返回这一条。
	list, err := r.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != up.ID || list[0].RoleKey != "admin" ||
		list[0].Prefix != "order:" || list[0].Action != "" {
		t.Fatalf("list 应恰好返回新建的规则: %+v", list)
	}

	// 更新：同 ID 全量覆盖其余字段。
	up2, err := r.Upsert(ctx, Rule{ID: up.ID, RoleKey: "ops", Prefix: "user:", Action: "read"})
	if err != nil {
		t.Fatalf("upsert(更新): %v", err)
	}
	if up2.ID != up.ID || up2.RoleKey != "ops" || up2.Prefix != "user:" || up2.Action != "read" {
		t.Fatalf("upsert(更新) 应覆盖字段: %+v", up2)
	}
	list, err = r.List(ctx)
	if err != nil || len(list) != 1 || list[0].RoleKey != "ops" || list[0].Action != "read" {
		t.Fatalf("list 应反映更新: %v %v", list, err)
	}

	// 更新不存在的 ID 返回 ErrNotFound（自增主键不支持手工指定 ID 新建）。
	if _, err := r.Upsert(ctx, Rule{ID: 9901, RoleKey: "auditor", Prefix: "audit:", Action: ""}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("upsert 不存在的 ID 应返回 ErrNotFound, 实际 %v", err)
	}

	// 第二条新建产生不同的正整数 ID。
	up3, err := r.Upsert(ctx, Rule{RoleKey: "auditor", Prefix: "audit:", Action: ""})
	if err != nil || up3.ID == 0 || up3.ID == up.ID {
		t.Fatalf("upsert(第二条) 应生成不同的正整数 ID: %v %+v", err, up3)
	}
	list, err = r.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list 应有 2 条: %v %v", list, err)
	}

	// Delete 后 List 不再包含；删除不存在的 ID 返回 ErrNotFound。
	if err := r.Delete(ctx, up.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err = r.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != up3.ID {
		t.Fatalf("delete 后 list 应只剩第二条: %v %v", list, err)
	}
	if err := r.Delete(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete 缺失 ID 应返回 ErrNotFound, 实际 %v", err)
	}
}

// testResourceTypeRepo 用同一组断言约束 ResourceTypeRepo 的全部实现。
func testResourceTypeRepo(t *testing.T, r ResourceTypeRepo) {
	ctx := context.Background()

	// Create 落库并填充 CreatedAt（id 为自增主键，回填正整数）；ManageCode
	// 原样落库（名单管理码，可空）。
	rt, err := r.Create(ctx, ResourceType{Name: "order", DslBlock: "model order {...}", ManageCode: "order-tools:manage"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rt.ID == 0 || rt.Name != "order" || rt.DslBlock != "model order {...}" || rt.ManageCode != "order-tools:manage" {
		t.Fatalf("create 字段应原样落库且回填 ID: %+v", rt)
	}
	if rt.CreatedAt.IsZero() {
		t.Fatal("create 应填充 CreatedAt")
	}

	// Get 按名取回完整记录（含自增 ID 与 ManageCode）。
	got, err := r.Get(ctx, "order")
	if err != nil || got.ID != rt.ID || got.Name != "order" || got.DslBlock != "model order {...}" || got.ManageCode != "order-tools:manage" || got.CreatedAt.IsZero() {
		t.Fatalf("get: %v %+v", err, got)
	}

	// Get 未命中返回 ErrNotFound。
	if _, err := r.Get(ctx, "no-such-type"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get 缺失名应返回 ErrNotFound, 实际 %v", err)
	}

	// 重名 Create 报错（两个实现语义一致）。
	if _, err := r.Create(ctx, ResourceType{Name: "order", DslBlock: "dup"}); err == nil {
		t.Fatal("create 重名应报错")
	}

	// List 返回全部注册项。
	if _, err := r.Create(ctx, ResourceType{Name: "user", DslBlock: "model user {...}"}); err != nil {
		t.Fatalf("create 第二个: %v", err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list 应有 2 条: %v %v", list, err)
	}
}

// testModelVersionRepo 用同一组断言约束 ModelVersionRepo 的全部实现。
// 主键改造：id 为 DB 自增主键（内部序号），幂等键是 EnginemodelID（引擎 modelID）。
func testModelVersionRepo(t *testing.T, r ModelVersionRepo) {
	ctx := context.Background()

	// 空表 Latest 返回 ErrNotFound。
	if _, err := r.Latest(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("空表 latest 应返回 ErrNotFound, 实际 %v", err)
	}

	// Save 落库（id 自增生成，EnginemodelID 为引擎 modelID），语义经 Latest 验证。
	if err := r.Save(ctx, ModelVersion{EnginemodelID: "m1", Dsl: "dsl-v1"}); err != nil {
		t.Fatalf("save v1: %v", err)
	}

	// Latest 返回最后保存的版本。
	latest, err := r.Latest(ctx)
	if err != nil || latest.Dsl != "dsl-v1" || latest.EnginemodelID != "m1" {
		t.Fatalf("latest 应返回 v1: %v %+v", err, latest)
	}
	if latest.ID == 0 || latest.CreatedAt.IsZero() {
		t.Fatalf("latest 应带正整数 ID 与 CreatedAt: %+v", latest)
	}

	// 再保存一条，Latest 翻转为最新那条（两实现同秒内也要分得出先后）。
	v1ID := latest.ID
	time.Sleep(1100 * time.Millisecond) // MySQL DATETIME 默认秒精度，跨秒保证 created_at 严格递增
	if err := r.Save(ctx, ModelVersion{EnginemodelID: "m2", Dsl: "dsl-v2"}); err != nil {
		t.Fatalf("save v2: %v", err)
	}
	latest, err = r.Latest(ctx)
	if err != nil || latest.Dsl != "dsl-v2" || latest.EnginemodelID != "m2" || latest.ID == v1ID {
		t.Fatalf("latest 应返回 v2: %v %+v", err, latest)
	}

	// 同 EnginemodelID 再 Save = 幂等覆盖（不新增行，自增 ID 不变，DSL 更新）。
	v2ID := latest.ID
	if err := r.Save(ctx, ModelVersion{EnginemodelID: "m2", Dsl: "dsl-v2b"}); err != nil {
		t.Fatalf("save 幂等覆盖 m2: %v", err)
	}
	latest, err = r.Latest(ctx)
	if err != nil || latest.EnginemodelID != "m2" || latest.Dsl != "dsl-v2b" || latest.ID != v2ID {
		t.Fatalf("latest 应返回 m2 覆盖后的版本且 ID 不变: %v %+v", err, latest)
	}
}

// testAuditRepo 用同一组断言约束 AuditRepo 的全部实现（审计先行三态 + 对账 + 分页）。
func testAuditRepo(t *testing.T, r AuditRepo) {
	ctx := context.Background()

	// CreatePending 生成正整数 ID 并以 pending 态落库，Snapshot（JSON 文本）原样存取。
	id1, err := r.CreatePending(ctx, AuditEntry{
		Operator: "u1", Action: "create_org", Target: "org:1", Snapshot: `{"org":"1"}`,
	})
	if err != nil || id1 == 0 {
		t.Fatalf("createPending: %v %d", err, id1)
	}
	entries, total, err := r.Query(ctx, AuditFilter{Operator: "u1", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(entries) != 1 {
		t.Fatalf("query(u1): %v total=%d len=%d", err, total, len(entries))
	}
	e := entries[0]
	if e.ID != id1 || e.Operator != "u1" || e.Action != "create_org" ||
		e.Target != "org:1" || e.Snapshot != `{"org":"1"}` || e.Status != AuditPending {
		t.Fatalf("query(u1) 字段回读不符: %+v", e)
	}

	// MarkCommitted：提交引擎成功后置 committed。
	if err := r.MarkCommitted(ctx, id1); err != nil {
		t.Fatalf("markCommitted: %v", err)
	}
	entries, _, err = r.Query(ctx, AuditFilter{Operator: "u1", Page: 1, PageSize: 10})
	if err != nil || len(entries) != 1 || entries[0].Status != AuditCommitted {
		t.Fatalf("markCommitted 后应为 committed: %v %+v", err, entries)
	}

	// ListPending：负 olderThan（分界线在未来）取全部 pending，规避 MySQL
	// DATETIME 秒精度下"刚创建的条目 < now"不成立的假阴性。
	id2, err := r.CreatePending(ctx, AuditEntry{Operator: "u2", Action: "grant", Target: "org:2"})
	if err != nil || id2 == 0 {
		t.Fatalf("createPending u2: %v", err)
	}
	pend, err := r.ListPending(ctx, -time.Minute)
	if err != nil {
		t.Fatalf("listPending: %v", err)
	}
	hasID2, hasID1 := false, false
	for _, p := range pend {
		if p.ID == id2 {
			hasID2 = true
		}
		if p.ID == id1 {
			hasID1 = true
		}
	}
	if !hasID2 {
		t.Fatalf("listPending 应包含 pending 条目 %d: %+v", id2, pend)
	}
	if hasID1 {
		t.Fatal("listPending 不应包含已 committed 的条目")
	}

	// MarkFailed：置 failed，条目退出 pending 集合。
	if err := r.MarkFailed(ctx, id2, "engine down"); err != nil {
		t.Fatalf("markFailed: %v", err)
	}
	entries, _, err = r.Query(ctx, AuditFilter{Operator: "u2", Page: 1, PageSize: 10})
	if err != nil || len(entries) != 1 || entries[0].Status != AuditFailed {
		t.Fatalf("markFailed 后应为 failed: %v %+v", err, entries)
	}
	pend, err = r.ListPending(ctx, -time.Minute)
	if err != nil {
		t.Fatalf("listPending(2): %v", err)
	}
	for _, p := range pend {
		if p.ID == id2 {
			t.Fatal("markFailed 后条目不应再出现在 listPending")
		}
	}

	// 未命中的标记操作返回 ErrNotFound。
	if err := r.MarkCommitted(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("markCommitted 缺失 ID 应返回 ErrNotFound, 实际 %v", err)
	}
	if err := r.MarkFailed(ctx, 999999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("markFailed 缺失 ID 应返回 ErrNotFound, 实际 %v", err)
	}

	// 分页与总数：同一 operator 建 3 条，PageSize=2 分两页取尽。
	for i := 0; i < 3; i++ {
		if _, err := r.CreatePending(ctx, AuditEntry{Operator: "p1", Action: "a", Target: "t"}); err != nil {
			t.Fatalf("createPending p1-%d: %v", i, err)
		}
	}
	p1, total, err := r.Query(ctx, AuditFilter{Operator: "p1", Page: 1, PageSize: 2})
	if err != nil || total != 3 || len(p1) != 2 {
		t.Fatalf("query 第 1 页: %v total=%d len=%d", err, total, len(p1))
	}
	p2, total, err := r.Query(ctx, AuditFilter{Operator: "p1", Page: 2, PageSize: 2})
	if err != nil || total != 3 || len(p2) != 1 {
		t.Fatalf("query 第 2 页: %v total=%d len=%d", err, total, len(p2))
	}
	// 超出范围的页返回空列表但 total 不变。
	p3, total, err := r.Query(ctx, AuditFilter{Operator: "p1", Page: 3, PageSize: 2})
	if err != nil || total != 3 || len(p3) != 0 {
		t.Fatalf("query 第 3 页: %v total=%d len=%d", err, total, len(p3))
	}

	// 空 Operator = 不过滤，总数应含此前全部条目（u1/u2/p1 共 5 条）。
	_, total, err = r.Query(ctx, AuditFilter{Page: 1, PageSize: 10})
	if err != nil || total != 5 {
		t.Fatalf("query 空 operator 应返回全部 5 条: %v total=%d", err, total)
	}
}

// testMenuRepo 用同一组断言约束 MenuRepo 的全部实现（id 为行身份）。
func testMenuRepo(t *testing.T, r MenuRepo) {
	ctx := context.Background()

	// Get 未命中返回 ErrNotFound。
	if _, err := r.Get(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get 缺失 id 应返回 ErrNotFound, 实际 %v", err)
	}

	// Create 新建：ID 自增生成，字段原样落库，时间戳由实现填充。
	m1, err := r.Create(ctx, Menu{Title: "实例管理", Icon: "Coin",
		Path: "/redis", Sort: 10, PermissionCode: "redis:read", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m1.ID == 0 || m1.Title != "实例管理" || m1.Path != "/redis" ||
		m1.Sort != 10 || m1.PermissionCode != "redis:read" || !m1.Enabled ||
		m1.CreatedAt.IsZero() || m1.UpdatedAt.IsZero() {
		t.Fatalf("create 字段应原样落库: %+v", m1)
	}

	// Create 子行：parent_id 引用父行 id。
	child, err := r.Create(ctx, Menu{ParentID: m1.ID, Type: "button", Title: "新建实例", Sort: 1, Enabled: true})
	if err != nil || child.ParentID != m1.ID {
		t.Fatalf("create 子行应保留 parent_id: %v %+v", err, child)
	}

	// Update：按 id 覆盖字段，CreatedAt/CreateBy 保持不变。
	m2, err := r.Update(ctx, Menu{ID: m1.ID, Title: "改名", Path: "/redis/x", Sort: 20, Enabled: false})
	if err != nil || m2.Title != "改名" || m2.Path != "/redis/x" || m2.Enabled {
		t.Fatalf("update 应覆盖字段: %v %+v", err, m2)
	}
	if m2.CreatedAt != m1.CreatedAt {
		t.Fatalf("update 不应改 CreatedAt: %v -> %v", m1.CreatedAt, m2.CreatedAt)
	}
	if _, err := r.Update(ctx, Menu{ID: 999, Title: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update 缺失 id 应返回 ErrNotFound, 实际 %v", err)
	}

	// 第二/三条 + List 排序：按 (sort, id) 升序。
	if _, err := r.Create(ctx, Menu{Title: "Topic 管理", Sort: 45, Enabled: true}); err != nil {
		t.Fatalf("create 第二条: %v", err)
	}
	if _, err := r.Create(ctx, Menu{Title: "集群概览", Sort: 45}); err != nil {
		t.Fatalf("create 第三条: %v", err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 4 {
		t.Fatalf("list 应有 4 条: %v %v", list, err)
	}
	if list[0].Title != "新建实例" || list[1].ID != m1.ID || list[2].Title != "Topic 管理" || list[3].Title != "集群概览" {
		t.Fatalf("list 应按 (sort, id) 升序: %+v", list)
	}

	// Get 命中回读。
	got, err := r.Get(ctx, list[3].ID)
	if err != nil || got.Title != "集群概览" {
		t.Fatalf("get: %v %+v", err, got)
	}

	// Delete 后 List 不再包含；删除不存在的 id 返回 ErrNotFound。
	if err := r.Delete(ctx, list[3].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.Delete(ctx, list[3].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete 缺失 id 应返回 ErrNotFound, 实际 %v", err)
	}
	list, _ = r.List(ctx)
	if len(list) != 3 {
		t.Fatalf("delete 后 list 应剩 3 条: %+v", list)
	}
}

// TestMemoryRepos 验证四个内存实现满足各自共享断言组。
func TestMemoryRepos(t *testing.T) {
	testRuleRepo(t, NewMemoryRuleRepo())
	testResourceTypeRepo(t, NewMemoryResourceTypeRepo())
	testModelVersionRepo(t, NewMemoryModelVersionRepo())
	testAuditRepo(t, NewMemoryAuditRepo())
	testMenuRepo(t, NewMemoryMenuRepo())
	testAttributeRepo(t, NewMemoryAttributeRepo())
	testPolicyRepo(t, NewMemoryPolicyRepo())
}

// TestNewMySQLReposInvalidDSN 验证 DSN 非法或连不上时 NewMySQLRepos 返回
// error（Ping 校验），不依赖真实 MySQL 实例。
func TestNewMySQLReposInvalidDSN(t *testing.T) {
	if _, err := NewMySQLRepos("::::not-a-dsn"); err == nil {
		t.Fatal("malformed dsn: want error, got nil")
	}
	// 本机回环保留端口，连接必然被拒绝且快速失败，不产生跨网络请求。
	if _, err := NewMySQLRepos("u:p@tcp(127.0.0.1:1)/nemo_authz_biz?charset=utf8mb4"); err == nil {
		t.Fatal("unreachable dsn: want error, got nil")
	}
}

// truncateTable 清空指定表（集成测试清场用，保证精确计数断言可重复执行）。
func truncateTable(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	if _, err := db.Exec("TRUNCATE TABLE " + table); err != nil {
		t.Fatalf("清场 %s 失败（迁移是否已执行？）: %v", table, err)
	}
}

// TestMySQLRepos 真库集成测试：仅在本机设置了 MYSQL_DSN 时执行，否则跳过，
// 保证无 MySQL 实例的机器上 `go test` 全绿。每组断言前清空对应表，
// 保证精确计数断言对可重复执行的集成测试成立（仅本包测试自知表名，
// 生产代码一律经 repo 接口操作）。
func TestMySQLRepos(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN 未设置，跳过 MySQL 集成测试")
	}
	repos, err := NewMySQLRepos(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer repos.Close()

	groups := []struct {
		name      string
		truncate  string
		assertion func(*testing.T)
	}{
		{"rule", "rules", func(t *testing.T) { testRuleRepo(t, repos.RuleRepo) }},
		{"resource_type", "resource_types", func(t *testing.T) { testResourceTypeRepo(t, repos.ResourceTypeRepo) }},
		{"model_version", "model_versions", func(t *testing.T) { testModelVersionRepo(t, repos.ModelVersionRepo) }},
		{"audit", "audits", func(t *testing.T) { testAuditRepo(t, repos.AuditRepo) }},
		{"menu", "menus", func(t *testing.T) { testMenuRepo(t, repos.MenuRepo) }},
		{"attribute", "attributes", func(t *testing.T) { testAttributeRepo(t, repos.AttributeRepo) }},
		{"policy", "policies", func(t *testing.T) { testPolicyRepo(t, repos.PolicyRepo) }},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			truncateTable(t, repos.db, g.truncate)
			g.assertion(t)
		})
	}
}

// testAttributeRepo 用同一组断言约束 AttributeRepo 的全部实现（upsert、查询、删除）。
func testAttributeRepo(t *testing.T, r AttributeRepo) {
	ctx := context.Background()

	// Set + Get：主体属性写入与查询。
	if err := r.Set(ctx, Attribute{Owner: "user:zhangsan", AttrKey: "securityLevel", AttrValue: "2"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := r.Set(ctx, Attribute{Owner: "user:zhangsan", AttrKey: "bizLine", AttrValue: "trade"}); err != nil {
		t.Fatalf("set second: %v", err)
	}
	attrs, err := r.Get(ctx, "user:zhangsan")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if attrs["securityLevel"] != "2" || attrs["bizLine"] != "trade" || len(attrs) != 2 {
		t.Fatalf("get 应返回全部属性: %+v", attrs)
	}

	// 幂等 upsert：同 key 覆盖，不产生重复。
	if err := r.Set(ctx, Attribute{Owner: "user:zhangsan", AttrKey: "securityLevel", AttrValue: "3"}); err != nil {
		t.Fatalf("set upsert: %v", err)
	}
	attrs, _ = r.Get(ctx, "user:zhangsan")
	if attrs["securityLevel"] != "3" || len(attrs) != 2 {
		t.Fatalf("upsert 应覆盖 value 不增条数: %+v", attrs)
	}

	// 资源属性与主体隔离（owner 不同命名空间）。
	if err := r.Set(ctx, Attribute{Owner: "kafka_topic:1", AttrKey: "securityLevel", AttrValue: "4"}); err != nil {
		t.Fatalf("set resource: %v", err)
	}
	resAttrs, _ := r.Get(ctx, "kafka_topic:1")
	if resAttrs["securityLevel"] != "4" || len(resAttrs) != 1 {
		t.Fatalf("资源属性应独立: %+v", resAttrs)
	}

	// 无属性 owner 返回空 map（非 ErrNotFound）。
	empty, err := r.Get(ctx, "user:nobody")
	if err != nil || len(empty) != 0 {
		t.Fatalf("无属性应返回空 map: %+v %v", empty, err)
	}

	// Delete 幂等：删存在 key 后消失；删不存在 key 不报错。
	if err := r.Delete(ctx, "user:zhangsan", "bizLine"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	attrs, _ = r.Get(ctx, "user:zhangsan")
	if _, ok := attrs["bizLine"]; ok || len(attrs) != 1 {
		t.Fatalf("delete 后 key 应消失: %+v", attrs)
	}
	if err := r.Delete(ctx, "user:zhangsan", "not-exist"); err != nil {
		t.Fatalf("delete 不存在 key 应幂等: %v", err)
	}
}

// TestNormalizeOwner 校验属性归属标识归一规则。
func TestNormalizeOwner(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"user:zhangsan", "user:zhangsan", false},
		{" org:tech-frontend#member ", "org:tech-frontend#member", false},
		{"kafka_topic:1", "kafka_topic:1", false},
		{"kafka:cluster-a", "kafka:cluster-a", false},
		{"", "", true},
		{"plain", "", true},
		{"user:", "", true},
		{":id", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeOwner(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("%q 应报错", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("%q → got=%q err=%v, want=%q", c.in, got, err, c.want)
		}
	}
}

// testPolicyRepo 用同一组断言约束 PolicyRepo 的全部实现（CRUD、ID 生成）。
func testPolicyRepo(t *testing.T, r PolicyRepo) {
	ctx := context.Background()
	base := Policy{Name: "高安全等级限制", ResourceType: "kafka_topic", Effect: "deny",
		SubjectAttr: "securityLevel", Op: OpLt, ResourceAttr: "securityLevel", Enabled: true}

	p1, err := r.Create(ctx, base)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p1.ID == 0 || p1.Name != "高安全等级限制" || !p1.Enabled {
		t.Fatalf("create 应回填 ID 且原样落库: %+v", p1)
	}
	p2, err := r.Create(ctx, Policy{Name: "业务线限制", SubjectAttr: "bizLine", Op: OpEq, Value: "trade"})
	if err != nil || p2.ID <= p1.ID {
		t.Fatalf("create 第二条: %v %+v", err, p2)
	}

	list, err := r.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != p1.ID || list[1].ID != p2.ID {
		t.Fatalf("list 应按 ID 升序: %+v %v", list, err)
	}

	// Update：同 ID 覆盖（ResourceAttr 清空改走字面量），CreatedAt 保持。
	up, err := r.Update(ctx, Policy{ID: p1.ID, Name: "改名", SubjectAttr: "securityLevel", Op: OpGte, Value: "3", Enabled: false})
	if err != nil || up.ID != p1.ID || up.Name != "改名" || up.Op != OpGte || up.Enabled {
		t.Fatalf("update 应覆盖字段: %+v %v", up, err)
	}
	if _, err := r.Update(ctx, Policy{ID: 999999, Name: "x", SubjectAttr: "a", Op: OpEq, Value: "1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update 缺失应 ErrNotFound: %v", err)
	}

	// Delete：命中删除；缺失 ErrNotFound。
	if err := r.Delete(ctx, p2.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.Delete(ctx, p2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete 缺失应 ErrNotFound: %v", err)
	}
	after, _ := r.List(ctx)
	if len(after) != 1 || after[0].ID != p1.ID {
		t.Fatalf("delete 后应剩一条: %+v", after)
	}
}

// TestEvaluatePolicy 覆盖条件求值（数值/字面量/缺属性；effect 与 enabled
// 的过滤由评估层承担，本函数纯条件求值——条件命中即 true，与效果无关）。
func TestEvaluatePolicy(t *testing.T) {
	sub := map[string]string{"securityLevel": "2", "bizLine": "trade"}
	res := map[string]string{"securityLevel": "4"}

	cases := []struct {
		name string
		p    Policy
		want bool
	}{
		{"数值 lt 命中（复现旧 required_level：2 < 4）", Policy{Effect: "deny", Enabled: true, SubjectAttr: "securityLevel", Op: OpLt, ResourceAttr: "securityLevel"}, true},
		{"数值 gte 不命中", Policy{Effect: "deny", Enabled: true, SubjectAttr: "securityLevel", Op: OpGte, ResourceAttr: "securityLevel"}, false},
		{"字面量 eq 命中", Policy{Effect: "deny", Enabled: true, SubjectAttr: "bizLine", Op: OpEq, Value: "trade"}, true},
		{"字面量 neq 不命中", Policy{Effect: "deny", Enabled: true, SubjectAttr: "bizLine", Op: OpNeq, Value: "trade"}, false},
		{"effect/enable 过滤由评估层承担（纯求值只看条件）", Policy{Effect: "allow", Enabled: false, SubjectAttr: "securityLevel", Op: OpLt, ResourceAttr: "securityLevel"}, true},
		{"主体缺属性不命中", Policy{Effect: "deny", Enabled: true, SubjectAttr: "missing", Op: OpEq, Value: "x"}, false},
		{"资源缺属性不命中", Policy{Effect: "deny", Enabled: true, SubjectAttr: "securityLevel", Op: OpLt, ResourceAttr: "missing"}, false},
		{"非数值 lt 不命中（不比较）", Policy{Effect: "deny", Enabled: true, SubjectAttr: "bizLine", Op: OpLt, ResourceAttr: "securityLevel"}, false},
		{"资源类型过滤由调用方承担", Policy{Effect: "deny", Enabled: true, SubjectAttr: "securityLevel", Op: OpLt, ResourceAttr: "securityLevel"}, true},
	}
	for _, c := range cases {
		if got := EvaluatePolicy(c.p, sub, res); got != c.want {
			t.Fatalf("%s: got=%v want=%v", c.name, got, c.want)
		}
	}
}

// TestValidPolicyEffect 覆盖效果取值归一（deny/allow/空合法，其余非法）。
func TestValidPolicyEffect(t *testing.T) {
	for _, ok := range []string{"", "deny", "allow", "Deny", "ALLOW"} {
		if !ValidPolicyEffect(ok) {
			t.Errorf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"permit", "deny;drop", "permitted"} {
		if ValidPolicyEffect(bad) {
			t.Errorf("%q 应非法", bad)
		}
	}
	if EffectOf("") != EffectDeny || EffectOf("allow") != EffectAllow || EffectOf("ALLOW") != EffectAllow || EffectOf("x") != EffectDeny {
		t.Errorf("EffectOf 归一化错误")
	}
}
