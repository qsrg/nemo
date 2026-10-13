// authz_test.go 覆盖 kit/authz 三能力：
//  1. Checker 接口 + CheckObject（对象标识拼装语义）；
//  2. Remote（对 httptest 对端：200/无权/上游 5xx/超时/身份与 request id 透传/熔断镜像）；
//  3. Require 中间件（fake Checker 注入：放行/403 AUTHZ_DENIED/503 AUTHZ_SERVICE_UNAVAILABLE、
//     perm 解析、路径取对象 ID、JWT 与 request id 进 ctx）。
package authz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kiterrors "nemo/kit/errors"
)

// fakeChecker 是 Checker 的测试替身：记录最近一次入参，按配置返回结果。
type fakeChecker struct {
	gotObject   string
	gotRelation string
	gotToken    string
	gotReqID    string
	allowed     bool
	err         error
	calls       int
}

// Check 记录入参并返回预设结果；同时捕获 ctx 内的身份与 request id。
func (f *fakeChecker) Check(ctx context.Context, object, relation string) (bool, error) {
	f.calls++
	f.gotObject, f.gotRelation = object, relation
	f.gotToken = tokenFrom(ctx)
	f.gotReqID = kiterrors.RequestIDFrom(ctx)
	return f.allowed, f.err
}

// ListObjects 测试替身不使用。
func (f *fakeChecker) ListObjects(context.Context, string, string) ([]string, error) {
	return nil, nil
}

// BatchCheck 测试替身不使用。
func (f *fakeChecker) BatchCheck(context.Context, []BatchCheckItem) ([]bool, error) {
	return nil, nil
}

// newRequireReq 构造带对象 ID 的测试请求（perm 解析自路径末段）。
func newRequireReq(path string, authHeader string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		r.Header.Set("Authorization", authHeader)
	}
	return r
}

