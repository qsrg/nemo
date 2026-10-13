// syncer_test.go 覆盖组织同步对账核心：首次全量落库/引擎元组、幂等（二轮
// 零动作）、调岗（删旧写新 member）、组织撤销（无引用删除/有授权引用跳过）、
// 悬空归属跳过与同步审计落库。CAS 侧用 httptest 假目录服务，引擎用真
// memory 后端（与 logic 层测试同基建思路，保证元组读写语义真实）。
package orgsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"nemo/services/authz/internal/casdir"
	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// mockCAS 是假 CAS 目录服务：持有可变的组织/用户数据（测试内直接改字段
// 模拟 CAS 侧变更），实现 /orgs 与 /users（含分页）两个契约端点。
type mockCAS struct {
	// Orgs 组织节点全集（测试内直接改）。
	Orgs []casdir.Org
	// Users 用户全集（测试内直接改）。
	Users []casdir.DirUser
	// srv httptest 服务器（Close 由测试 Cleanup 负责）。
	srv *httptest.Server
}

// newMockCAS 启动假 CAS 服务并返回配好的客户端。
func newMockCAS(t *testing.T) (*mockCAS, *casdir.Client) {
	t.Helper()
	m := &mockCAS{}
	mux := http.NewServeMux()
	mux.HandleFunc("/orgs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": m.Orgs, "total": len(m.Orgs)})
	})
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 100 {
			size = 100
		}
		start := (page - 1) * size
		end := start + size
		if start > len(m.Users) {
			start = len(m.Users)
		}
		if end > len(m.Users) {
			end = len(m.Users)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": m.Users[start:end], "total": len(m.Users)})
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m, casdir.NewClient(m.srv.URL, false, 2*time.Second)
}

// seedOrgUsers 写入测试组织的初始 CAS 数据：root + 两个部门，各部门成员。
func (m *mockCAS) seedOrgUsers() {
	m.Orgs = []casdir.Org{
		{ID: "root", Name: "nemo 平台"},
		{ID: "tech", Name: "技术部", ParentID: "root"},
		{ID: "product", Name: "产品部", ParentID: "root"},
	}
	m.Users = []casdir.DirUser{
		{Username: "zhangsan", OrgID: "tech"},
		{Username: "lisi", OrgID: "tech"},
		{Username: "wangwu", OrgID: "product"},
	}
}

// newTestSvcCtx 经 svc.NewServiceContext 真实装配：随机端口 memory 引擎 +
// 内存 repo，并绑定 model.fga（与 logic 层测试同基建思路）。
func newTestSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	var c config.Config
	c.Name = "orgsync-test"
	c.Port = 0
	c.Engine.HTTPAddr = "127.0.0.1:0"
	c.Engine.StoreName = "orgsync-test-store"
	c.Engine.Datastore = engine.DatastoreMemory

	sc := svc.NewServiceContext(c)
	t.Cleanup(sc.Stop)

	dsl, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	if _, _, err := sc.Engine.EnsureModel(context.Background(), string(dsl)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	return sc
}

// tupleCount 统计引擎里指定对象的指定关系元组条数。
func tupleCount(t *testing.T, sc *svc.ServiceContext, object, relation string) int {
	t.Helper()
	tuples, err := sc.Engine.ReadTuples(context.Background(), object, relation)
	if err != nil {
		t.Fatalf("ReadTuples(%s, %s): %v", object, relation, err)
	}
	return len(tuples)
}

