// engine_mysql_test.go 是 M3b 模型测试全矩阵：
// 四场景断言组（租户隔离/层级继承/共享与越权转授/平台角色×规则组合）在
// memory 与 mysql 两种 datastore 上跑同一组断言（TestModelMatrixMemory /
// TestModelMatrixMySQL），验证双后端 Check 语义一致。
// MySQL 组需要真库：环境变量 MYSQL_DSN 提供连接串（库可任意，测试内部强制
// 切换到引擎库 nemo_authz，防止 OpenFGA 表误建进业务库）；未设置时 skip。
package engine

import (
	"context"
	"os"
	"slices"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// matrixStoreName 是矩阵测试专用 Store 名（与 PoC 测试隔离；MySQL 后端
// 跨测试运行复用同一持久 Store，依赖 WriteTuples/EnsureModel 幂等）。
const matrixStoreName = "matrix-test-store"

// startTestEngineDS 启动指定 datastore 的测试引擎并注册 Cleanup，返回 Handle。
// dsn 仅在 mysql datastore 时传入。
func startTestEngineDS(t *testing.T, datastore, dsn string) *Handle {
	t.Helper()

	h, err := Start(context.Background(), Options{
		HTTPAddr:  "127.0.0.1:0", // 随机端口，避免并行冲突
		StoreName: matrixStoreName,
		Datastore: datastore,
		MySQLDSN:  dsn,
	})
	if err != nil {
		t.Fatalf("启动引擎(datastore=%s): %v", datastore, err)
	}
	t.Cleanup(h.Stop)
	return h
}

// engineMySQLDSN 读取测试用 MySQL DSN：取 MYSQL_DSN 环境变量，强制把库名
// 切换为引擎库 nemo_authz（引擎表 store/authorization_model/tuple/assertion
// 只允许落引擎库，防误建进业务库）；未设置时 skip 本测试。
func engineMySQLDSN(t *testing.T) string {
	t.Helper()

	raw := os.Getenv("MYSQL_DSN")
	if raw == "" {
		t.Skip("MYSQL_DSN 未设置，跳过 MySQL datastore 测试")
	}
	cfg, err := mysqldriver.ParseDSN(raw)
	if err != nil {
		t.Fatalf("解析 MYSQL_DSN: %v", err)
	}
	cfg.DBName = "nemo_authz"
	return cfg.FormatDSN()
}

// matrixTuples 构造四场景矩阵的种子元组。
func matrixTuples() []Tuple {
	return []Tuple{
		// 层级链：mroot → mchild → mleaf（两层父子，parent/child 互逆成对：
		// child 供判定面 member from child 聚合，模型 v5）。
		{User: "org:mroot", Relation: "parent", Object: "org:mchild"},
		{User: "org:mchild", Relation: "parent", Object: "org:mleaf"},
		{User: "org:mchild", Relation: "child", Object: "org:mroot"},
		{User: "org:mleaf", Relation: "child", Object: "org:mchild"},
		// 订单挂组织：m1/m2 挂 mchild，m4 挂 mleaf，m3 挂另一租户 mother。
		{User: "org:mchild", Relation: "org", Object: "order:m1"},
		{User: "org:mchild", Relation: "org", Object: "order:m2"},
		{User: "org:mleaf", Relation: "org", Object: "order:m4"},
		{User: "org:mother", Relation: "org", Object: "order:m3"},
		// u1 是 mleaf 的 member（子树末端成员，经 child 聚合在整条链上均为成员），
		// u9 是另一租户 mother 的 member。
		{User: "user:u1", Relation: "member", Object: "org:mleaf"},
		{User: "user:u9", Relation: "member", Object: "org:mother"},
		// u1 是 m1 的 owner（editor/can_delete/can_share 由此派生）。
		{User: "user:u1", Relation: "owner", Object: "order:m1"},
		// u3 仅是 m2 的 viewer（共享场景负样本）。
		{User: "user:u3", Relation: "viewer", Object: "order:m2"},
		// admin1 是 mchild 的 org 管理员（非 owner，权限全靠模型规则派生）。
		{User: "user:admin1", Relation: "admin", Object: "org:mchild"},
		// 平台角色：u1 是 pr1 的 assignee，u3 是 pr2 的 assignee。
		{User: "user:u1", Relation: "assignee", Object: "platform_role:pr1"},
		{User: "user:u3", Relation: "assignee", Object: "platform_role:pr2"},
	}
}

// runModelMatrix 是双 datastore 共享的四场景断言组。
func runModelMatrix(t *testing.T, h *Handle) {
	t.Helper()
	ctx := context.Background()

	// EnsureModel（含幂等二次调用验证）。
	dsl, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	storeID, modelID, err := h.EnsureModel(ctx, string(dsl))
	if err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if storeID == "" || modelID == "" {
		t.Fatalf("EnsureModel 返回空 ID")
	}
	storeID2, _, err := h.EnsureModel(ctx, string(dsl))
	if err != nil {
		t.Fatalf("EnsureModel 二次调用: %v", err)
	}
	if storeID2 != storeID {
		t.Errorf("EnsureModel 幂等失败：StoreID 变化 %s -> %s", storeID, storeID2)
	}

	// 写种子元组（含幂等二次写入验证；MySQL 后端跨运行持久，幂等保证可重跑）。
	tuples := matrixTuples()
	if err := h.WriteTuples(ctx, tuples); err != nil {
		t.Fatalf("WriteTuples: %v", err)
	}
	if err := h.WriteTuples(ctx, tuples); err != nil {
		t.Errorf("WriteTuples 幂等失败（二次写入报错）: %v", err)
	}

	// checkAll 断言同一 (user, relation, object) 的一组期望值。
	checkAll := func(user, relation, object string, want bool) {
		t.Helper()
		allowed, err := h.Check(ctx, user, relation, object)
		if err != nil {
			t.Fatalf("Check(%s,%s,%s): %v", user, relation, object, err)
		}
		if allowed != want {
			t.Errorf("Check(%s,%s,%s)=%v，期望 %v", user, relation, object, allowed, want)
		}
	}

	// ① 租户隔离：本租户成员/无关用户看不到他租户订单（双向）。
	checkAll("user:u9", "can_view", "order:m1", false) // 他租户 member 不可见
	checkAll("user:u9", "can_view", "order:m2", false)
	checkAll("user:u1", "can_view", "order:m3", false) // u1 看不到 mother 租户订单
	checkAll("user:u3", "can_view", "order:m1", false) // 同租户但无关系（非 member 非 viewer）不可见

	// ② 层级继承（模型 v5 子→父聚合）：u1 是 mleaf member，聚合为 mchild/mroot
	// 成员——可见 m4（挂叶子）、m1/m2（挂中层 mchild；m2 的 viewer 本是 u3，
	// member-from-org 亦可见）。
	checkAll("user:u1", "can_view", "order:m1", true)
	checkAll("user:u1", "can_view", "order:m2", true)
	checkAll("user:u1", "can_view", "order:m4", true)

	// ③ 共享与越权转授：
	//    - owner 可分享，viewer 只可见不可分享（越权转授拒绝）；
	//    - org 管理员经 sharer 规则（admin from org）可分享，但非 owner 不能删；
	//    - owner 转授 viewer 给 u4 后：u4 可见但不可再转授、不可编辑。
	checkAll("user:u1", "can_share", "order:m1", true)
	checkAll("user:u3", "can_share", "order:m2", false)
	checkAll("user:u3", "can_view", "order:m2", true)
	checkAll("user:admin1", "can_share", "order:m1", true)
	checkAll("user:admin1", "can_delete", "order:m1", false)
	checkAll("user:admin1", "can_view", "order:m2", true) // admin 经 member 链可见

	// owner 转授：写 u4 viewer m1，验证可见但无转授/编辑权。
	grant := []Tuple{{User: "user:u4", Relation: "viewer", Object: "order:m1"}}
	if err := h.WriteTuples(ctx, grant); err != nil {
		t.Fatalf("WriteTuples(转授): %v", err)
	}
	checkAll("user:u4", "can_view", "order:m1", true)
	checkAll("user:u4", "can_share", "order:m1", false)
	checkAll("user:u4", "can_edit", "order:m1", false)

	// 转授回收：删除元组后权限回退（删路径双后端同语义）。
	if err := h.DeleteTuples(ctx, grant); err != nil {
		t.Fatalf("DeleteTuples(转授回收): %v", err)
	}
	checkAll("user:u4", "can_view", "order:m1", false)

	// ④ 平台角色×规则组合：
	//    - 平台角色 assignee 关系按授予精确成立；
	//    - 平台角色不旁路资源规则（u3 有角色但 m2 的 can_share 仍为 false）；
	//    - 权限只经模型规则派生：admin1 的分享权来自 admin from org 规则而非
	//      任何平台角色（③ 已断言 can_share=true，此处补角色无贡献的对照）。
	checkAll("user:u1", "assignee", "platform_role:pr1", true)
	checkAll("user:u3", "assignee", "platform_role:pr2", true)
	checkAll("user:u3", "assignee", "platform_role:pr1", false)
	checkAll("user:u3", "can_share", "order:m2", false) // 角色不旁路
	checkAll("user:admin1", "assignee", "platform_role:pr1", false)

	// ListObjects 补充断言：u1 可见含两级继承的 m1/m4，不含他租户 m3。
	objects, err := h.ListObjects(ctx, "user:u1", "can_view", "order")
	if err != nil {
		t.Fatalf("ListObjects(u1): %v", err)
	}
	for _, want := range []string{"order:m1", "order:m4"} {
		if !slices.Contains(objects, want) {
			t.Errorf("ListObjects 应含 %s，实际 %v", want, objects)
		}
	}
	if slices.Contains(objects, "order:m3") {
		t.Errorf("ListObjects 不应含他租户 order:m3，实际 %v", objects)
	}
}

// TestModelMatrixMemory 在 memory datastore 上跑全矩阵。
func TestModelMatrixMemory(t *testing.T) {
	runModelMatrix(t, startTestEngineDS(t, DatastoreMemory, ""))
}

// TestModelMatrixMySQL 在 mysql datastore（真库）上跑全矩阵；
// MYSQL_DSN 未设置时 skip（engineMySQLDSN 内部处理）。
func TestModelMatrixMySQL(t *testing.T) {
	h := startTestEngineDS(t, DatastoreMySQL, engineMySQLDSN(t))
	// MySQL 后端建连/迁移有真实 IO，EnsureModel 前先确认引擎可达。
	if err := h.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	runModelMatrix(t, h)
}
