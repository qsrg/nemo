// remote.go 实现 Checker 的远端形态：把 Check/ListObjects 调用转发给集中式
// authz 判定服务（POST /v1/authz/check、/v1/authz/list-objects）。
// 身份透传：Checker 的 ctx 内含 JWT（WithToken 注入，Require 中间件从请求头
// 提取后写入），本实现取出后以 Authorization: Bearer 头发出；
// 同时按 kit 约定透传 X-Request-ID。非 2xx 响应解析规则（区分"上游身份错误"
// 与"服务故障"，详见 upstreamError）：401 → 透传或归一为 401
// AUTHZ_UNAUTHENTICATED（Require 原样回写，调用方可换 token 自救）；
// 其余经 kit/errors.Decode 原样带回或兜底 INTERNAL（Require 统一按 503
// fail-closed 处理）——上游内部细节不外泄。
package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	kiterrors "nemo/kit/errors"
)

// defaultTimeout 是未配置 Timeout 时的单次调用超时（防 0 超时导致请求必败）。
const defaultTimeout = 5 * time.Second

// ctxKey 是 JWT 的 context 私有键类型，防止跨包键冲突。
type ctxKey struct{}

// WithToken 将 JWT 写入 context；Remote 调用时读取并注入 Authorization 头。
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ctxKey{}, token)
}

// tokenFrom 从 context 取 JWT；不存在返回空串。
func tokenFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// Config 是 Remote 的构造参数。
type Config struct {
	// BaseURL 是 authz 服务根地址（形如 http://127.0.0.1:8084，末尾 / 会被剔除）。
	BaseURL string
	// Timeout 是单次调用超时；非正值取 defaultTimeout。
	Timeout time.Duration
	// ServiceTokenFn 返回服务身份凭证（如 svc-redis 服务账号的平台 JWT）。
	// 非空时本 Remote 实例的所有请求**优先**以它作 Bearer（ctx 内用户 token
	// 被覆盖）——供"服务端编排的元组写"（资源创建补 owner、删除清理、对账）
	// 使用服务身份，绕开名单管理权对终端用户的判定（调用方服务的路由守卫
	// 已验过业务操作权）。判定/名单查询类调用**必须**用无 ServiceTokenFn 的
	// 用户身份实例（服务视角的判定结果对终端用户无意义）。
	ServiceTokenFn func() string
}

// Remote 是 Checker 的远端实现（无调用间状态——kit 不做运行时治理，
// 熔断/重试由服务治理层负责）。
type Remote struct {
	// baseURL 是 authz 服务根地址。
	baseURL string
	// hc 承载固定超时的底层 HTTP 客户端。
	hc *http.Client
	// serviceTokenFn 非空时优先作为请求 Bearer（服务身份实例）。
	serviceTokenFn func() string
}

// New 创建指向 authz 服务的远端 Checker。
func New(cfg Config) *Remote {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Remote{
		baseURL:        strings.TrimSuffix(cfg.BaseURL, "/"),
		hc:             &http.Client{Timeout: timeout},
		serviceTokenFn: cfg.ServiceTokenFn,
	}
}

// bearer 取请求的 Bearer token：服务身份实例优先用 ServiceTokenFn，否则取
// ctx 内用户 token（Identity 中间件注入）。
func (r *Remote) bearer(ctx context.Context) string {
	if r.serviceTokenFn != nil {
		if tk := r.serviceTokenFn(); tk != "" {
			return tk
		}
	}
	return tokenFrom(ctx)
}

// Check 判定 object 上 relation 权限：POST /v1/authz/check。
// 200{allowed:false} 是"明确无权"，返回 (false, nil) 不报错；
// 上游 401（身份错误）返回带 Status=401 的 *kiterrors.Error 供 Require 原样
// 回写；网络/超时/其余非 2xx 一律返回 error（上层中间件按 503 fail-closed 处理）。
func (r *Remote) Check(ctx context.Context, object, relation string) (bool, error) {
	var out struct {
		Allowed bool `json:"allowed"`
	}
	in := struct {
		Object   string `json:"object"`
		Relation string `json:"relation"`
	}{Object: object, Relation: relation}
	if err := r.post(ctx, "/v1/authz/check", in, &out); err != nil {
		return false, err
	}
	return out.Allowed, nil
}

// ListObjects 枚举当前用户在 objType 类型对象上拥有 relation 权限的实例：
// POST /v1/authz/list-objects。
func (r *Remote) ListObjects(ctx context.Context, objType, relation string) ([]string, error) {
	var out struct {
		Objects []string `json:"objects"`
	}
	in := struct {
		Type     string `json:"type"`
		Relation string `json:"relation"`
	}{Type: objType, Relation: relation}
	if err := r.post(ctx, "/v1/authz/list-objects", in, &out); err != nil {
		return nil, err
	}
	return out.Objects, nil
}

// BatchCheck 一次判定多个 object/relation 组合，返回与 items 下标一一对应的
// bool 切片。POST /v1/authz/batch-check。语义与 Check 一致：
// 明确无权 → 对应位置 false，整体 error 为 nil；引擎/网络故障 → 整体 error。
func (r *Remote) BatchCheck(ctx context.Context, items []BatchCheckItem) ([]bool, error) {
	if len(items) == 0 {
		return []bool{}, nil
	}
	var out struct {
		Results []bool `json:"results"`
	}
	in := struct {
		Items []BatchCheckItem `json:"items"`
	}{Items: items}
	if err := r.post(ctx, "/v1/authz/batch-check", in, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// post 执行单次 JSON 调用：注入身份与 request id 头；2xx 解码出参；
// 非 2xx 经 kit/errors.Decode 归一化带回。
func (r *Remote) post(ctx context.Context, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("authz: 序列化请求体: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("authz: 构造请求 %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token := r.bearer(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if id := kiterrors.RequestIDFrom(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("authz: 请求上游 %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("authz: 读取上游响应 %s: %w", path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return upstreamError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("authz: 解码上游响应 %s: %w", path, err)
		}
	}
	return nil
}

// upstreamError 将非 2xx 响应体解析为统一错误。语义设计（区分"上游身份错误"与
// "服务故障"，Require 依赖此分类）：
//   - 上游 401 是身份错误（上游 JWT 校验失败/claims 缺失——判定面路由在 jwt 组内）：
//     调用方可自行修复（换有效 token 重试），必须与"服务故障 503"区分。
//     约定错误体（{"error":{...}}，如 authz logic 层的 AUTHZ_UNAUTHORIZED）可解析时
//     原样透传（code/message 保留，Status 补 401）；否则（go-zero jwt 中间件的
//     非约定体）归一为 401 AUTHZ_UNAUTHENTICATED——"未认证"不是"无权"，
//     AUTHZ_DENIED 不适用，且上游内部细节不外泄。
//   - 其余状态码（其他 4xx/5xx）：约定体可解析且 code 非空时原样带回供上层诊断；
//     否则兜底 INTERNAL，由 Require 统一按 503 fail-closed 处理——403 理论上
//     不出现在判定面，其他 4xx 属契约错误（调用方无法自行修复），不透传以免
//     把服务端契约问题伪装成客户端可解问题。
func upstreamError(status int, body []byte) error {
	if e := kiterrors.Decode(body); e != nil && e.Code != "" {
		e.Status = status
		return e
	}
	if status == http.StatusUnauthorized {
		return kiterrors.New(status, CodeUnauthenticated, "未认证或凭证已失效")
	}
	return kiterrors.New(status, "INTERNAL", "权限判定服务错误")
}