// TestSyncFirstRunAndIdempotent 首轮全量同步（注册表 + parent/member 元组），
// 二轮零动作（幂等收敛），同步审计 committed。
func TestSyncFirstRunAndIdempotent(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.seedOrgUsers()
	ctx := context.Background()

	res, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("首轮同步: %v", err)
	}
	// 每个非根部门写 parent + child 两条互逆元组（模型 v5 判定方向）。
	if res.OrgsUpserted != 3 || res.ParentsWritten != 4 || res.MembersWritten != 3 {
		t.Fatalf("首轮摘要 = %+v, 期望 upsert=3 parent+child=4 member=3", res)
	}
	if got := tupleCount(t, sc, "org:tech", "parent"); got != 1 {
		t.Errorf("org:tech parent 元组 = %d, 期望 1", got)
	}
	if got := tupleCount(t, sc, "org:root", "child"); got != 2 {
		// child 元组挂在父对象上：root 的孩子 = tech、product 两条。
		t.Errorf("org:root child 元组 = %d, 期望 2", got)
	}
	if got := tupleCount(t, sc, "org:tech", "member"); got != 2 {
		t.Errorf("org:tech member 元组 = %d, 期望 2", got)
	}
	org, err := sc.Orgs.Get(ctx, "tech")
	if err != nil || org.Name != "技术部" || org.ParentID != "root" || org.UpdateBy != "system" {
		t.Errorf("注册表 tech = %+v, err=%v, 期望 名称/父/操作人正确", org, err)
	}

	// 二轮：目标与现状一致，零动作。
	res2, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("二轮同步: %v", err)
	}
	if res2.OrgsUpserted != 3 || res2.ParentsWritten != 0 || res2.MembersWritten != 0 ||
		res2.MembersDeleted != 0 || res2.ParentsDeleted != 0 {
		t.Errorf("二轮摘要 = %+v, 期望除幂等 upsert 外零动作", res2)
	}

	// 审计：两条 sync_orgs 均 committed。
	entries, _, err := sc.AuditRepo.Query(ctx, repo.AuditFilter{Operator: systemOperator})
	if err != nil {
		t.Fatalf("查审计: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("sync_orgs 审计 = %d 条, 期望 2", len(entries))
	}
	for _, e := range entries {
		if e.Status != repo.AuditCommitted {
			t.Errorf("审计状态 = %q, 期望 committed", e.Status)
		}
	}
}

// TestSyncMemberMoves 调岗场景：用户 orgId 变更 → 删旧组织 member + 写新组织 member。
func TestSyncMemberMoves(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.seedOrgUsers()
	ctx := context.Background()

	if _, err := RunOnce(ctx, sc, client); err != nil {
		t.Fatalf("首轮同步: %v", err)
	}

	// zhangsan 从技术部调到产品部。
	m.Users[0].OrgID = "product"
	res, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("调岗同步: %v", err)
	}
	if res.MembersWritten != 1 || res.MembersDeleted != 1 {
		t.Fatalf("调岗摘要 = %+v, 期望写 1 删 1", res)
	}
	if got := tupleCount(t, sc, "org:tech", "member"); got != 1 {
		t.Errorf("调岗后 tech member = %d, 期望 1", got)
	}
	if got := tupleCount(t, sc, "org:product", "member"); got != 2 {
		t.Errorf("调岗后 product member = %d, 期望 2", got)
	}
}

