// bootstrap_test.go 覆盖幂等引导：连跑两次 Run 无 error（WriteTuples 读后写
// 幂等路径生效）、默认超管（配置未声明 → admin）与配置化超管（多用户/多组织）
// 的 Check 语义正确、Store/Model ID 已绑定供后续任务承接。
package bootstrap

import (
	"context"
	"testing"

	"nemo/services/authz/internal/engine"
)

// startTestEngine 启动 memory 后端测试引擎并注册 Cleanup，返回 Handle。
func startTestEngine(t *testing.T) *engine.Handle {
	t.Helper()

	h, err := engine.Start(context.Background(), engine.Options{
		HTTPAddr:  "127.0.0.1:0", // 随机端口，避免并行冲突
		StoreName: "bootstrap-test-store",
		Datastore: engine.DatastoreMemory,
	})
	if err != nil {
		t.Fatalf("启动引擎: %v", err)
	}
	t.Cleanup(h.Stop)
	return h
}

// runTwice 连续跑两次 Run（验证幂等），返回 handle。
func runTwice(t *testing.T, h *engine.Handle, opts Options) {
	t.Helper()
	if err := Run(context.Background(), h, opts); err != nil {
		t.Fatalf("Run 第一次: %v", err)
	}
	if err := Run(context.Background(), h, opts); err != nil {
		t.Fatalf("Run 第二次（幂等）: %v", err)
	}
}

// mustCheck 断言 Check(user, relation, object) 结果等于 want。
func mustCheck(t *testing.T, h *engine.Handle, user, relation, object string, want bool) {
	t.Helper()
	got, err := h.Check(context.Background(), user, relation, object)
	if err != nil {
		t.Fatalf("Check(%s, %s, %s): %v", user, relation, object, err)
	}
	if got != want {
		t.Errorf("Check(%s, %s, %s) = %v, want %v", user, relation, object, got, want)
	}
}

// TestRunDefaultAdminIdempotentAndSemantics 覆盖默认超管（配置未声明）：
// ①连续跑 Run 两次均无 error；②默认超管 admin 是根组织成员；
// ③admin 是超管；④无关用户既非根组织成员、也非超管（负样本）。
func TestRunDefaultAdminIdempotentAndSemantics(t *testing.T) {
	// 测试 cwd 是 internal/bootstrap/，模型文件在模块根 services/authz/。
	ModelFile = "../../model.fga"

	h := startTestEngine(t)

	// 首次引导（Options 零值：AdminUsers 空 → 默认 admin；AdminOrgs 空）。
	runTwice(t, h, Options{})

	// ② 根组织成员语义：默认超管 admin 是 org:root 的 member。
	mustCheck(t, h, "user:admin", "member", RootOrg, true)

	// ③ 超管全通配权限：admin 持有 permission:*.*（码 *:*）的 can——
	// me/permissions 经 ListObjects(can, permission) 枚举出 *:* 码，超管
	// 天然全权（规避"admin 无码进不了授权页"的引导死锁）。
	mustCheck(t, h, "user:admin", "can", AdminAllPerm, true)

	// ④ 负样本：无关用户不是根组织成员、不持全通配权限。
	mustCheck(t, h, "user:u2", "member", RootOrg, false)
	mustCheck(t, h, "user:u2", "can", AdminAllPerm, false)
}

// TestRunConfiguredAdmins 覆盖配置化超管（多用户 + 多组织）：
// ①AdminUsers 非空时按配置下发（不再含未声明用户）；
// ②每个配置用户授根组织成员 + 全通配权限；
// ③AdminOrgs 组织成员集合（org#member 主体）继承超管，组织内用户
// Check(can) 为 true，组织外用户为 false；
// ④重复执行仍幂等。
func TestRunConfiguredAdmins(t *testing.T) {
	ModelFile = "../../model.fga"

	h := startTestEngine(t)

	// 多用户 + 多组织声明：ops-admin/ops-root 两个用户，ops 组织成员集合为超管。
	runTwice(t, h, Options{
		AdminUsers: []string{"ops-admin", "root-ops"},
		AdminOrgs:  []string{"ops"},
	})

	// ② 配置用户：根组织成员 + 超管全权。
	for _, u := range []string{"ops-admin", "root-ops"} {
		mustCheck(t, h, "user:"+u, "member", RootOrg, true)
		mustCheck(t, h, "user:"+u, "can", AdminAllPerm, true)
	}

	// ③ 组织维度：ops 组织成员集合授超管。先写 member 元组再验证继承。
	memberTuples := []engine.Tuple{
		{User: "user:ops-member-a", Relation: "member", Object: "org:ops"},
		{User: "user:ops-member-b", Relation: "member", Object: "org:ops"},
	}
	if err := h.WriteTuples(context.Background(), memberTuples); err != nil {
		t.Fatalf("写组织成员元组: %v", err)
	}
	for _, u := range []string{"ops-member-a", "ops-member-b"} {
		mustCheck(t, h, "user:"+u, "can", AdminAllPerm, true) // 组织继承超管
	}
	// 组织外用户不继承。
	mustCheck(t, h, "user:outsider", "can", AdminAllPerm, false)

	// ① 未声明用户（admin）在配置化模式下不再自动成为超管（配置即真源）。
	mustCheck(t, h, "user:admin", "can", AdminAllPerm, false)
}

// TestRunWithRootOrgAsAdminOrg 覆盖 AdminOrgs 含根组织（root）的场景：
// root 组织成员集合被授超管，且不产生 org:root 自引用 member 元组。
func TestRunWithRootOrgAsAdminOrg(t *testing.T) {
	ModelFile = "../../model.fga"

	h := startTestEngine(t)

	runTwice(t, h, Options{AdminOrgs: []string{"root"}})

	// root 组织成员集合授超管：root 下用户（如默认 admin）Check(can) 为 true。
	mustCheck(t, h, "user:admin", "can", AdminAllPerm, true)
	// 根组织 self-member 自引用不存在（org:root 不能 member org:root）。
	mustCheck(t, h, "org:root", "member", RootOrg, false)
}
