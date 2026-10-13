// Package errors 定义全栈统一的错误约定（架构 §5）：错误码、HTTP 状态与 JSON 错误体。
//
// 约定：
//   - code 采用 <域>_<原因> 大写下划线，全局唯一（如 USER_NOT_FOUND、ORDER_USER_INVALID）
//   - message 面向调用方可读，中文描述
//   - 错误体统一为 {"error":{"code":"...","message":"...","request_id":"..."}}
//   - 成功响应直接返回资源 JSON，不包裹额外信封；分页为 {"items":[],"total":n}
package errors

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
)

// Error 是业务错误的统一载体。Status 与 Code 一一对应，在 New 时显式声明。
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// 语义化构造器（BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）
// 见 constructors.go；统一错误出口见 gozero.go。

// From 将任意 error 归一化为 *Error：已是 *Error（含 %w 包装链中的 *Error，
// 用 errors.As 识别）的直接透传——跨服务调用传递上游错误码时依赖此行为；
// 其余一律视为 INTERNAL（错误码与 errorBody 兜底一致）。
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if stderrors.As(err, &e) {
		return e
	}
	return New(http.StatusInternalServerError, "INTERNAL", "服务内部错误")
}

// WriteJSON 按 kit 统一格式输出错误体，request_id 取自请求 context。
// 与全局错误出口（errorBody，gozero.go）共用同一响应体结构与字段语义，
// handler 中错误出口统一走这里或返回 error 交由 go-zero 出口，不要自行拼错误 JSON。
func (e *Error) WriteJSON(w http.ResponseWriter, r *http.Request) {
	status, b := errorBody(r.Context(), e)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(b)
}

// Decode 从响应体解析统一错误体（kit httpx.Client 与前端 packages/http 对应实现）。
func Decode(b []byte) *Error {
	var body struct {
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(b, &body); err != nil || body.Error == nil {
		return nil
	}
	return body.Error
}
