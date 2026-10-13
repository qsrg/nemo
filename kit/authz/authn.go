// authn.go 提供平台统一身份中间件（kit 侧）：Identity 中间件把标准 JWT claims
// （sub/name/tenant/roles/typ）以类型化 Session 方式注入 ctx，IdentityFrom 供
// logic/中间件取用，TestToken 仅供测试与本地路由验证签发 token。
//
// 背景（go-zero v1.10.3 实际行为，经 go doc 与源码核对）：
// rest 的 jwt 中间件（rest/handler.Authorize）解析 token 后只把【非标准】claims
// 注入 ctx，标准 claims（sub/aud/exp/iss 等）被显式跳过——即业务层无法从 ctx
// 取到 claims.sub。为满足"user 一律取 claims.sub"的约束，Identity 中间件在
// jwt 校验之后复用 go-zero 同一 token 解析器按同一 secret 再解析一次，
// 仅做身份透传、不重复鉴权（鉴权仍由路由组 rest.WithJwt 负责）。
//
// 统一会话口径（服务合并 auth 并入 authz 后平台一致）：Session 携带 sub/name/
// tenant/roles/token（/v1/me 直取 name/tenant，/v1/auth/refresh 续签需透传原
// token）；typ=temp 的临时 token（仅 CAS 回调链路取角色用）不得访问任何业务
// 接口，Identity 直接 401 拒绝。
//
// 职责边界（kit 纪律）：不含业务逻辑、不出现业务名词；错误统一经 kit/errors
// 约定体输出。JWT 校验 fail-closed（401）由路由组 rest.WithJwt 承担，本中间件
// 只做身份透传——公开路由（healthz/readyz/登录通道）不挂 WithJwt 即可匿名访问。
package authz

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/rest/token"

	kiterrors "nemo/kit/errors"
)

// sessionCtxKey 是会话注入 ctx 的键类型，独立类型避免与其他包的字符串键冲突
//（与 remote.go 的 JWT ctx 键 ctxKey 区分开）。
type sessionCtxKey string

// keySession 是会话注入 ctx 的键。
const keySession sessionCtxKey = "authn.session"

// tokenTypeTemp 是临时 token 的 typ 标记（平台凭证契约：临时 token 只服务于
// CAS 回调链路向引擎取角色，短 TTL、无角色，不允许访问任何业务接口）。
const tokenTypeTemp = "temp"

// ErrNoIdentity 表示 ctx 中没有身份信息（未经 Identity 中间件或 token 无 sub），
// 调用方用 errors.Is 判定。
var ErrNoIdentity = errors.New("ctx 中无身份信息")

// Session 是当前用户会话（从平台 token 的 claims 提取的身份视图）。
type Session struct {
	// Sub 用户标识（JWT 标准 claim sub，裸用户名）。
	Sub string
	// Name 展示名（claim name）。
	Name string
	// Tenant 租户（一期固定 platform）。
	Tenant string
	// Roles 平台角色列表（claim roles）。
	Roles []string
	// Token 原始 Bearer token（不含 "Bearer " 前缀）——refresh 续签与
	// kit/authz Remote 透传用（WithToken 与 Session.Token 双写，中间件直接
	// 注入 authz ctx 键，logic 无需再取）。
	Token string
}

// Identity 返回按 secret 解析 Bearer token 并注入会话的中间件。
// 解析失败（无 Authorization 头 / 坏 token）直接放行：受保护路由由 go-zero
// jwt 中间件负责 401，公开路由（healthz/readyz/登录通道）不受影响。
// 链路顺序（go-zero rest engine 实际装配）：jwt 校验 → server.Use 中间件 → 本
// 中间件，即本中间件看到的 token 必已通过校验。
// typ=temp 的临时 token 被直接拒绝（401）：临时 token 只服务于 CAS 回调链路
// 向引擎取角色（60s TTL），不应访问任何业务接口。
func Identity(secret string) func(http.HandlerFunc) http.HandlerFunc {
	parser := token.NewTokenParser()
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			tok, err := parser.ParseToken(r, secret, "")
			if err != nil {
				next(w, r)
				return
			}
			claims, ok := tok.Claims.(jwt.MapClaims)
			if !ok {
				next(w, r)
				return
			}
			// 临时 token 拒绝：虽然签名与有效期均有效（jwt 层已放行），
			// 但它无角色、短 TTL，只用于取角色，不允许访问业务接口。
			if typ, _ := claims["typ"].(string); typ == tokenTypeTemp {
				kiterrors.Unauthorized("AUTH_TOKEN_TYPE_FORBIDDEN", "临时 token 不能访问业务接口").WriteJSON(w, r)
				return
			}
			// typ 缺省（旧 token/无 typ claim）视为 access 放行（向后兼容）。
			s := Session{
				Sub:    stringOf(claims["sub"]),
				Name:   stringOf(claims["name"]),
				Tenant: stringOf(claims["tenant"]),
				Roles:  stringSlice(claims["roles"]),
				Token:  bearerToken(r),
			}
			if s.Sub == "" {
				// 无 sub：不注入会话（下游按未认证拒绝）。
				next(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), keySession, s)
			// 同步把原始 token 注入 kit/authz 的 ctx 键：logic 经 kit/authz
			// Remote 调集中式 authz 时透传身份（Check/ListObjects/relations
			// 都需要 Authorization 头；Require 中间件之外的独立服务由本
			// 中间件完成 WithToken 职责）。
			ctx = WithToken(ctx, s.Token)
			next(w, r.WithContext(ctx))
		}
	}
}

// stringOf 断言 claim 为 string，否则返回空串。
func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

// stringSlice 把 JWT JSON 解出的 []any 归一化为 []string，非数组返回 nil。
func stringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// bearerToken 从 Authorization 头取原始 token：复用 middleware.go 的同一实现
//（去 "Bearer " 前缀、大小写不敏感、无前缀原样返回）。

// WithIdentity 向 ctx 注入身份（userID + roles），与 Identity 中间件写入的键
// 完全一致，供 logic 层单测构造带身份的 ctx（生产路径一律走 Identity 中间件）。
func WithIdentity(ctx context.Context, userID string, roles []string) context.Context {
	return WithIdentitySession(ctx, Session{Sub: userID, Roles: roles})
}

// WithIdentitySession 向 ctx 注入完整会话，与 Identity 中间件写入的键完全一致，
// 供单测构造带会话的 ctx（auth 迁移测试用）。
func WithIdentitySession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, keySession, s)
}

// IdentityFrom 从 ctx 取当前用户会话；ctx 中无会话时返回 ErrNoIdentity
//（Roles 可能为空切片，不是错误）。
func IdentityFrom(ctx context.Context) (Session, error) {
	s, ok := ctx.Value(keySession).(Session)
	if !ok || s.Sub == "" {
		return Session{}, ErrNoIdentity
	}
	return s, nil
}

// TestToken 用指定 secret 签发携带 sub 与 roles 的测试 JWT（HS256，1 小时过期），
// 仅供单测与本地路由验证使用，禁止用于生产。
func TestToken(secret, sub string, roles []string) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   sub,
		"roles": roles,
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	return tok.SignedString([]byte(secret))
}