// errBody 从响应体解析统一错误体（kit/authz 输出均为约定 JSON 错误体）。
func errBody(t *testing.T, w *httptest.ResponseRecorder) kiterrors.Error {
	t.Helper()
	var b struct {
		Error kiterrors.Error `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("解析错误体: %v（body=%s）", err, w.Body.String())
	}
	return b.Error
}

// ---------------------------------------------------------------------------
// CheckObject
// ---------------------------------------------------------------------------

// TestCheckObject 验证对象标识拼装：type + ":" + id 交给 Check。
func TestCheckObject(t *testing.T) {
	f := &fakeChecker{allowed: true}
	allowed, err := CheckObject(context.Background(), f, "widget", "o1", "can_view")
	if err != nil {
		t.Fatalf("CheckObject: %v", err)
	}
	if !allowed {
		t.Errorf("CheckObject 应透传 allowed=true")
	}
	if f.gotObject != "widget:o1" || f.gotRelation != "can_view" {
		t.Errorf("Check 入参错误：object=%q relation=%q，期望 widget:o1 / can_view", f.gotObject, f.gotRelation)
	}
}

// ---------------------------------------------------------------------------
// Remote（httptest 对端）
// ---------------------------------------------------------------------------

// remoteUpstream 构造按自定义 handler 响应的测试对端，返回指向它的 Remote。
func remoteUpstream(t *testing.T, handler http.HandlerFunc) *Remote {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL + "/", Timeout: 2 * time.Second})
}

// TestRemoteCheck 三态：200{allowed:true}→(true,nil)；200{allowed:false}→(false,nil)；
// 上游 503 kit 错误体 → 透传 *kiterrors.Error（code 原样、Status 由状态码补齐）。
func TestRemoteCheck(t *testing.T) {
	// 允许。
	r := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/authz/check" {
			t.Errorf("请求路径错误: %s", req.URL.Path)
		}
		var body struct {
			Object   string `json:"object"`
			Relation string `json:"relation"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body.Object != "widget:o1" || body.Relation != "can_view" {
			t.Errorf("请求体错误: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"allowed": true})
	})
	allowed, err := r.Check(context.Background(), "widget:o1", "can_view")
	if err != nil || !allowed {
		t.Fatalf("Check 允许场景: allowed=%v err=%v", allowed, err)
	}

	// 明确无权：200{allowed:false}，不报错。
	r2 := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"allowed": false})
	})
	allowed, err = r2.Check(context.Background(), "widget:o1", "can_view")
	if err != nil {
		t.Fatalf("Check 无权场景不应报错: %v", err)
	}
	if allowed {
		t.Errorf("无权场景 allowed 应为 false")
	}

	// 上游 503 + kit 错误体：code 透传，Status 由 HTTP 状态码补齐。
	r3 := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"AUTHZ_ENGINE_ERROR","message":"权限判定服务异常"}}`))
	})
	_, err = r3.Check(context.Background(), "widget:o1", "can_view")
	var e *kiterrors.Error
	if !errors.As(err, &e) {
		t.Fatalf("上游 kit 错误应解析为 *kiterrors.Error: %v", err)
	}
	if e.Code != "AUTHZ_ENGINE_ERROR" || e.Status != http.StatusServiceUnavailable {
		t.Errorf("错误透传不符: code=%q status=%d", e.Code, e.Status)
	}

	// 上游 500 非 kit 错误体：兜底 INTERNAL，上游内部细节不外泄。
	r4 := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err = r4.Check(context.Background(), "widget:o1", "can_view")
	if !errors.As(err, &e) || e.Code != "INTERNAL" || e.Status != http.StatusInternalServerError {
		t.Errorf("非 kit 错误体应兜底 INTERNAL/500: %v", err)
	}
}

// TestRemoteTokenAndRequestID 验证 ctx 身份与 request id 透传：
// WithToken 注入的 JWT 以 Authorization: Bearer 头发出；
// ctx 中的 request id 以 X-Request-ID 头发出。
func TestRemoteTokenAndRequestID(t *testing.T) {
	r := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer tok-1" {
			t.Errorf("Authorization 头错误: %q", got)
		}
		if got := req.Header.Get("X-Request-ID"); got != "rid-1" {
			t.Errorf("X-Request-ID 头错误: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"allowed": true})
	})
	ctx := WithToken(kiterrors.WithRequestID(context.Background(), "rid-1"), "tok-1")
	if _, err := r.Check(ctx, "widget:o1", "can_view"); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

// TestRemoteTimeout 验证超时语义：对端响应慢于 Config.Timeout → 返回错误（fail-closed）。
func TestRemoteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	r := New(Config{BaseURL: srv.URL, Timeout: 50 * time.Millisecond})
	if _, err := r.Check(context.Background(), "widget:o1", "can_view"); err == nil {
		t.Errorf("超时应返回错误")
	}
}

// TestRemoteListObjects 验证对象枚举：200{objects:[...]}→切片；无对象→空切片不报错。
func TestRemoteListObjects(t *testing.T) {
	r := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/authz/list-objects" {
			t.Errorf("请求路径错误: %s", req.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []string{"widget:o1", "widget:o2"}})
	})
	objs, err := r.ListObjects(context.Background(), "widget", "can_view")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(objs) != 2 || objs[0] != "widget:o1" || objs[1] != "widget:o2" {
		t.Errorf("ListObjects 结果错误: %v", objs)
	}

	// 空列表。
	r2 := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []string{}})
	})
	objs, err = r2.ListObjects(context.Background(), "widget", "can_view")
	if err != nil || len(objs) != 0 {
		t.Fatalf("空列表场景: objs=%v err=%v", objs, err)
	}
}

// TestRemoteCircuitMirror 是 clientx nil 分支教训的镜像测试：
// kit 纪律禁止 Remote 做运行时治理（熔断由服务治理层负责），但镜像断言仍须成立——
// 一次失败调用之后，后续连续成功调用必须全部正常（无粘性失败状态、
// 无"成功被计为失败"的自我熔断）。若未来引入任何调用间状态，此测试最先报警。
func TestRemoteCircuitMirror(t *testing.T) {
	fail := true
	r := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"allowed": true})
	})
	// 第一次调用失败（上游 500）。
	if _, err := r.Check(context.Background(), "widget:o1", "can_view"); err == nil {
		t.Fatalf("首次调用应失败")
	}
	// 上游恢复后连续 20 次成功调用必须全部成功。
	fail = false
	for i := 0; i < 20; i++ {
		allowed, err := r.Check(context.Background(), "widget:o1", "can_view")
		if err != nil || !allowed {
			t.Fatalf("第 %d 次连续成功调用被误判: allowed=%v err=%v（熔断/粘性失败嫌疑）", i+1, allowed, err)
		}
	}
}

// TestRemoteDefaultTimeout 验证未配置 Timeout 时取默认值（防止 0 超时导致请求必败）。
func TestRemoteDefaultTimeout(t *testing.T) {
	r := New(Config{BaseURL: "http://127.0.0.1:1"})
	if r.hc.Timeout != defaultTimeout {
		t.Errorf("默认超时应为 %s，实际 %s", defaultTimeout, r.hc.Timeout)
	}
}

// ---------------------------------------------------------------------------
// Require 中间件
// ---------------------------------------------------------------------------

// wrapRequire 构造 Require 中间件包裹的计数放行 handler。
func wrapRequire(t *testing.T, f *fakeChecker, perm string) (http.HandlerFunc, *int) {
	t.Helper()
	next := 0
	h := Require(f, perm)(func(w http.ResponseWriter, r *http.Request) {
		next++
		_, _ = w.Write([]byte("ok"))
	})
	return h, &next
}

// TestRequireAllow 验证放行链路：checker 允许 → next 执行 → ctx 携带身份透传。
func TestRequireAllow(t *testing.T) {
	f := &fakeChecker{allowed: true}
	h, next := wrapRequire(t, f, "widget:manage")

	w := httptest.NewRecorder()
	h(w, newRequireReq("/v1/widgets/o1", "Bearer tok-1"))
	if w.Code != http.StatusOK || *next != 1 {
		t.Fatalf("允许场景应放行: code=%d next=%d", w.Code, *next)
	}
	if f.gotObject != "widget:o1" || f.gotRelation != "manage" {
		t.Errorf("perm 解析错误: object=%q relation=%q", f.gotObject, f.gotRelation)
	}
	if f.gotToken != "tok-1" {
		t.Errorf("JWT 未进 ctx: %q", f.gotToken)
	}
}

// TestRequireDenied 验证无权语义：allowed=false → 403 AUTHZ_DENIED，next 不执行。
func TestRequireDenied(t *testing.T) {
	f := &fakeChecker{allowed: false}
	h, next := wrapRequire(t, f, "widget:manage")

	w := httptest.NewRecorder()
	h(w, newRequireReq("/v1/widgets/o1", ""))
	if w.Code != http.StatusForbidden || *next != 0 {
		t.Fatalf("无权应 403 且不放行: code=%d next=%d", w.Code, *next)
	}
	e := errBody(t, w)
	if e.Code != "AUTHZ_DENIED" {
		t.Errorf("错误码应逐字为 AUTHZ_DENIED: %q", e.Code)
	}
}

// TestRequireUnavailable 验证故障语义：checker 报错 → 503 AUTHZ_SERVICE_UNAVAILABLE（fail-closed）。
func TestRequireUnavailable(t *testing.T) {
	f := &fakeChecker{err: errors.New("connection refused")}
	h, next := wrapRequire(t, f, "widget:manage")

	w := httptest.NewRecorder()
	h(w, newRequireReq("/v1/widgets/o1", ""))
	if w.Code != http.StatusServiceUnavailable || *next != 0 {
		t.Fatalf("故障应 503 且不放行: code=%d next=%d", w.Code, *next)
	}
	e := errBody(t, w)
	if e.Code != "AUTHZ_SERVICE_UNAVAILABLE" {
		t.Errorf("错误码应逐字为 AUTHZ_SERVICE_UNAVAILABLE: %q", e.Code)
	}
}

// TestRequireInvalidPerm 验证 perm 配置错误 fail-closed：无冒号/空段 → 403，checker 不被调用。
func TestRequireInvalidPerm(t *testing.T) {
	for _, perm := range []string{"nocolon", "widget:", ":manage"} {
		f := &fakeChecker{allowed: true}
		h, next := wrapRequire(t, f, perm)
		w := httptest.NewRecorder()
		h(w, newRequireReq("/v1/widgets/o1", ""))
		if w.Code != http.StatusForbidden || *next != 0 || f.calls != 0 {
			t.Errorf("perm=%q 应 fail-closed 403: code=%d calls=%d", perm, w.Code, f.calls)
		}
	}
}

// TestRequireNoObjectID 验证路径无对象 ID（尾段为空）→ 403，checker 不被调用。
func TestRequireNoObjectID(t *testing.T) {
	f := &fakeChecker{allowed: true}
	h, next := wrapRequire(t, f, "widget:manage")
	w := httptest.NewRecorder()
	h(w, newRequireReq("/v1/widgets/", ""))
	if w.Code != http.StatusForbidden || *next != 0 || f.calls != 0 {
		t.Errorf("无对象 ID 应 fail-closed 403: code=%d calls=%d", w.Code, f.calls)
	}
}

// TestRequireRequestID 验证 request id 经 ctx 透传给 Checker（供 Remote 上行）。
func TestRequireRequestID(t *testing.T) {
	f := &fakeChecker{allowed: true}
	h, _ := wrapRequire(t, f, "widget:manage")
	r := newRequireReq("/v1/widgets/o1", "")
	r = r.WithContext(kiterrors.WithRequestID(r.Context(), "rid-9"))
	w := httptest.NewRecorder()
	h(w, r)
	if f.gotReqID != "rid-9" {
		t.Errorf("request id 未透传: %q", f.gotReqID)
	}
}

// TestRequireBearerPrefix 归一化：Authorization 头不带 Bearer 前缀时原样透传。
func TestRequireBearerPrefix(t *testing.T) {
	f := &fakeChecker{allowed: true}
	h, _ := wrapRequire(t, f, "widget:manage")
	w := httptest.NewRecorder()
	h(w, newRequireReq("/v1/widgets/o1", "raw-token"))
	if f.gotToken != "raw-token" {
		t.Errorf("裸 token 应原样进 ctx: %q", f.gotToken)
	}
}

// ---------------------------------------------------------------------------
// 上游身份错误（401）与 503 的语义区分（审查 Important 修复）
// ---------------------------------------------------------------------------

// TestRemoteUpstreamUnauthorized 验证上游 401 的归一化/透传：
//   - 非约定体（go-zero jwt 的 401）→ 归一为 401 AUTHZ_UNAUTHENTICATED；
//   - 约定体（authz logic 层 401 AUTHZ_UNAUTHORIZED）→ code 原样透传，Status 补 401。
func TestRemoteUpstreamUnauthorized(t *testing.T) {
	// 非约定体。
	r := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
	_, err := r.Check(context.Background(), "widget:o1", "can_view")
	var e *kiterrors.Error
	if !errors.As(err, &e) {
		t.Fatalf("401 应解析为 *kiterrors.Error: %v", err)
	}
	if e.Status != http.StatusUnauthorized || e.Code != CodeUnauthenticated {
		t.Errorf("非约定体 401 应归一为 401/%s: status=%d code=%q", CodeUnauthenticated, e.Status, e.Code)
	}

	// 约定体。
	r2 := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"AUTHZ_UNAUTHORIZED","message":"ctx 中无身份信息"}}`))
	})
	_, err = r2.Check(context.Background(), "widget:o1", "can_view")
	if !errors.As(err, &e) {
		t.Fatalf("401 应解析为 *kiterrors.Error: %v", err)
	}
	if e.Status != http.StatusUnauthorized || e.Code != "AUTHZ_UNAUTHORIZED" {
		t.Errorf("约定体 401 应透传 code: status=%d code=%q", e.Status, e.Code)
	}
}

