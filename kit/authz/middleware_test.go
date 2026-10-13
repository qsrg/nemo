// middleware_test.go 覆盖 RequireWith 声明式守卫：对象 ID 提取器（自定义
// 提取器 / 提取失败 400 AUTHZ_OBJECT_INVALID）、权限码回退链（持有放行 /
// 全不持有 403 / 回退判定故障 503 fail-closed）、以及 Require 兼容形态
// （既有 authz_test.go 的 Require 用例继续覆盖简路径与 401/503 语义）。
package authz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	kiterrors "nemo/kit/errors"
)

// fakeListChecker 是支持 Check 与 ListObjects 双路的测试替身：Check 按配置，
// ListObjects 返回预设权限点对象集合（回退链判定用）。
type fakeListChecker struct {
	allowed    bool
	checkErr   error
	objects    []string
	listErr    error
	checkCalls int
	listCalls  int
}

// Check 返回预设判定结果（或故障）。
func (f *fakeListChecker) Check(context.Context, string, string) (bool, error) {
	f.checkCalls++
	return f.allowed, f.checkErr
}

// ListObjects 返回预设权限点对象集合（或故障）。
func (f *fakeListChecker) ListObjects(context.Context, string, string) ([]string, error) {
	f.listCalls++
	return f.objects, f.listErr
}

// BatchCheck 测试替身不使用。
func (f *fakeListChecker) BatchCheck(context.Context, []BatchCheckItem) ([]bool, error) {
	return nil, nil
}

// newGuardReq 构造带 Bearer 头的测试请求。
func newGuardReq(path string) *http.Request {
	r := httptest.NewRequest(http.MethodPut, path, nil)
	r.Header.Set("Authorization", "Bearer tok-1")
	return r
}

