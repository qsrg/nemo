// authn_test.go 覆盖：TestToken 签发 → Identity 中间件注入 → IdentityFrom 取用
// 的完整链路，以及无 token / 坏 token / 无 roles / temp token 拒绝的边界。
package authz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v4"
)

// roundtrip 用 Identity 中间件跑一次请求，返回 next 里取到的会话。
func roundtrip(t *testing.T, secret, token string) (sess Session, err error) {
	t.Helper()

	// Session 经闭包变量带出 next（IdentityFrom 现返回 Session）。
	nextSess := Session{}
	next := func(w http.ResponseWriter, r *http.Request) {
		nextSess, err = IdentityFrom(r.Context())
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/authz/check", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	Identity(secret)(next)(rec, req)
	return nextSess, err
}

// TestIdentityRoundtrip 验证标准 claims（sub/roles）经中间件类型化注入并可取出。
func TestIdentityRoundtrip(t *testing.T) {
	const secret = "test-secret"
	token, err := TestToken(secret, "user:u1", []string{"super_admin", "ops"})
	if err != nil {
		t.Fatalf("TestToken: %v", err)
	}

	sess, err := roundtrip(t, secret, token)
	if err != nil {
		t.Fatalf("IdentityFrom: %v", err)
	}
	if sess.Sub != "user:u1" {
		t.Errorf("userID = %q, 期望 user:u1", sess.Sub)
	}
	if len(sess.Roles) != 2 || sess.Roles[0] != "super_admin" || sess.Roles[1] != "ops" {
		t.Errorf("roles = %v, 期望 [super_admin ops]", sess.Roles)
	}
}

// TestIdentityNoToken 无 Authorization 头时放行且 ctx 无身份（ErrNoIdentity）。
func TestIdentityNoToken(t *testing.T) {
	_, err := roundtrip(t, "test-secret", "")
	if !errors.Is(err, ErrNoIdentity) {
		t.Errorf("无 token 应返回 ErrNoIdentity, 实际 %v", err)
	}
}

// TestIdentityBadToken 用错误 secret 签发的 token：中间件解析失败放行，无身份。
func TestIdentityBadToken(t *testing.T) {
	token, err := TestToken("another-secret", "user:u1", []string{"super_admin"})
	if err != nil {
		t.Fatalf("TestToken: %v", err)
	}
	_, err = roundtrip(t, "test-secret", token)
	if !errors.Is(err, ErrNoIdentity) {
		t.Errorf("坏 token 应返回 ErrNoIdentity, 实际 %v", err)
	}
}

// TestIdentityNoRolesClaim token 不带 roles claim：身份可用，roles 为空。
func TestIdentityNoRolesClaim(t *testing.T) {
	const secret = "test-secret"
	token, err := TestToken(secret, "user:u1", nil)
	if err != nil {
		t.Fatalf("TestToken: %v", err)
	}
	sess, err := roundtrip(t, secret, token)
	if err != nil {
		t.Fatalf("IdentityFrom: %v", err)
	}
	if sess.Sub != "user:u1" {
		t.Errorf("userID = %q, 期望 user:u1", sess.Sub)
	}
	if len(sess.Roles) != 0 {
		t.Errorf("roles = %v, 期望空", sess.Roles)
	}
}

// TestIdentityTempTokenRejected typ=temp 的临时 token 签名有效但被直接 401 拒绝
//（临时 token 只服务于 CAS 回调链路取角色，不得访问任何业务接口）。
func TestIdentityTempTokenRejected(t *testing.T) {
	const secret = "test-secret"
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user:u1",
		"typ": "temp",
		"exp": int64(1_800_000_000_000),
	})
	token, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("签发 temp token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/authz/check", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	called := false
	Identity(secret)(func(w http.ResponseWriter, r *http.Request) { called = true })(rec, req)
	if called {
		t.Error("temp token 不应放行到 next")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("状态码 = %d, 期望 401", rec.Code)
	}
}

// TestIdentityFromEmptyCtx 空 ctx 直接取：返回 ErrNoIdentity。
func TestIdentityFromEmptyCtx(t *testing.T) {
	if _, err := IdentityFrom(context.Background()); !errors.Is(err, ErrNoIdentity) {
		t.Errorf("空 ctx 应返回 ErrNoIdentity, 实际 %v", err)
	}
}

// TestWithIdentity 覆盖 WithIdentity：注入后 IdentityFrom 取回同一身份，
// 供 logic 层单测构造带身份的 ctx（生产路径走 Identity 中间件）。
func TestWithIdentity(t *testing.T) {
	ctx := WithIdentity(context.Background(), "admin", []string{"super_admin"})
	sess, err := IdentityFrom(ctx)
	if err != nil {
		t.Fatalf("IdentityFrom: %v", err)
	}
	if sess.Sub != "admin" {
		t.Errorf("userID = %q, 期望 admin", sess.Sub)
	}
	if len(sess.Roles) != 1 || sess.Roles[0] != "super_admin" {
		t.Errorf("roles = %v, 期望 [super_admin]", sess.Roles)
	}
}