// TestRemoteUpstreamKitBodyWithoutCode 验证约定体缺 code 字段按不可解析处理：
// 兜底 INTERNAL（Require 层按 503 fail-closed），不产出空码错误体。
func TestRemoteUpstreamKitBodyWithoutCode(t *testing.T) {
	r := remoteUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"缺 code 的错误体"}}`))
	})
	_, err := r.Check(context.Background(), "widget:o1", "can_view")
	var e *kiterrors.Error
	if !errors.As(err, &e) {
		t.Fatalf("应解析为 *kiterrors.Error: %v", err)
	}
	if e.Code != "INTERNAL" || e.Status != http.StatusInternalServerError {
		t.Errorf("缺 code 约定体应兜底 INTERNAL/500: code=%q status=%d", e.Code, e.Status)
	}
}

// TestRequireUpstreamUnauthorized 验证 Require 对上游 401 的语义：
// 原样回写 401（调用方可换 token 自救），不是 503；next 不执行。
func TestRequireUpstreamUnauthorized(t *testing.T) {
	f := &fakeChecker{err: kiterrors.New(http.StatusUnauthorized, CodeUnauthenticated, "未认证或凭证已失效")}
	h, next := wrapRequire(t, f, "widget:manage")

	w := httptest.NewRecorder()
	h(w, newRequireReq("/v1/widgets/o1", "Bearer expired-token"))
	if w.Code != http.StatusUnauthorized || *next != 0 {
		t.Fatalf("上游身份错误应 401 且不放行: code=%d next=%d", w.Code, *next)
	}
	e := errBody(t, w)
	if e.Code != CodeUnauthenticated {
		t.Errorf("错误码应逐字为 %s: %q", CodeUnauthenticated, e.Code)
	}
}
