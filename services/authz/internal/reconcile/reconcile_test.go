// reconcile_test.go 覆盖审计对账任务的核心语义:扫超时 pending → 回查
// 引擎/业务表实态 → committed/failed 修正。采用"真引擎 + 受控审计数据"
// 注入(直启 memory 引擎,手工 CreatePending 构造各动作的 pending 条目,
// 引擎/业务表实态按用例摆布),olderThan 传负值使全量 pending 入扫。
package reconcile

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// newTestCtx 经 svc.NewServiceContext 真实装配测试上下文:随机端口 memory
// 引擎 + 内存 repo,EnsureModel 绑定 model.fga,并注册 Cleanup 停引擎。
func newTestCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Name = "authz-api"
	c.Port = 0
	c.Auth.AccessSecret = "test-secret"
	c.Engine.HTTPAddr = "127.0.0.1:0"
	c.Engine.StoreName = "reconcile-test-store"
	c.Engine.Datastore = engine.DatastoreMemory
	c.AdminRoles = []string{"super_admin"}

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

// addPending 手工落一条 pending 审计(对账输入),返回条目 ID（int64 自增）。
func addPending(t *testing.T, sc *svc.ServiceContext, action, target, snapshot string) int64 {
	t.Helper()
	id, err := sc.AuditRepo.CreatePending(context.Background(), repo.AuditEntry{
		Operator: "admin",
		Action:   action,
		Target:   target,
		Snapshot: snapshot,
	})
	if err != nil {
		t.Fatalf("CreatePending(%s): %v", action, err)
	}
	return id
}

// statusOf 取指定条目当前状态。
func statusOf(t *testing.T, sc *svc.ServiceContext, id int64) repo.AuditStatus {
	t.Helper()
	entries, _, err := sc.AuditRepo.Query(context.Background(), repo.AuditFilter{})
	if err != nil {
		t.Fatalf("Query 审计: %v", err)
	}
	for _, e := range entries {
		if e.ID == id {
			return e.Status
		}
	}
	t.Fatalf("未找到审计条目 %d", id)
	return ""
}

// TestReconcileTupleProbes 引擎元组类动作的修正矩阵:实态已生效 →
// committed;未生效 → failed(附对账原因)。覆盖正向(create_org/
// add_member/assign_role)与反向(delete_org/remove_member/revoke_role)。
func TestReconcileTupleProbes(t *testing.T) {
	sc := newTestCtx(t)
	ctx := context.Background()

	// 实态:org:o1 已建、成员 u1 在 org:o1、u2 在 org:o2(尚未移除)。
	tuples := []engine.Tuple{
		{User: "user:u1", Relation: "member", Object: "org:o1"},
		{User: "user:u2", Relation: "member", Object: "org:o2"},
		{User: "user:u3", Relation: "assignee", Object: "platform_role:operator"},
	}
	if err := sc.Engine.WriteTuples(ctx, tuples); err != nil {
		t.Fatalf("摆布引擎实态: %v", err)
	}

	cases := []struct {
		name   string
		action string
		target string
		snap   string
		want   repo.AuditStatus
	}{
		{"create_org 已生效", "create_org", "org:o1", `{}`, repo.AuditCommitted},
		{"create_org 未生效", "create_org", "org:oX", `{}`, repo.AuditFailed},
		{"delete_org 已生效", "delete_org", "org:oGone", `{}`, repo.AuditCommitted},
		{"delete_org 未生效", "delete_org", "org:o2", `{}`, repo.AuditFailed},
		{"add_member 已生效", "add_member", "org:o1", `{"orgId":"o1","userId":"u1"}`, repo.AuditCommitted},
		{"add_member 未生效", "add_member", "org:o1", `{"orgId":"o1","userId":"u9"}`, repo.AuditFailed},
		{"remove_member 已生效", "remove_member", "org:o1", `{"orgId":"o1","userId":"u9"}`, repo.AuditCommitted},
		{"remove_member 未生效", "remove_member", "org:o1", `{"orgId":"o1","userId":"u1"}`, repo.AuditFailed},
		{"assign_role 已生效", "assign_role", "platform_role:operator", `{"roleKey":"operator","userId":"u3"}`, repo.AuditCommitted},
		{"assign_role 未生效", "assign_role", "platform_role:operator", `{"roleKey":"operator","userId":"u9"}`, repo.AuditFailed},
		{"revoke_role 已生效", "revoke_role", "platform_role:operator", `{"roleKey":"operator","userId":"u9"}`, repo.AuditCommitted},
		{"revoke_role 未生效", "revoke_role", "platform_role:operator", `{"roleKey":"operator","userId":"u3"}`, repo.AuditFailed},
	}

	for _, tc := range cases {
		id := addPending(t, sc, tc.action, tc.target, tc.snap)
		if _, err := RunOnce(ctx, sc, -time.Second); err != nil { // 负值 = 全量 pending 入扫
			t.Fatalf("%s: RunOnce: %v", tc.name, err)
		}
		if got := statusOf(t, sc, id); got != tc.want {
			t.Errorf("%s: 修正后状态 = %q, 期望 %q", tc.name, got, tc.want)
		}
	}
}

