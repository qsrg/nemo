// middleware.go 提供 Require / RequireWith 权限中间件（标准库 http.HandlerFunc
// 形态）：以 perm 字符串 "<对象类型>:<关系>" 声明保护语义。Require 是对象
// 实例 ID 取自请求路径末段的简形态（/v1/widgets/o1 → "o1"，无实例 ID 的集合
// 路由不适用）；RequireWith 是声明式进阶形态（Options）——自定义对象 ID
// 提取器（多级路径/格式校验）与权限码回退链（资源级判定不通过时兜底
// 管理码全权）。
// 判定经注入的 Checker 完成（测试注入 fake，生产注入 Remote）；
// 请求头中的 JWT 提取后写入 ctx（WithToken），由 Remote 实现透传给判定服务。
package authz

import (
	stderrors "errors"
	"net/http"
	"strings"

	kiterrors "nemo/kit/errors"
)

// 语义错误码（全局约束逐字）：
//   - 无权 403 AUTHZ_DENIED；
//   - 判定服务不可用 503 AUTHZ_SERVICE_UNAVAILABLE（fail-closed）；
//   - 上游身份错误 401（AUTHZ_UNAUTHENTICATED 或上游透传的 401 kit 错误码，
//     语义设计见 remote.go upstreamError——"未认证"与"无权"是两类事实，
//     AUTHZ_DENIED 只表达后者，故 401 单列）；
//   - 路径无法产出合法对象 ID 400 AUTHZ_OBJECT_INVALID（RequireWith 配置了
//     提取器时——请求侧格式问题与"无权"是两类事实，用 400 区分）。
const (
	// CodeDenied 是无权访问的错误码。
	CodeDenied = "AUTHZ_DENIED"
	// CodeUnavailable 是判定服务不可用的错误码。
	CodeUnavailable = "AUTHZ_SERVICE_UNAVAILABLE"
	// CodeUnauthenticated 是上游身份错误（未认证/凭证失效）的兜底错误码。
	CodeUnauthenticated = "AUTHZ_UNAUTHENTICATED"
	// CodeObjectInvalid 是对象 ID 提取失败（路径格式非法）的错误码。
	CodeObjectInvalid = "AUTHZ_OBJECT_INVALID"
)

// 语义错误文案（中文，与 kit 约定一致）。
var (
	errDenied        = kiterrors.New(http.StatusForbidden, CodeDenied, "无权访问该资源")
	errUnavailable   = kiterrors.New(http.StatusServiceUnavailable, CodeUnavailable, "权限判定服务不可用")
	errObjectInvalid = kiterrors.New(http.StatusBadRequest, CodeObjectInvalid, "请求路径中的资源 ID 非法")
)

// Options 是 RequireWith 的声明式配置。
type Options struct {
	// Perm 主判定 "<对象类型>:<关系>"（如 order:can_edit）。
	Perm string
	// ObjectID 从请求提取对象实例 ID。nil 时取路径末段（与 Require 一致：
	// 无实例 ID 按 403 fail-closed）。非 nil 时返回 ok=false 表示路径无法
	// 产出合法 ID（格式非法），中间件以 400 AUTHZ_OBJECT_INVALID 拒绝，
	// 不调用判定——调用方在提取器内做格式校验即可保留 400 契约。
	ObjectID func(*http.Request) (string, bool)
	// FallbackCodes 权限码回退链：主判定明确无权时逐个经 CodeGranted 判定，
	// 任一持有即放行（页面级管理码"管理员全权"语义）；判定故障仍按 503
	// fail-closed 处理，不吞错放行。
	FallbackCodes []string
}

// Require 返回按 perm（形如 "<对象类型>:<关系>"）保护 next 的中间件。
// 等价于 RequireWith(c, Options{Perm: perm})，保留既有签名供简形态路由使用。
// 判定链路与错误语义见 RequireWith。
func Require(c Checker, perm string) func(http.HandlerFunc) http.HandlerFunc {
	return RequireWith(c, Options{Perm: perm})
}

