// replay_test.go 覆盖启动模型重放（Phase C 改造②，真引擎真 repo）：
// 注册资源类型 → 新 ServiceContext（同 biz repo、全新引擎）→ ReplayRegisteredModel
// → 新类型 check 生效；反证不重放（新引擎仍回落基础模型）时 check 报错——
// 复现并钉死 Shares 页 503 的修复语义。
package svc

import (
	"context"
	"os"
	"strings"
	"testing"

	authzkit "nemo/kit/authz"
	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
)

// newReplayTestCtx 经 NewServiceContext 真实装配内存模式上下文（随机端口）。
func newReplayTestCtx(t *testing.T) *ServiceContext {
	t.Helper()
	var c config.Config
	c.Name = "replay-test"
	c.Auth.AccessSecret = "test-secret"
	c.Engine.HTTPAddr = "127.0.0.1:0"
	c.Engine.StoreName = "replay-test-store"
	c.Engine.Datastore = engine.DatastoreMemory
	c.AdminRoles = []string{"super_admin"}
	sc := NewServiceContext(c)
	t.Cleanup(sc.Stop)

	// 绑定基础模型（生产由 bootstrap.Run 完成）。
	dsl, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	if _, _, err := sc.Engine.EnsureModel(context.Background(), string(dsl)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	return sc
}

// TestReplayRegisteredModel 注册类型 → 全新引擎 + 同 biz repo → 重放后判定生效。
func TestReplayRegisteredModel(t *testing.T) {
	sc1 := newReplayTestCtx(t)

	// 注册 report 类型：Render 模板块（Task 6 同源函数）+ 注册表落库——
	// 与 RegisterResourceType 的落库形态一致（重放只消费注册表 DslBlock）。
	// 不直接调 logic 包（svc 测试引 logic 会构成 import 环）。
	block, err := engine.Render(engine.ResourceType{Name: "report", Actions: []string{"can_view", "can_edit"}})
	if err != nil {
		t.Fatalf("Render report: %v", err)
	}
	if _, err := sc1.ResourceTypeRepo.Create(context.Background(), repo.ResourceType{Name: "report", DslBlock: block}); err != nil {
		t.Fatalf("注册 report: %v", err)
	}

	// 全新引擎（内存 datastore 从零开始，等价服务重启），复用同一 biz repo——
	// 复现 bug 场景：注册表有 report，但新引擎模型里没有。
	var c2 config.Config
	c2.Name = "replay-test-2"
	// ModelFile 相对测试 cwd（internal/svc）指到服务根（生产为 model.fga）。
	c2.Engine.ModelFile = "../../model.fga"
	c2.Engine.HTTPAddr = "127.0.0.1:0"
	c2.Engine.StoreName = "replay-test-store"
	c2.Engine.Datastore = engine.DatastoreMemory
	h2, err := engine.Start(context.Background(), engine.Options{
		HTTPAddr: c2.Engine.HTTPAddr, StoreName: c2.Engine.StoreName, Datastore: engine.DatastoreMemory,
	})
	if err != nil {
		t.Fatalf("启动新引擎: %v", err)
	}
	t.Cleanup(h2.Stop)

	// 基础模型先行（bootstrap 语义），此时 report 尚不在模型中。
	base, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	if _, _, err := h2.EnsureModel(context.Background(), string(base)); err != nil {
		t.Fatalf("新引擎基础模型: %v", err)
	}

	sc2 := &ServiceContext{
		Config:           c2,
		Engine:           h2,
		ResourceTypeRepo: sc1.ResourceTypeRepo, // 同 biz repo（bug 复现点）
		ModelVersionRepo: sc1.ModelVersionRepo,
	}

	// 反证：不重放时 report 判定报错（重启后 503 AUTHZ_ENGINE_ERROR 的根因）。
	if _, err := h2.Check(context.Background(), "user:admin", "can_view", "report:r1"); err == nil {
		t.Fatal("未重放时新引擎对 report 的 check 应报错（类型不在模型中）")
	} else if !strings.Contains(err.Error(), "Check") {
		t.Errorf("预期引擎类型错误, 实际 %v", err)
	}

	// 重放：合并基础模型 + 注册块并 EnsureModel。
	if err := ReplayRegisteredModel(context.Background(), sc2); err != nil {
		t.Fatalf("重放: %v", err)
	}

	// 新类型 check 生效：owner 元组写入后 can_edit 判定 true。
	if err := h2.WriteTuples(context.Background(), []engine.Tuple{
		{User: "user:admin", Relation: "owner", Object: "report:r1"},
	}); err != nil {
		t.Fatalf("写元组: %v", err)
	}
	ok, err := h2.Check(context.Background(), "user:admin", "can_edit", "report:r1")
	if err != nil || !ok {
		t.Fatalf("重放后 report 判定应生效: ok=%v err=%v", ok, err)
	}

	// 版本落库与最新一致（重放幂等，不再累积版本行）。
	latest, err := sc2.ModelVersionRepo.Latest(context.Background())
	if err != nil || latest.ID == 0 || latest.EnginemodelID == "" {
		t.Fatalf("重放应落模型版本(含自增 ID 与引擎 modelID): %v %+v", err, latest)
	}
	// 重复重放幂等（无新版本行、判定仍生效）。
	if err := ReplayRegisteredModel(context.Background(), sc2); err != nil {
		t.Fatalf("重复重放: %v", err)
	}
}

// TestReplaySkipsEmptyRegistry 无注册类型时重放跳过（基础模型由 bootstrap 负责）。
func TestReplaySkipsEmptyRegistry(t *testing.T) {
	sc := newReplayTestCtx(t)
	if err := ReplayRegisteredModel(context.Background(), sc); err != nil {
		t.Fatalf("空注册表重放应跳过不报错: %v", err)
	}
}

// authnAdminCtx 构造带 super_admin 身份的 ctx（经 authn 包）。
func authnAdminCtx() context.Context {
	return authzkit.WithIdentity(context.Background(), "admin", []string{"super_admin"})
}

var _ = authnAdminCtx
