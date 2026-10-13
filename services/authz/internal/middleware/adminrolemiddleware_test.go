// adminrolemiddleware_test.go 覆盖管理面鉴权（去角色化改造后的四态）：
// 超管元组放行 / 无元组 403 / 无身份 403（fail-closed）/ 引擎故障 503（fail-closed）。
// 链路按真实装配构造：authzkit.Identity 注入身份 → AdminRole 引擎实时 Check。
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
)

// newTestEngine 启动 memory 引擎并绑定 model.fga（相对本目录两级），写入
// 超管元组（bootstrap 同款）。测试结束经 Cleanup 停引擎。
func newTestEngine(t *testing.T, withAdmin bool) *engine.Handle {
	t.Helper()

	h, err := engine.Start(context.Background(), engine.Options{
		HTTPAddr: "127.0.0.1:0", StoreName: "adminrole-test", Datastore: engine.DatastoreMemory,
	})
	if err != nil {
		t.Fatalf("启动引擎: %v", err)
	}
	t.Cleanup(h.Stop)

	dsl, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}
	if _, _, err := h.EnsureModel(context.Background(), string(dsl)); err != nil {
		t.Fatalf("EnsureModel: %v", err)
	}
	if withAdmin {
		if err := h.WriteTuples(context.Background(), []engine.Tuple{
			{User: "user:admin", Relation: "can", Object: "permission:*.*"},
		}); err != nil {
			t.Fatalf("写超管权限元组: %v", err)
		}
	}
	return h
}

// runAdminRole 按 Identity → AdminRole 的链路跑一次带 token 的请求，返回 (状态码, 响应体)。
func runAdminRole(t *testing.T, h *engine.Handle, token string) (int, string) {
	t.Helper()

	errors.InstallErrorHandler()
	identity := authzkit.Identity("test-secret")
	adminRole := NewAdminRoleMiddleware(h).Handle

	next := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("passed"))
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/orgs", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	identity(adminRole(next))(rec, req)
	return rec.Code, rec.Body.String()
}

// TestAdminRoleAllowsAdmin 引擎中存在 permission:*.* can 元组（超管权限）
// 应放行——claims.roles 不再参与判定（token 带不带 roles 结果一致）。
func TestAdminRoleAllowsAdmin(t *testing.T) {
	h := newTestEngine(t, true)
	token, err := authzkit.TestToken("test-secret", "admin", nil)
	if err != nil {
		t.Fatalf("TestToken: %v", err)
	}
	code, body := runAdminRole(t, h, token)
	if code != http.StatusOK || body != "passed" {
		t.Errorf("超管应放行, 实际 code=%d body=%q", code, body)
	}
}

// TestAdminRoleRejectsNonAdmin 引擎无元组的普通用户（即使 token roles 带
// super_admin——去角色化后 claims.roles 不再被信任）应 403 AUTHZ_FORBIDDEN。
func TestAdminRoleRejectsNonAdmin(t *testing.T) {
	h := newTestEngine(t, true)
	token, err := authzkit.TestToken("test-secret", "u1", []string{"super_admin"})
	if err != nil {
		t.Fatalf("TestToken: %v", err)
	}
	code, body := runAdminRole(t, h, token)
	if code != http.StatusForbidden {
		t.Errorf("无元组用户应 403, 实际 code=%d body=%q", code, body)
	}
	if !strings.Contains(body, "AUTHZ_FORBIDDEN") {
		t.Errorf("错误码应为 AUTHZ_FORBIDDEN, 实际 body=%q", body)
	}
}

// TestAdminRoleRejectsNoIdentity 无身份（未带 token 的异常路径）应 403（fail-closed）。
func TestAdminRoleRejectsNoIdentity(t *testing.T) {
	h := newTestEngine(t, true)
	code, body := runAdminRole(t, h, "")
	if code != http.StatusForbidden {
		t.Errorf("无身份应 403, 实际 code=%d body=%q", code, body)
	}
	if !strings.Contains(body, "AUTHZ_FORBIDDEN") {
		t.Errorf("错误码应为 AUTHZ_FORBIDDEN, 实际 body=%q", body)
	}
}

// TestAdminRoleEngineFailure503 引擎故障（停掉引擎模拟）→ 503
// AUTHZ_SERVICE_UNAVAILABLE fail-closed——绝不把引擎不可达放行为管理员。
func TestAdminRoleEngineFailure503(t *testing.T) {
	h := newTestEngine(t, true)
	token, err := authzkit.TestToken("test-secret", "admin", []string{"super_admin"})
	if err != nil {
		t.Fatalf("TestToken: %v", err)
	}
	h.Stop() // 关掉引擎端口模拟引擎故障
	code, body := runAdminRole(t, h, token)
	if code != http.StatusServiceUnavailable {
		t.Errorf("引擎故障应 503, 实际 code=%d body=%q", code, body)
	}
	if !strings.Contains(body, "AUTHZ_SERVICE_UNAVAILABLE") {
		t.Errorf("错误码应为 AUTHZ_SERVICE_UNAVAILABLE, 实际 body=%q", body)
	}
}
