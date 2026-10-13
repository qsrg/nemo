// testutil_test.go 是 logic 层测试的共享装配:
// 直启真 memory 引擎(经 svc.NewServiceContext 真实装配路径)+ 内存 repo,
// EnsureModel 绑定 model.fga(生产由 bootstrap 完成),并提供建组织/查审计的
// 公共小助手——三域测试共用,避免重复脚手架。
package logic

import (
	"context"
	"os"
	"strings"
	"testing"

	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	authzkit "nemo/kit/authz"
)

// newTestSvcCtx 经 svc.NewServiceContext 真实装配测试上下文:
// 随机端口 memory 引擎 + 内存 repo + 组织注册表,并注册 Cleanup 停引擎。
func newTestSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Name = "authz-api"
	c.Port = 0
	c.Auth.AccessSecret = "test-secret"
	c.Engine.HTTPAddr = "127.0.0.1:0" // 随机端口，避免并行冲突
	c.Engine.StoreName = "logic-test-store"
	c.Engine.Datastore = engine.DatastoreMemory
	c.Engine.ModelFile = "../../model.fga" // 注册/合并读取基础模型（cwd 为 internal/logic）
	c.AdminRoles = []string{"super_admin"}

	sc := svc.NewServiceContext(c)
	t.Cleanup(sc.Stop)

	// 绑定授权模型(生产由 bootstrap.Run 完成);测试文件位于 internal/logic/。
	dsl, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	if _, _, err := sc.Engine.EnsureModel(context.Background(), string(dsl)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}

	// 预置 admin 的 kafka Topic 演示域权限码（菜单级授权改造：写接口与
	// 列表徽标按 kafka-topics:manage 判定；read 供菜单可见性）。与生产
	// admin 种子授权同形态（permission:{code} can user:admin）。
	if err := sc.Engine.WriteTuples(context.Background(), []engine.Tuple{
		{User: "user:admin", Relation: "can", Object: "permission:kafka-topics.read"},
		{User: "user:admin", Relation: "can", Object: "permission:kafka-topics.manage"},
	}); err != nil {
		t.Fatalf("预置 admin kafka-topics 权限码: %v", err)
	}
	return sc
}

// adminCtx 构造带 super_admin 身份的 ctx(生产由 authzkit.Identity 中间件注入)。
func adminCtx() context.Context {
	return authzkit.WithIdentity(context.Background(), "admin", []string{"super_admin"})
}

// registerTypeViaMenus 经菜单派生路径注册资源类型（替代原独立注册 logic，
// 与生产形态一致：类型 = 资源驱动菜单/资源级按钮声明的归类）：
//   - 菜单行（resourceType=<name>）隐含 can_view（页面入口 = 类型列表可见性）；
//   - 其余动作各建一个资源级按钮（relation=动作）；
//   - manageCode 非空时先预置注册表存量（派生 sync 保留存量管理码），
//     模拟历史注册的类型级名单策略。
//
// 注意：会写 create_menu 审计（页 + 每动作各一条），需断言审计条数的测试勿用。
func registerTypeViaMenus(t *testing.T, sc *svc.ServiceContext, name string, actions []string, manageCode string) {
	t.Helper()
	ctx := adminCtx()
	slug := strings.ReplaceAll(name, "_", "-")

	if manageCode != "" {
		block, rerr := engine.Render(engine.ResourceType{Name: name, Actions: actions})
		if rerr != nil {
			t.Fatalf("渲染预置块 %s: %v", name, rerr)
		}
		if _, cerr := sc.ResourceTypeRepo.Create(ctx, repo.ResourceType{
			Name: name, DslBlock: block, ManageCode: manageCode, CreateBy: "test", UpdateBy: "test",
		}); cerr != nil {
			t.Fatalf("预置存量管理码 %s: %v", name, cerr)
		}
	}

	page, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
		Type:         "menu",
		Title:        name + " 页",
		Path:         "/" + slug,
		ResourceType: name,
	})
	if err != nil {
		t.Fatalf("创建资源驱动菜单 %s: %v", name, err)
	}
	for _, a := range actions {
		if a == "can_view" {
			continue // 菜单行隐含
		}
		act := strings.TrimPrefix(a, "can_")
		if _, err := NewCreateMenuLogic(ctx, sc).CreateMenu(&types.CreateMenuReq{
			Type:           "button",
			ParentId:       page.Id,
			Title:          name + " " + act,
			PermissionCode: slug + ":" + act,
			ResourceType:   name,
			Relation:       a,
		}); err != nil {
			t.Fatalf("创建资源级按钮 %s:%s: %v", name, act, err)
		}
	}
}

// registerKafkaTopic 在测试上下文注册 kafka_topic 资源类型（2026-09-16 起
// topic 迁注册表，不再内置在 model.fga）：kafka 域测试（topiclogic/
// abaclogic/resourcerelations）的对象元组写 kafka_topic:{id}，须先注册类型
// 引擎才认识。经菜单派生路径（资源驱动菜单 + 资源级按钮 + sync），与生产
// 形态一致。注意：会写 create_menu 审计，需断言审计条数的测试勿用。
func registerKafkaTopic(t *testing.T, sc *svc.ServiceContext) {
	t.Helper()
	// 对齐生产注册表：manageCode = kafka-topics:manage（2026-10-11 收敛专用
	// 分支后由注册表 ManageCode 声明驱动，管理码持有者名单全权）。
	registerTypeViaMenus(t, sc, "kafka_topic", []string{"can_view", "can_edit", "can_delete"}, "kafka-topics:manage")
}

// createOrgViaLogic 经 CreateOrg logic 建组织并返回其 ID(测试脚手架)。
func createOrgViaLogic(t *testing.T, sc *svc.ServiceContext, name, parentID string) string {
	t.Helper()

	resp, err := NewCreateOrgLogic(adminCtx(), sc).CreateOrg(&types.CreateOrgReq{
		Name:     name,
		ParentId: parentID,
	})
	if err != nil {
		t.Fatalf("建组织 %q(parent=%q): %v", name, parentID, err)
	}
	return resp.Id
}

// auditOf 按动作取一条审计条目,不存在则 t.Fatalf。
func auditOf(t *testing.T, sc *svc.ServiceContext, action string) repo.AuditEntry {
	t.Helper()

	entries, _, err := sc.AuditRepo.Query(context.Background(), repo.AuditFilter{})
	if err != nil {
		t.Fatalf("查询审计: %v", err)
	}
	for _, e := range entries {
		if e.Action == action {
			return e
		}
	}
	t.Fatalf("未找到动作 %q 的审计条目(共 %d 条)", action, len(entries))
	return repo.AuditEntry{}
}

// auditCount 返回审计条目总数。
func auditCount(t *testing.T, sc *svc.ServiceContext) int {
	t.Helper()

	entries, total, err := sc.AuditRepo.Query(context.Background(), repo.AuditFilter{})
	if err != nil {
		t.Fatalf("查询审计: %v", err)
	}
	_ = entries
	return total
}