// TestSyncOrgRemoval 组织撤销：无引用时删除元组与注册表行；仍被授权引用时
// 跳过删除（防悬空授权）。
func TestSyncOrgRemoval(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.seedOrgUsers()
	ctx := context.Background()

	if _, err := RunOnce(ctx, sc, client); err != nil {
		t.Fatalf("首轮同步: %v", err)
	}

	// 给产品部授一条权限（模拟管理员授权，同步器不可触碰）。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:product#member", Relation: "can", Object: "permission:demo.read"},
	}); err != nil {
		t.Fatalf("预置授权: %v", err)
	}

	// CAS 撤销产品部（连成员一起消失——成员随组织删除由删除分支清理）。
	m.Orgs = m.Orgs[:2]
	var kept []casdir.DirUser
	for _, u := range m.Users {
		if u.OrgID != "product" {
			kept = append(kept, u)
		}
	}
	m.Users = kept

	res, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("撤销同步: %v", err)
	}
	if res.OrgsSkipped != 1 || res.OrgsDeleted != 0 {
		t.Fatalf("撤销摘要 = %+v, 期望产品部因授权引用跳过删除", res)
	}
	// 授权元组与注册表行必须原样保留（边界 1：不碰授权；跳过删除）。
	if got := tupleCount(t, sc, "permission:demo.read", "can"); got != 1 {
		t.Errorf("授权元组 = %d, 期望保留 1", got)
	}
	if _, err := sc.Orgs.Get(ctx, "product"); err != nil {
		t.Errorf("跳过删除的组织注册表行不应消失: %v", err)
	}

	// 人工移除授权后，下一轮同步正常删除。
	if err := sc.Engine.DeleteTuples(ctx, []engine.Tuple{
		{User: "org:product#member", Relation: "can", Object: "permission:demo.read"},
	}); err != nil {
		t.Fatalf("移除授权: %v", err)
	}
	res2, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("二次撤销同步: %v", err)
	}
	if res2.OrgsDeleted != 1 {
		t.Fatalf("二次撤销摘要 = %+v, 期望删除 1 个组织", res2)
	}
	if _, err := sc.Orgs.Get(ctx, "product"); err == nil {
		t.Error("已撤销组织的注册表行应被删除")
	}
}

// TestSyncEngineOrphans 引擎孤儿清理：注册表无行（重启丢失）但引擎有元组的
// 组织，同步时被扫出并清理——元组 MySQL 持久而注册表内存期的历史遗留形态。
func TestSyncEngineOrphans(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.seedOrgUsers()
	ctx := context.Background()

	// 预置孤儿组织（不在 CAS、不在注册表）的引擎元组。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:root", Relation: "parent", Object: "org:org-legacy01"},
		{User: "user:zhangsan", Relation: "member", Object: "org:org-legacy01"},
	}); err != nil {
		t.Fatalf("预置孤儿元组: %v", err)
	}

	res, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("孤儿清理同步: %v", err)
	}
	if res.OrgsDeleted != 1 {
		t.Fatalf("摘要 = %+v, 期望孤儿组织被删除 1 个", res)
	}
	if got := tupleCount(t, sc, "org:org-legacy01", ""); got != 0 {
		t.Errorf("孤儿组织元组残留 %d 条, 期望全部清理", got)
	}

	// 二轮：孤儿已清，无新动作。
	res2, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("二轮同步: %v", err)
	}
	if res2.OrgsDeleted != 0 {
		t.Errorf("二轮 OrgsDeleted = %d, 期望 0", res2.OrgsDeleted)
	}
}

// TestSyncInheritanceDirection 继承方向（模型 v5 核心语义）：给部门授权 =
// 该部门子树内（直属 + 全部子孙部门）人员获得权限；授权叶子不影响父级/兄弟。
// 数据三层：root → tech → tech-sub（各挂一名成员）。
func TestSyncInheritanceDirection(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.Orgs = []casdir.Org{
		{ID: "root", Name: "nemo 平台"},
		{ID: "tech", Name: "技术部", ParentID: "root"},
		{ID: "tech-sub", Name: "前端组", ParentID: "tech"},
	}
	m.Users = []casdir.DirUser{
		{Username: "boss", OrgID: "tech"},
		{Username: "zhangsan", OrgID: "tech-sub"},
	}
	ctx := context.Background()
	if _, err := RunOnce(ctx, sc, client); err != nil {
		t.Fatalf("同步: %v", err)
	}

	// 给技术部授 demo.read：直属 boss 与子部门成员 zhangsan 都应获得。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:tech#member", Relation: "can", Object: "permission:demo.read"},
	}); err != nil {
		t.Fatalf("授权技术部: %v", err)
	}
	for _, u := range []string{"boss", "zhangsan"} {
		objs, err := sc.Engine.ListObjects(ctx, "user:"+u, "can", "permission")
		if err != nil {
			t.Fatalf("ListObjects(%s): %v", u, err)
		}
		if !containsStr(objs, "permission:demo.read") {
			t.Errorf("%s 应经技术部授权获得 demo.read, 实际 %v", u, objs)
		}
	}

	// 给叶子前端组授 demo.write：zhangsan 获得；boss（父级直属）不获得。
	if err := sc.Engine.WriteTuples(ctx, []engine.Tuple{
		{User: "org:tech-sub#member", Relation: "can", Object: "permission:demo.write"},
	}); err != nil {
		t.Fatalf("授权前端组: %v", err)
	}
	objs, err := sc.Engine.ListObjects(ctx, "user:zhangsan", "can", "permission")
	if err != nil || !containsStr(objs, "permission:demo.write") {
		t.Errorf("zhangsan 应获得前端组 demo.write, 实际 %v err=%v", objs, err)
	}
	bossObjs, err := sc.Engine.ListObjects(ctx, "user:boss", "can", "permission")
	if err != nil {
		t.Fatalf("ListObjects(boss): %v", err)
	}
	if containsStr(bossObjs, "permission:demo.write") {
		t.Errorf("boss 不应获得叶子前端组的 demo.write（授权叶子不影响父级）, 实际 %v", bossObjs)
	}
}

