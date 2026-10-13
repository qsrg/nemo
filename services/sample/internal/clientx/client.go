// Package clientx 提供跨服务 HTTP 调用基座样板：统一注入 request id 与
// Bearer 令牌、固定超时、按主机熔断、非 2xx 统一错误体解析。
// 新服务接入时复制本目录按需裁剪（kit 不收编为公共包，避免过早抽象）。
package clientx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/breaker"

	kiterrors "nemo/kit/errors"
)

// callTimeout 是单次上游调用的固定超时。
const callTimeout = 5 * time.Second

// ctxKey 是 Bearer 令牌的 context 私有键类型，防止跨包键冲突。
type ctxKey struct{}

// WithToken 将 Bearer 令牌写入 context；Do 读取并注入 Authorization 头。
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ctxKey{}, token)
}

// tokenFrom 从 context 取 Bearer 令牌；不存在返回空串。
func tokenFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// Client 是跨服务 HTTP 调用基座：Do 为唯一入口，出入参均为 any（JSON 编解码），
// 与具体业务类型解耦。
type Client struct {
	// baseURL 上游服务根地址（形如 http://127.0.0.1:8085，末尾 / 会被剔除）。
	baseURL string
	// hc 承载固定超时的底层 HTTP 客户端。
	hc *http.Client
}

// New 创建指向 baseURL 的调用基座：单次调用超时 5s；熔断器按主机名隔离
// （go-zero breaker 包内按名称缓存熔断器，Client 无需持有状态）。
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		hc:      &http.Client{Timeout: callTimeout},
	}
}

// Do 执行一次上游调用：
//   - in 非 nil 时序列化为 JSON 请求体（Content-Type: application/json）；
//   - out 非 nil 且响应体非空时按 JSON 解码到 out；
//   - 注入 X-Request-ID（kiterrors.RequestIDFrom 取自 ctx，存在才带）与
//     Authorization: Bearer（WithToken 注入，存在才带）；
//   - 每次调用经 go-zero breaker（按主机名隔离）：5xx 与网络错误计熔断失败，4xx 不计；
//   - 非 2xx：响应体按统一错误体解析为 *kiterrors.Error 原样带回（code/message
//     透传，Status 由 HTTP 状态码补齐），调用方用 errors.As 判定。
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("clientx: 序列化请求体: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("clientx: 构造请求 %s%s: %w", method, path, err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if id := kiterrors.RequestIDFrom(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}
	if token := tokenFrom(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// 熔断按主机隔离：同一上游共享失败统计，不同上游互不影响；
	// execute 返回的错误即最终错误（含 *kiterrors.Error），直接透传给调用方。
	return breaker.DoWithAcceptable(req.URL.Host, func() error {
		return c.doHTTP(req, out)
	}, acceptable)
}

// doHTTP 执行单次 HTTP 往返：2xx 解码出参；非 2xx 归一化为 *kiterrors.Error；
// 网络/解码错误包装上下文返回（计入熔断失败，见 acceptable）。
func (c *Client) doHTTP(req *http.Request, out any) error {
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("clientx: 请求上游 %s%s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("clientx: 读取上游响应 %s%s: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return upstreamError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("clientx: 解码上游响应 %s%s: %w", req.Method, req.URL.Path, err)
		}
	}
	return nil
}

// upstreamError 将非 2xx 响应体解析为统一错误：约定错误体（{"error":{...}}）经
// kit/errors.Decode 原样带回（code/message 透传，Status 由 HTTP 状态码补齐——
// Decode 不解析该字段）；其余错误体兜底 INTERNAL，上游内部细节不外泄。
func upstreamError(status int, body []byte) error {
	if e := kiterrors.Decode(body); e != nil {
		e.Status = status
		return e
	}
	return kiterrors.New(status, "INTERNAL", "上游服务错误")
}

// acceptable 是熔断判定：err==nil 必须计成功——go-zero breaker 对成功调用
// 也无条件回调 acceptable（googleBreaker.doReq 无 nil 保护），缺此分支会把
// 成功计为失败、accepts 恒 0，健康上游也会被自我熔断；*kiterrors.Error 且
// 状态 < 500（即 4xx）是调用方可预期的业务结果，不计失败；5xx 与其余错误
// （网络、解码等）计失败。
func acceptable(err error) bool {
	if err == nil {
		return true
	}
	var e *kiterrors.Error
	if errors.As(err, &e) {
		return e.Status < http.StatusInternalServerError
	}
	return false
}
