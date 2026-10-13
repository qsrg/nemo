package errors

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ctxKey 是 request id 的 context 私有键类型，防止跨包键冲突。
type ctxKey struct{}

// WithRequestID 将请求 id 写入 context（kit/logging 中间件写入，统一错误出口读取）。
// 键由本包私有持有：RequestID 中间件、(*Error).WriteJSON 与 errorBody 共用同一来源，
// 保证同一请求全链路只有一个 request id。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestIDFrom 从 context 取请求 id；不存在返回空串。
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// NewRequestID 生成 128 位随机 hex 作为请求 id；随机源不可用时退化为纳秒时间戳。
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
