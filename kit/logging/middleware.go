package logging

import (
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
)

// RequestID 中间件：读取入站 X-Request-ID（上游网关/服务已注入则透传，无则生成），
// 写入 context（供统一错误出口与业务取用）并回写响应头，全链路排障串联。
// 必须挂在中间件链最外层（AccessLog 依赖其写入的 request id）。
func RequestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = errors.NewRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next(w, r.WithContext(errors.WithRequestID(r.Context(), id)))
	}
}

// statusWriter 捕获下游写入的状态码，供访问日志读取；
// 其余方法透传内嵌 ResponseWriter，不改变原语义。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// AccessLog 中间件：以统一字段输出访问日志
// method/path/status/duration_ms/request_id（backend-design §8）。
// 挂在 RequestID 之后，保证 request_id 可取。
func AccessLog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next(sw, r)
		logx.Infow("access",
			logx.Field("method", r.Method),
			logx.Field("path", r.URL.Path),
			logx.Field("status", sw.status),
			logx.Field("duration_ms", time.Since(start).Milliseconds()),
			logx.Field("request_id", errors.RequestIDFrom(r.Context())))
	}
}
