package logging

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
)

// TestRequestIDGenerated 验证无入站 id 时生成新 id：写 context、回写响应头。
func TestRequestIDGenerated(t *testing.T) {
	var got string
	h := RequestID(func(w http.ResponseWriter, r *http.Request) {
		got = errors.RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	if got == "" {
		t.Fatal("expected generated request id in context")
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header")
	}
}

// TestRequestIDPassedThrough 验证入站 X-Request-ID 优先透传，不重新生成。
func TestRequestIDPassedThrough(t *testing.T) {
	var got string
	h := RequestID(func(w http.ResponseWriter, r *http.Request) {
		got = errors.RequestIDFrom(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
	req.Header.Set("X-Request-ID", "req-fixed")
	h(httptest.NewRecorder(), req)
	if got != "req-fixed" {
		t.Fatalf("got %q, want req-fixed", got)
	}
}

// TestAccessLog 用注入的内存 writer 捕获真实日志输出，
// 断言统一字段齐全（method/path/status/duration_ms/request_id）。
func TestAccessLog(t *testing.T) {
	var buf strings.Builder
	logx.SetWriter(logx.NewWriter(&buf))
	defer logx.Reset()

	h := RequestID(AccessLog(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/items", nil))

	out := buf.String()
	for _, key := range []string{`"method":"POST"`, `"path":"/v1/items"`, `"status":418`, `"duration_ms":`, `"request_id":"`} {
		if !strings.Contains(out, key) {
			t.Fatalf("access log missing %s: %s", key, out)
		}
	}
}

// TestInitConsoleSmoke 冒烟：console 模式初始化不报错（文件模式轮转属 logx 职责）。
func TestInitConsoleSmoke(t *testing.T) {
	Init("test-svc", false)
}
