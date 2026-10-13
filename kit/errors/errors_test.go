package errors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// TestConstructors 验证六个语义构造器的状态码与错误码一一对应。
func TestConstructors(t *testing.T) {
	cases := []struct {
		name   string
		got    *Error
		status int
		code   string
	}{
		{"BadRequest", BadRequest("TEST_BAD", "参数错误"), 400, "TEST_BAD"},
		{"Unauthorized", Unauthorized("TEST_NO_AUTH", "未登录"), 401, "TEST_NO_AUTH"},
		{"Forbidden", Forbidden("TEST_DENIED", "无权限"), 403, "TEST_DENIED"},
		{"NotFound", NotFound("TEST_NOT_FOUND", "不存在"), 404, "TEST_NOT_FOUND"},
		{"Conflict", Conflict("TEST_DUP", "重复"), 409, "TEST_DUP"},
		{"Internal", Internal("TEST_INTERNAL", "内部错误"), 500, "TEST_INTERNAL"},
	}
	for _, c := range cases {
		if c.got.Status != c.status || c.got.Code != c.code {
			t.Errorf("%s: got (%d,%s), want (%d,%s)", c.name, c.got.Status, c.got.Code, c.status, c.code)
		}
	}
}

// TestRequestIDContext 验证请求 id 的 context 写入/读取往返，以及缺失时返回空串。
func TestRequestIDContext(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-1")
	if RequestIDFrom(ctx) != "req-1" {
		t.Fatal("request id round-trip failed")
	}
	if RequestIDFrom(context.Background()) != "" {
		t.Fatal("empty ctx should return empty id")
	}
}

// TestErrorBody 验证 *Error 经 errorBody 输出统一的 JSON 错误体形状（含 request_id）。
func TestErrorBody(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-9")
	status, body := errorBody(ctx, NotFound("TEST_NOT_FOUND", "不存在"))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d", status)
	}
	raw, _ := json.Marshal(body)
	want := `{"error":{"code":"TEST_NOT_FOUND","message":"不存在","request_id":"req-9"}}`
	if string(raw) != want {
		t.Fatalf("body = %s, want %s", raw, want)
	}
}

// TestErrorBodyFallback 验证非 *Error 的未知错误统一归一为 500 INTERNAL，
// 且原始错误内容不进响应体（防内部细节泄漏）。
func TestErrorBodyFallback(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-8")
	status, body := errorBody(ctx, fmt.Errorf("db password is hunter2"))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d", status)
	}
	raw, _ := json.Marshal(body)
	want := `{"error":{"code":"INTERNAL","message":"服务内部错误","request_id":"req-8"}}`
	if string(raw) != want {
		t.Fatalf("body = %s, want %s", raw, want)
	}
	if strings.Contains(string(raw), "hunter2") {
		t.Fatal("internal error detail leaked into response body")
	}
}

// TestErrorBodyWrapping 验证 errorBody 经 errors.As 识别 %w 包装链中的
// *Error：fmt.Errorf 包装的 NotFound 仍按 404 与原始 code/message 输出。
func TestErrorBodyWrapping(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-5")
	wrapped := fmt.Errorf("wrap: %w", NotFound("TEST_X", "y"))
	status, body := errorBody(ctx, wrapped)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if body.Error.Code != "TEST_X" || body.Error.Message != "y" {
		t.Fatalf("code/message = (%s,%s), want (TEST_X,y)", body.Error.Code, body.Error.Message)
	}
}

// TestFronemorapping 验证 From 同样经 errors.As 识别 %w 包装链中的 *Error
// 并原样透传（跨服务调用传递上游错误码依赖此行为）。
func TestFronemorapping(t *testing.T) {
	wrapped := fmt.Errorf("wrap: %w", NotFound("TEST_X", "y"))
	got := From(wrapped)
	if got == nil || got.Status != http.StatusNotFound || got.Code != "TEST_X" {
		t.Fatalf("From(wrapped) = %+v, want 透传 TEST_X/404", got)
	}
}

// TestInstallErrorHandlerRendering 验证 InstallErrorHandler 注册后，
// go-zero 真实错误渲染路径（httpx.ErrorCtx，即 goctl 生成 handler 的出口）
// 输出统一 JSON 错误体：状态码、Content-Type 与请求 id 均正确。
func TestInstallErrorHandlerRendering(t *testing.T) {
	InstallErrorHandler()
	ctx := WithRequestID(context.Background(), "req-7")
	rec := httptest.NewRecorder()
	httpx.ErrorCtx(ctx, rec, NotFound("TEST_NOT_FOUND", "不存在"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q", ct)
	}
	got := strings.TrimSpace(rec.Body.String())
	want := `{"error":{"code":"TEST_NOT_FOUND","message":"不存在","request_id":"req-7"}}`
	if got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

// TestWriteJSONShape 验证 (*Error).WriteJSON 与全局错误出口（errorBody）输出同一形状。
func TestWriteJSONShape(t *testing.T) {
	e := NotFound("TEST_NOT_FOUND", "不存在")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/x", nil).
		WithContext(WithRequestID(context.Background(), "req-6"))
	e.WriteJSON(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	got := strings.TrimSpace(rec.Body.String())
	want := `{"error":{"code":"TEST_NOT_FOUND","message":"不存在","request_id":"req-6"}}`
	if got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}