// RequireWith 返回按 opts 声明式保护 next 的中间件。判定链路：
//  1. Perm 解析失败（无冒号/空段）→ 403 AUTHZ_DENIED（fail-closed，配置
//     错误按拒绝处理，不调用 Checker）；
//  2. 对象实例 ID 提取：Options.ObjectID 缺省取路径末段（空 → 403，同
//     Require）；配置了提取器且返回 !ok → 400 AUTHZ_OBJECT_INVALID；
//  3. 请求头 Authorization（Bearer 前缀可省）提取 JWT 写入 ctx；
//  4. 主判定 Check 报错 → 按错误类别区分：
//     - 上游身份错误（*kiterrors.Error 且 Status==401，Remote 归一化/透传）
//     → 原样回写 401（调用方可自行修复——换有效 token 重试，与"服务故障"
//     语义区分，避免过期 token 被误报成 503）；
//     - 其余错误（网络/5xx/超时/上游其他 4xx 契约错误）→ 503
//     AUTHZ_SERVICE_UNAVAILABLE（fail-closed）；
//  5. 主判定明确无权 → 逐个权限码回退判定（CodeGranted）：持有任一码即
//     放行；全部不持有 → 403 AUTHZ_DENIED；回退判定故障经 writeCheckError
//     分类——上游 401 原样透传、其余 503 fail-closed（与主判定同口径）；
//  6. 允许 → 放行 next（ctx 已带身份透传）。
func RequireWith(c Checker, opts Options) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// perm 解析：首个冒号切分对象类型与关系；任一段为空即配置错误。
			objType, relation, ok := strings.Cut(opts.Perm, ":")
			if !ok || objType == "" || relation == "" {
				errDenied.WriteJSON(w, r)
				return
			}

			// 对象实例 ID：缺省提取器取路径末段（无 ID 按配置错误 403，
			// 与 Require 既有语义一致）；自定义提取器 !ok 按请求格式问题 400。
			var objID string
			if opts.ObjectID == nil {
				if objID = pathObjectID(r.URL.Path); objID == "" {
					errDenied.WriteJSON(w, r)
					return
				}
			} else {
				id, valid := opts.ObjectID(r)
				if !valid || id == "" {
					errObjectInvalid.WriteJSON(w, r)
					return
				}
				objID = id
			}

			// 身份提取：JWT 写入 ctx，供 Remote 透传（无 Authorization 头时
			// 仍照常判定，由判定服务 fail-closed 处理匿名请求）。
			ctx := r.Context()
			if tok := bearerToken(r); tok != "" {
				ctx = WithToken(ctx, tok)
			}

			allowed, err := CheckObject(ctx, c, objType, objID, relation)
			if err != nil {
				writeCheckError(w, r, err)
				return
			}
			if !allowed {
				// 权限码回退链：主判定明确无权后逐码 CodeGranted，任一持有
				// 即放行；判定故障经 writeCheckError 分类（上游 401 原样透传、
				// 其余 503 fail-closed——与主判定路径同口径，不吞错放行）。
				for _, code := range opts.FallbackCodes {
					granted, gerr := CodeGranted(ctx, c, code)
					if gerr != nil {
						writeCheckError(w, r, gerr)
						return
					}
					if granted {
						next(w, r.WithContext(ctx))
						return
					}
				}
				errDenied.WriteJSON(w, r)
				return
			}
			next(w, r.WithContext(ctx))
		}
	}
}

// RequireCode 返回按权限码集合保护 next 的中间件（资源创建等**无对象实例
// ID 的集合路由**：如"新建 Topic"入口持管理码即可）：逐码 CodeGranted，
// 任一持有即放行。
// 错误语义与 RequireWith 判定链一致：上游身份错误（401）原样透传、其余
// 判定故障 503 AUTHZ_SERVICE_UNAVAILABLE（fail-closed）、全部不持有 403
// AUTHZ_DENIED；codes 为空按配置错误 403 fail-closed，不调用判定。
func RequireCode(c Checker, codes ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 配置错误 fail-closed：无码可判即拒绝，不调用 Checker。
			if len(codes) == 0 {
				errDenied.WriteJSON(w, r)
				return
			}
			// 身份提取：JWT 写入 ctx，供 Remote 透传（同 RequireWith）。
			ctx := r.Context()
			if tok := bearerToken(r); tok != "" {
				ctx = WithToken(ctx, tok)
			}
			for _, code := range codes {
				granted, gerr := CodeGranted(ctx, c, code)
				if gerr != nil {
					writeCheckError(w, r, gerr)
					return
				}
				if granted {
					next(w, r.WithContext(ctx))
					return
				}
			}
			errDenied.WriteJSON(w, r)
		}
	}
}

// writeCheckError 按错误类别回写判定故障：上游身份错误（401）原样回写
// （调用方可换 token 自救）；其余（网络/5xx/超时/上游契约错误）→ 503
// fail-closed。
func writeCheckError(w http.ResponseWriter, r *http.Request, err error) {
	var e *kiterrors.Error
	if stderrors.As(err, &e) && e.Status == http.StatusUnauthorized {
		e.WriteJSON(w, r)
		return
	}
	errUnavailable.WriteJSON(w, r)
}

// pathObjectID 取请求路径末段作为对象实例 ID（/v1/widgets/o1 → "o1"）；
// 路径为空或以 / 结尾（集合路由，无实例 ID）返回 ""，由调用方 fail-closed 拒绝。
// 注意：Require 只应挂在带 :id 的路由上；集合路由没有实例 ID 可判定。
func pathObjectID(p string) string {
	if p == "" || strings.HasSuffix(p, "/") {
		return ""
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// bearerToken 从请求头提取 JWT：去 "Bearer " 前缀（大小写不敏感），
// 无该头返回空串，裸 token 原样返回。
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	if len(h) >= 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(h)
}
