package errors

import (
	"context"
	stderrors "errors"
	"net/http"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// body 是错误响应体结构，全栈统一（backend-design §4.2），前端依赖此格式。
type body struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// errorBody 把 error 转为 (HTTP 状态码, 响应体)：
//   - *Error：按其 Status/Code 输出（含 %w 包装链中的 *Error，用 errors.As 识别）
//   - 其他 error：一律 500 INTERNAL，原始 err 内容不进响应体（防泄漏）；
//     code 与 From、middleware.Recover 的兜底保持一致（统一错误码体系）
func errorBody(ctx context.Context, err error) (int, body) {
	var b body
	b.Error.RequestID = RequestIDFrom(ctx)
	var e *Error
	if stderrors.As(err, &e) {
		b.Error.Code, b.Error.Message = e.Code, e.Message
		return e.Status, b
	}
	b.Error.Code, b.Error.Message = "INTERNAL", "服务内部错误"
	// 原始 err 的最后可见点必须落日志（否则 500 无从排障，AGENTS.md 错误显式
	// 处理纪律）：*Error 是业务语义错误由调用方按需记录，其余（如参数解析失败、
	// 未归一化的底层错误）在此记完整原始 err；日志直用 logx 而非 kit/logging，
	// 避免与 kit/logging → kit/errors 的依赖成环。
	logx.WithContext(ctx).Errorf("unhandled error (request_id=%s): %v", b.Error.RequestID, err)
	return http.StatusInternalServerError, b
}

// InstallErrorHandler 注册 go-zero 全局错误出口，输出统一 JSON 错误体。
// 服务 main.go 启动时调用一次。使用 SetErrorHandlerCtx（v1.10.3 与
// SetErrorHandler 设置同一 handler），回调可拿到请求 ctx，从而写入 request_id。
func InstallErrorHandler() {
	httpx.SetErrorHandlerCtx(func(ctx context.Context, err error) (int, any) {
		status, b := errorBody(ctx, err)
		return status, b
	})
}