// containsStr 判断切片是否含目标串。
func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// TestSyncDanglingUser 悬空归属：用户 orgId 不在组织架构中时跳过并计数，
// 不写 member 元组，同步本身不失败。
func TestSyncDanglingUser(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.seedOrgUsers()
	m.Users = append(m.Users, casdir.DirUser{Username: "luxiaoyu", OrgID: "ghost-dept"})
	ctx := context.Background()

	res, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("悬空归属同步不应失败: %v", err)
	}
	if res.DanglingUsers != 1 {
		t.Errorf("DanglingUsers = %d, 期望 1", res.DanglingUsers)
	}
	if got := tupleCount(t, sc, "org:ghost-dept", "member"); got != 0 {
		t.Errorf("悬空归属不应产生 member 元组, 实际 %d", got)
	}
}

// TestSyncParentChange 换父场景：组织 parentId 变更 → 删旧 parent 元组 + 写新。
func TestSyncParentChange(t *testing.T) {
	sc := newTestSvcCtx(t)
	m, client := newMockCAS(t)
	m.seedOrgUsers()
	ctx := context.Background()

	if _, err := RunOnce(ctx, sc, client); err != nil {
		t.Fatalf("首轮同步: %v", err)
	}

	// 产品部改挂到技术部下：parent/child 互逆元组成对删旧写新。
	m.Orgs[2].ParentID = "tech"
	res, err := RunOnce(ctx, sc, client)
	if err != nil {
		t.Fatalf("换父同步: %v", err)
	}
	if res.ParentsWritten != 2 || res.ParentsDeleted != 2 {
		t.Fatalf("换父摘要 = %+v, 期望写 2 删 2（parent+child）", res)
	}
	if got := tupleCount(t, sc, "org:product", "parent"); got != 1 {
		t.Errorf("换父后 product parent 元组 = %d, 期望仍为 1", got)
	}
	parents, err := sc.Engine.ReadTuples(ctx, "org:product", "parent")
	if err != nil || len(parents) != 1 || parents[0].User != "org:tech" {
		t.Errorf("换父后 parent 指向 = %v, err=%v, 期望 org:tech", parents, err)
	}
	childs, err := sc.Engine.ReadTuples(ctx, "org:tech", "child")
	// child 元组挂在父对象上：tech 的孩子应恰为 product。
	if err != nil || len(childs) != 1 || childs[0].User != "org:product" {
		t.Errorf("换父后 tech 的 child 元组 = %v, err=%v, 期望 user=org:product", childs, err)
	}
	oldChilds, err := sc.Engine.ReadTuples(ctx, "org:root", "child")
	if err != nil {
		t.Fatalf("读 root child: %v", err)
	}
	for _, c := range oldChilds {
		if c.User == "org:product" {
			t.Errorf("换父后 root 不应再有 product 的 child 元组: %v", oldChilds)
		}
	}
}