// digitID 是测试用对象 ID 提取器：取路径末段且须为正整数（kafka 守卫同型）。
func digitID(r *http.Request) (string, bool) {
	id, err := strconv.ParseInt(pathObjectID(r.URL.Path), 10, 64)
	if err != nil || id <= 0 {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}

// TestRequireWithCustomObjectID 自定义提取器：对象 ID 取自提取器，主判定放行。
func TestRequireWithCustomObjectID(t *testing.T) {
	f := &fakeListChecker{allowed: true}
	next := 0
	h := RequireWith(f, Options{Perm: "widget:can_edit", ObjectID: digitID})(
		func(w http.ResponseWriter, r *http.Request) {
			next++
			_, _ = w.Write([]byte("ok"))
		})
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusOK || next != 1 {
		t.Fatalf("自定义提取器放行场景: code=%d next=%d", w.Code, next)
	}
}

// TestRequireWithInvalidObjectID 提取器判非法 → 400 AUTHZ_OBJECT_INVALID，
// 不调用判定。
func TestRequireWithInvalidObjectID(t *testing.T) {
	f := &fakeListChecker{allowed: true}
	next := 0
	h := RequireWith(f, Options{Perm: "widget:can_edit", ObjectID: digitID})(
		func(w http.ResponseWriter, r *http.Request) { next++ })
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/abc"))
	if w.Code != http.StatusBadRequest || next != 0 || f.checkCalls != 0 {
		t.Fatalf("非法 ID 应 400 且不进判定: code=%d next=%d calls=%d", w.Code, next, f.checkCalls)
	}
	e := errBody(t, w)
	if e.Code != CodeObjectInvalid {
		t.Errorf("错误码应逐字为 %s: %q", CodeObjectInvalid, e.Code)
	}
}

// TestRequireWithFallbackGranted 回退链放行：主判定明确无权，回退权限码
// 持有 → 放行（页面级管理码全权语义）。
func TestRequireWithFallbackGranted(t *testing.T) {
	f := &fakeListChecker{
		allowed: false,
		objects: []string{"permission:widgets.manage"},
	}
	next := 0
	h := RequireWith(f, Options{
		Perm:          "widget:can_edit",
		ObjectID:      digitID,
		FallbackCodes: []string{"widgets:manage"},
	})(func(w http.ResponseWriter, r *http.Request) {
		next++
		_, _ = w.Write([]byte("ok"))
	})
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusOK || next != 1 {
		t.Fatalf("回退码持有应放行: code=%d next=%d", w.Code, next)
	}
	if f.listCalls != 1 {
		t.Errorf("回退判定应恰好一次 ListObjects，实际 %d", f.listCalls)
	}
}

// TestRequireWithFallbackDenied 主判定与回退码均无 → 403 AUTHZ_DENIED。
func TestRequireWithFallbackDenied(t *testing.T) {
	f := &fakeListChecker{allowed: false, objects: []string{"permission:redis.read"}}
	next := 0
	h := RequireWith(f, Options{
		Perm:          "widget:can_edit",
		ObjectID:      digitID,
		FallbackCodes: []string{"widgets:manage"},
	})(func(w http.ResponseWriter, r *http.Request) { next++ })
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusForbidden || next != 0 {
		t.Fatalf("主判定与回退均无权应 403: code=%d next=%d", w.Code, next)
	}
	e := errBody(t, w)
	if e.Code != CodeDenied {
		t.Errorf("错误码应逐字为 AUTHZ_DENIED: %q", e.Code)
	}
}

// TestRequireWithFallbackUnavailable 回退判定故障 → 503 fail-closed，
// 不吞错放行。
func TestRequireWithFallbackUnavailable(t *testing.T) {
	f := &fakeListChecker{allowed: false, listErr: errors.New("connection refused")}
	next := 0
	h := RequireWith(f, Options{
		Perm:          "widget:can_edit",
		ObjectID:      digitID,
		FallbackCodes: []string{"widgets:manage"},
	})(func(w http.ResponseWriter, r *http.Request) { next++ })
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusServiceUnavailable || next != 0 {
		t.Fatalf("回退判定故障应 503 fail-closed: code=%d next=%d", w.Code, next)
	}
	e := errBody(t, w)
	if e.Code != CodeUnavailable {
		t.Errorf("错误码应逐字为 %s: %q", CodeUnavailable, e.Code)
	}
}

// TestRequireWithFallbackUnauthorized 回退判定遇到上游身份错误（401）必须
// 原样透传（回归测试：修复前回退分支写死 503，把过期 token 误报成服务故障，
// 与主判定路径的 401 透传语义分裂）。
func TestRequireWithFallbackUnauthorized(t *testing.T) {
	f := &fakeListChecker{
		allowed: false,
		listErr: kiterrors.New(http.StatusUnauthorized, CodeUnauthenticated, "未认证或凭证已失效"),
	}
	next := 0
	h := RequireWith(f, Options{
		Perm:          "widget:can_edit",
		ObjectID:      digitID,
		FallbackCodes: []string{"widgets:manage"},
	})(func(w http.ResponseWriter, r *http.Request) { next++ })
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusUnauthorized || next != 0 {
		t.Fatalf("回退判定上游 401 应透传且不放行: code=%d next=%d", w.Code, next)
	}
	e := errBody(t, w)
	if e.Code != CodeUnauthenticated {
		t.Errorf("错误码应逐字为 %s: %q", CodeUnauthenticated, e.Code)
	}
}

// TestRequireWithMainCheckUnavailable 主判定故障 → 503，不进回退链。
func TestRequireWithMainCheckUnavailable(t *testing.T) {
	f := &fakeListChecker{checkErr: errors.New("timeout"), objects: []string{"permission:widgets.manage"}}
	next := 0
	h := RequireWith(f, Options{
		Perm:          "widget:can_edit",
		ObjectID:      digitID,
		FallbackCodes: []string{"widgets:manage"},
	})(func(w http.ResponseWriter, r *http.Request) { next++ })
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusServiceUnavailable || next != 0 || f.listCalls != 0 {
		t.Fatalf("主判定故障应 503 且不进回退: code=%d next=%d listCalls=%d", w.Code, next, f.listCalls)
	}
}

// TestRequireCompatWithRequire Require 是 RequireWith 的简形态：
// 仅 Perm 零值配置行为与旧 Require 一致（路径末段对象 ID、无 ID 403）。
func TestRequireCompatWithRequire(t *testing.T) {
	f := &fakeListChecker{allowed: true}
	h := Require(f, "widget:can_edit")(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets/42"))
	if w.Code != http.StatusOK {
		t.Fatalf("Require 简形态应保持放行语义: code=%d", w.Code)
	}
	// 无对象 ID（尾段空）：403，同旧语义。
	f2 := &fakeListChecker{allowed: true}
	h2 := Require(f2, "widget:can_edit")(func(w http.ResponseWriter, r *http.Request) {})
	w2 := httptest.NewRecorder()
	h2(w2, newGuardReq("/v1/widgets/"))
	if w2.Code != http.StatusForbidden {
		t.Fatalf("无对象 ID 应 403: code=%d", w2.Code)
	}
}

// wrapCode 构造 RequireCode 守卫包裹的计数放行 handler。
func wrapCode(t *testing.T, f *fakeListChecker, codes ...string) (http.HandlerFunc, *int) {
	t.Helper()
	next := 0
	h := RequireCode(f, codes...)(func(w http.ResponseWriter, r *http.Request) {
		next++
		_, _ = w.Write([]byte("ok"))
	})
	return h, &next
}

// TestRequireCodeGranted 持有任一码即放行；集合顺序无关。
func TestRequireCodeGranted(t *testing.T) {
	f := &fakeListChecker{objects: []string{"permission:widgets.manage"}}
	h, next := wrapCode(t, f, "widgets:read", "widgets:manage")
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets"))
	if w.Code != http.StatusOK || *next != 1 {
		t.Fatalf("持有码应放行: code=%d next=%d", w.Code, *next)
	}
}

// TestRequireCodeDenied 全不持有 → 403 AUTHZ_DENIED。
func TestRequireCodeDenied(t *testing.T) {
	f := &fakeListChecker{objects: []string{"permission:redis.read"}}
	h, next := wrapCode(t, f, "widgets:manage")
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets"))
	if w.Code != http.StatusForbidden || *next != 0 {
		t.Fatalf("不持有应 403 且不放行: code=%d next=%d", w.Code, *next)
	}
	e := errBody(t, w)
	if e.Code != CodeDenied {
		t.Errorf("错误码应逐字为 AUTHZ_DENIED: %q", e.Code)
	}
}

// TestRequireCodeEmpty 配置错误（无码）fail-closed：403，不调用判定。
func TestRequireCodeEmpty(t *testing.T) {
	f := &fakeListChecker{allowed: true}
	h, next := wrapCode(t, f)
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets"))
	if w.Code != http.StatusForbidden || *next != 0 || f.listCalls != 0 {
		t.Fatalf("无码应 fail-closed 403: code=%d next=%d calls=%d", w.Code, *next, f.listCalls)
	}
}

// TestRequireCodeUnavailable 判定故障 → 503 fail-closed；上游 401 原样透传
// （与 RequireWith 回退链同口径）。
func TestRequireCodeUnavailable(t *testing.T) {
	f := &fakeListChecker{listErr: errors.New("connection refused")}
	h, next := wrapCode(t, f, "widgets:manage")
	w := httptest.NewRecorder()
	h(w, newGuardReq("/v1/widgets"))
	if w.Code != http.StatusServiceUnavailable || *next != 0 {
		t.Fatalf("判定故障应 503 fail-closed: code=%d next=%d", w.Code, *next)
	}

	f2 := &fakeListChecker{listErr: kiterrors.New(http.StatusUnauthorized, CodeUnauthenticated, "未认证")}
	h2, next2 := wrapCode(t, f2, "widgets:manage")
	w2 := httptest.NewRecorder()
	h2(w2, newGuardReq("/v1/widgets"))
	if w2.Code != http.StatusUnauthorized || *next2 != 0 {
		t.Fatalf("上游 401 应透传且不放行: code=%d next=%d", w2.Code, *next2)
	}
}
