package errors

import "net/http"

// 六个语义构造器：状态码与错误码一一对应，logic 层禁止手工构造非语义化错误
// （原始 err 归一化统一走 From）。

// BadRequest 返回 400 错误。
func BadRequest(code, message string) *Error { return New(http.StatusBadRequest, code, message) }

// Unauthorized 返回 401 错误。
func Unauthorized(code, message string) *Error { return New(http.StatusUnauthorized, code, message) }

// Forbidden 返回 403 错误。
func Forbidden(code, message string) *Error { return New(http.StatusForbidden, code, message) }

// NotFound 返回 404 错误。
func NotFound(code, message string) *Error { return New(http.StatusNotFound, code, message) }

// Conflict 返回 409 错误。
func Conflict(code, message string) *Error { return New(http.StatusConflict, code, message) }

// Internal 返回 500 错误；原始 err 不进响应体，只进日志。
func Internal(code, message string) *Error {
	return New(http.StatusInternalServerError, code, message)
}
