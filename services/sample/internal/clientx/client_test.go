package clientx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kiterrors "nemo/kit/errors"
)

// outThing 是测试用出参模型：clientx 与具体业务类型解耦（any 出入参）。
type outThing struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// TestDoDecodes2xx 验证 2xx 正常解码出参。
func TestDoDecodes2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"id":"it-1","name":"a"}`))
	}))
	defer srv.Close()

	var out outThing
	err := New(srv.URL).Do(context.Background(), http.MethodGet, "/v1/things/it-1", nil, &out)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if out.Id != "it-1" || out.Name != "a" {
		t.Fatalf("want decoded body, got %+v", out)
	}
}

// TestDoParsesErrorBody 验证非 2xx：上游统一错误体经 kit/errors.Decode 原样带回
// （code/message 透传），Status 由 HTTP 状态码补齐。
func TestDoParsesErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在").WriteJSON(w, r)
	}))
	defer srv.Close()

	err := New(srv.URL).Do(context.Background(), http.MethodGet, "/v1/things/missing", nil, nil)
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) {
		t.Fatalf("want *kiterrors.Error, got %v", err)
	}
	if kitErr.Code != "SAMPLE_NOT_FOUND" || kitErr.Message != "条目不存在" {
		t.Fatalf("want upstream code/message 透传, got %+v", kitErr)
	}
	if kitErr.Status != http.StatusNotFound {
		t.Fatalf("want status 404, got %d", kitErr.Status)
	}
}

// TestDoFallbackOnNonContractBody 验证非约定错误体兜底：不外泄上游内部细节。
func TestDoFallbackOnNonContractBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret stack trace", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := New(srv.URL).Do(context.Background(), http.MethodGet, "/v1/things", nil, nil)
	var kitErr *kiterrors.Error
	if !errors.As(err, &kitErr) {
		t.Fatalf("want *kiterrors.Error, got %v", err)
	}
	if kitErr.Status != http.StatusInternalServerError || kitErr.Code != "INTERNAL" {
		t.Fatalf("want 500 INTERNAL fallback, got %+v", kitErr)
	}
	if strings.Contains(kitErr.Message, "secret") {
		t.Fatalf("upstream detail leaked: %s", kitErr.Message)
	}
}

// TestDoInjectsHeaders 验证请求头注入：X-Request-ID 取自 ctx、Bearer 取自
// WithToken、JSON 请求体带 Content-Type；ctx 无值时对应头不注入。
func TestDoInjectsHeaders(t *testing.T) {
	var got struct{ reqID, auth, contentType, body string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.reqID = r.Header.Get("X-Request-ID")
		got.auth = r.Header.Get("Authorization")
		got.contentType = r.Header.Get("Content-Type")
		got.body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := kiterrors.WithRequestID(context.Background(), "req-1")
	ctx = WithToken(ctx, "tok-1")
	if err := New(srv.URL).Do(ctx, http.MethodPost, "/v1/things", outThing{Id: "it-1", Name: "a"}, nil); err != nil {
		t.Fatalf("do with headers: %v", err)
	}
	if got.reqID != "req-1" {
		t.Fatalf("want X-Request-ID req-1, got %q", got.reqID)
	}
	if got.auth != "Bearer tok-1" {
		t.Fatalf("want Authorization Bearer tok-1, got %q", got.auth)
	}
	if !strings.HasPrefix(got.contentType, "application/json") {
		t.Fatalf("want json content type, got %q", got.contentType)
	}
	if !strings.Contains(got.body, `"name":"a"`) {
		t.Fatalf("want marshaled body, got %q", got.body)
	}

	// ctx 无 request id / token 时对应头为空（不注入空值）。
	if err := New(srv.URL).Do(context.Background(), http.MethodPost, "/v1/things", outThing{Name: "b"}, nil); err != nil {
		t.Fatalf("do without headers: %v", err)
	}
	if got.reqID != "" || got.auth != "" {
		t.Fatalf("headers must be absent without ctx values, got reqID=%q auth=%q", got.reqID, got.auth)
	}
}

// TestDo4xxDoesNotTripBreaker 验证熔断约定：4xx 是调用方可预期的业务结果，
// 不计熔断失败——连续 50 次全量 404 后调用仍应正常送达（若误计失败，
// 熔断器打开后错误将变为 ErrServiceUnavailable 而非上游错误体）。
func TestDo4xxDoesNotTripBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在").WriteJSON(w, r)
	}))
	defer srv.Close()

	for i := 0; i < 50; i++ {
		err := New(srv.URL).Do(context.Background(), http.MethodGet, "/v1/things/missing", nil, nil)
		var kitErr *kiterrors.Error
		if !errors.As(err, &kitErr) || kitErr.Code != "SAMPLE_NOT_FOUND" {
			t.Fatalf("call %d: want SAMPLE_NOT_FOUND, got %v", i, err)
		}
	}
}

// TestDo2xxDoesNotTripBreaker 是 4xx 熔断用例的镜像（成功侧）：go-zero breaker
// 对成功调用也回调 acceptable，nil 必须计为成功——连续 50 次成功 2xx 调用
// 全程不被自我熔断。缺 nil 分支时 accepts 恒 0、成功全被计为失败，健康上游
// 超过保护阈值（protection=5）后 breaker 打开，调用开始报 ErrServiceUnavailable。
func TestDo2xxDoesNotTripBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"it-1","name":"a"}`))
	}))
	defer srv.Close()

	for i := 0; i < 50; i++ {
		var out outThing
		err := New(srv.URL).Do(context.Background(), http.MethodGet, "/v1/things/it-1", nil, &out)
		if err != nil {
			t.Fatalf("call %d: want success, got %v", i, err)
		}
		if out.Id != "it-1" || out.Name != "a" {
			t.Fatalf("call %d: want decoded body, got %+v", i, out)
		}
	}
}