// TestReconcileBizTableProbes 业务表类动作的修正:
//   - upsert_rule：id 改自增后新建在审计落库时拿不到 ID，按快照三元组
//     （roleKey+prefix+action）判存在性；
//   - delete_rule：按 target rule:<数字id> 判存在性（反向 = 不存在为生效）；
//   - register_resource_type：按 target type:<名> 判注册表。
func TestReconcileBizTableProbes(t *testing.T) {
	sc := newTestCtx(t)
	ctx := context.Background()

	// 摆布一条已落库规则（自增 ID 由 repo 生成），供三元组与删除判据复用。
	saved, err := sc.RuleRepo.Upsert(ctx, repo.Rule{RoleKey: "operator", Prefix: "order:"})
	if err != nil {
		t.Fatalf("摆布规则实态: %v", err)
	}
	if _, err := sc.ResourceTypeRepo.Create(ctx, repo.ResourceType{Name: "report"}); err != nil {
		t.Fatalf("摆布资源类型实态: %v", err)
	}

	cases := []struct {
		name   string
		action string
		target string
		snap   string
		want   repo.AuditStatus
	}{
		{"upsert_rule 已生效(三元组)", "upsert_rule", "rule:operator|order:|", `{"roleKey":"operator","prefix":"order:","action":""}`, repo.AuditCommitted},
		{"upsert_rule 未生效(三元组)", "upsert_rule", "rule:ghost|x:|", `{"roleKey":"ghost","prefix":"x:","action":""}`, repo.AuditFailed},
		{"delete_rule 已生效(规则已不存在)", "delete_rule", "rule:999999", `{}`, repo.AuditCommitted},
		{"delete_rule 未生效(规则仍在)", "delete_rule", "rule:" + strconv.FormatInt(saved.ID, 10), `{}`, repo.AuditFailed},
		{"register_resource_type 已生效", "register_resource_type", "type:report", `{}`, repo.AuditCommitted},
		{"register_resource_type 未生效", "register_resource_type", "type:ghost", `{}`, repo.AuditFailed},
	}
	for _, tc := range cases {
		id := addPending(t, sc, tc.action, tc.target, tc.snap)
		if _, err := RunOnce(ctx, sc, -time.Second); err != nil {
			t.Fatalf("%s: RunOnce: %v", tc.name, err)
		}
		if got := statusOf(t, sc, id); got != tc.want {
			t.Errorf("%s: 修正后状态 = %q, 期望 %q", tc.name, got, tc.want)
		}
	}
}

// TestReconcileSkipsUnknownAction 未纳入对账语义表的动作(如 Task 7 将
// 实现的 write_relations)保持 pending 不动,计入 Skipped。
func TestReconcileSkipsUnknownAction(t *testing.T) {
	sc := newTestCtx(t)
	ctx := context.Background()

	id := addPending(t, sc, "write_relations", "report:r1", `{}`)
	res, err := RunOnce(ctx, sc, -time.Second)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, 期望 1", res.Skipped)
	}
	if got := statusOf(t, sc, id); got != repo.AuditPending {
		t.Errorf("未知动作应保持 pending, 实际 %q", got)
	}
}

// TestReconcileRespectsOlderThanAndStatus 未超时的 pending 与已 committed
// 的条目不参与修正(olderThan 传正值,受控条目均为新建未超时)。
func TestReconcileRespectsOlderThanAndStatus(t *testing.T) {
	sc := newTestCtx(t)
	ctx := context.Background()

	pendingID := addPending(t, sc, "create_org", "org:o1", `{}`)
	committedID, err := sc.AuditRepo.CreatePending(ctx, repo.AuditEntry{
		Operator: "admin", Action: "create_org", Target: "org:o1", Snapshot: `{}`,
	})
	if err != nil {
		t.Fatalf("CreatePending: %v", err)
	}
	if err := sc.AuditRepo.MarkCommitted(ctx, committedID); err != nil {
		t.Fatalf("MarkCommitted: %v", err)
	}

	res, err := RunOnce(ctx, sc, 5*time.Minute)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Committed+res.Failed+res.Skipped != 0 {
		t.Errorf("无超时 pending 时不应有修正: %+v", res)
	}
	if got := statusOf(t, sc, pendingID); got != repo.AuditPending {
		t.Errorf("未超时 pending 应保持 pending, 实际 %q", got)
	}
	if got := statusOf(t, sc, committedID); got != repo.AuditCommitted {
		t.Errorf("committed 条目不应被动, 实际 %q", got)
	}
}
