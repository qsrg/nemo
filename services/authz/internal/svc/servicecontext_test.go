// servicecontext_test.go 覆盖 svc 装配：引擎进程内启动可 Ping、
// 四个业务 repo 骨架期为 nil、ReadyHandler 探针随引擎状态翻转。
package svc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nemo/services/authz/internal/config"
	"nemo/services/authz/internal/engine"
)

// newTestConfig 构造测试用 config：随机端口、memory 引擎、默认管理员角色。
func newTestConfig() config.Config {
	var c config.Config
	c.Name = "authz-api"
	c.Port = 0
	c.Auth.AccessSecret = "test-secret"
	c.Engine.HTTPAddr = "127.0.0.1:0" // 随机端口，避免并行冲突
	c.Engine.StoreName = "authz-test-store"
	c.Engine.Datastore = engine.DatastoreMemory
	c.AdminRoles = []string{"super_admin"}
	return c
}

// TestNewServiceContextStartsEngine 装配后引擎可用、Dsn 为空时四个业务
// repo 装配为内存实现（Task 4 起）、ReadyHandler 探针随引擎状态翻转。
func TestNewServiceContextStartsEngine(t *testing.T) {
	sc := NewServiceContext(newTestConfig())
	defer sc.Stop()

	if sc.Engine == nil {
		t.Fatalf("Engine 未装配")
	}
	if err := sc.Engine.Ping(); err != nil {
		t.Errorf("引擎启动后 Ping 应成功: %v", err)
	}
	// Dsn 为空 = 内存模式：四个业务 repo 均应装配为内存实现（非 nil）。
	if sc.RuleRepo == nil || sc.ResourceTypeRepo == nil ||
		sc.ModelVersionRepo == nil || sc.AuditRepo == nil {
		t.Errorf("内存模式下业务 repo 应全部装配非 nil")
	}
	// 组织元数据注册表应装配非 nil（Task 5 起管理 API 使用）。
	if sc.Orgs == nil {
		t.Errorf("组织注册表 Orgs 应装配非 nil")
	}
	if sc.AdminRole == nil {
		t.Errorf("AdminRole 中间件未装配")
	}
}

// TestNewServiceContextMySQLFastFail Dsn 非空但连不上时启动快速失败：
// panic 前先停掉已启动的引擎（端口不泄漏）。
func TestNewServiceContextMySQLFastFail(t *testing.T) {
	c := newTestConfig()
	// 本机回环保留端口，连接必然被拒绝且快速失败，不产生跨网络请求。
	c.Mysql.Dsn = "u:p@tcp(127.0.0.1:1)/nemo_authz_biz?charset=utf8mb4"
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MySQL 不可达时 NewServiceContext 应 panic")
		}
	}()
	_ = NewServiceContext(c)
}

// TestReadyHandler 探针语义：引擎在 200；Stop 后 Ping 失败 503。
func TestReadyHandler(t *testing.T) {
	sc := NewServiceContext(newTestConfig())
	defer sc.Stop()

	rec := httptest.NewRecorder()
	sc.ReadyHandler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("引擎在线 readyz 应 200, 实际 %d body=%q", rec.Code, rec.Body.String())
	}

	sc.Stop()
	rec2 := httptest.NewRecorder()
	sc.ReadyHandler(rec2, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec2.Code != http.StatusServiceUnavailable {
		t.Errorf("引擎停止后 readyz 应 503, 实际 %d body=%q", rec2.Code, rec2.Body.String())
	}
}
