// cascallback.go 承载 CAS 回调编排与 token 签发（逻辑主体；生成的
// cascallbacklogic.go 仅薄委托到此）。
//
// 编排：ValidateTicket → 用户映射（未命中自动建档）→ 临时 token 向 authz 取
// 真实角色（fail-closed）→ 签发正式平台 JWT。
package logic

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/core/logx"

	"nemo/services/authz/internal/casclient"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// idpTypeCAS 是一期固定接入的身份源类型。
const idpTypeCAS = "cas"

// tenantPlatform 是一期固定租户（无多租户，仅配置注释说明）。
const tenantPlatform = "platform"

// tempTokenTTL 是临时 token 的有效期：仅够完成一次 authz 角色查询，
// 进一步压缩暴露窗口（安全性论证见 fetchPlatformRoles 注释）。
const tempTokenTTL = 60 * time.Second

// token 类型标记（claims.typ）：区分临时 token（temp，60s 短 TTL、无角色，
// 仅用于 authz 取角色）与正式平台 token（access）。临时 token 在 60s 窗口内
// 泄露时不带角色也取不到管理能力，但仍以显式 typ 便于消费端识别与审计。
const (
	tokenTypeTemp   = "temp"
	tokenTypeAccess = "access"
)

// casCallback 执行 CAS 回调编排：
//  1. 入参自校验（契约全 optional，鉴权入口不信任入参）；
//  2. ValidateTicket 换用户信息（401/503 语义由 CAS 客户端区分，直接透传）；
//  3. 按 (cas, username) 映射用户，未命中即首次登录自动建档；
//  4. 取平台角色，失败 503 fail-closed 拒发 token；
//  5. 签发正式平台 JWT。
func casCallback(ctx context.Context, sc *svc.ServiceContext, req *types.CasCallbackReq) (*types.TokenResp, error) {
	ticket := strings.TrimSpace(req.Ticket)
	service := strings.TrimSpace(req.Service)
	if ticket == "" || service == "" {
		return nil, kiterrors.BadRequest("AUTH_CAS_PARAMS_INVALID", "ticket 与 service 不能为空")
	}

	cu, err := sc.CasClient.ValidateTicket(ctx, ticket, service)
	if err != nil {
		// 客户端契约已区分 503（CAS 故障）与 401（票据无效），原样透传。
		return nil, err
	}
	if strings.TrimSpace(cu.Username) == "" {
		// success 分支但 user 为空的异常报文：按票据无效处理（fail-closed）。
		return nil, kiterrors.Unauthorized("AUTH_CAS_TICKET_INVALID", "CAS ticket 校验失败")
	}

	u, err := ensureUser(ctx, sc.Users, cu)
	if err != nil {
		return nil, err
	}

	roles, err := fetchPlatformRoles(ctx, sc, u)
	if err != nil {
		return nil, err
	}

	resp, err := IssueToken(ctx, sc, u, roles)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ensureUser 按 IdP 身份映射用户：未命中（首次登录）自动建档。ID 由存储层
// 自增生成（不再拼 "user:" 前缀——业务身份以 idp_subject 为准，authz 引擎
// 判定面自行补 "user:" 前缀）。展示资料以首次建档为准，后续登录不覆盖。
func ensureUser(ctx context.Context, users repo.UserRepo, cu casclient.CasUser) (*repo.User, error) {
	u, err := users.GetByIdp(ctx, idpTypeCAS, cu.Username)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, repo.ErrNotFound) {
		// 查询自身的非未命中错误（如存储故障）：500，原始 err 只进日志。
		logx.Errorf("查询用户失败 idp=%s subject=%s: %v", idpTypeCAS, cu.Username, err)
		return nil, kiterrors.Internal("AUTH_USER_QUERY_FAILED", "用户查询失败")
	}
	u, err = users.Create(ctx, repo.User{
		Name:       cu.Name,
		Email:      cu.Email,
		IdpType:    idpTypeCAS,
		IdpSubject: cu.Username,
	})
	if err != nil {
		logx.Errorf("首次登录建档失败 idp=%s subject=%s: %v", idpTypeCAS, cu.Username, err)
		return nil, kiterrors.Internal("AUTH_USER_CREATE_FAILED", "用户建档失败")
	}
	logx.Infof("CAS 首次登录自动建档 user=%s", u.IdpSubject)
	return u, nil
}

// fetchPlatformRoles 以临时 token 向 authz 取该用户的真实平台角色。
//
// 设计决策（鸡生蛋问题）：authz /v1/authz/list-objects 位于 jwt 组内需要有效
// JWT，而角色 claims 恰恰要在取到角色之后签发的正式 token 里才存在。解法：
// 先按用户身份签一个 60s 短 TTL 临时 token 调 authz 取角色，再签正式 token。
// 安全性论证：临时 token 的 claims 不带 roles——authz 管理面按角色判定一律
// 403，判定面 list-objects 只消费身份（claims.sub），即临时 token 取不到任何
// 管理能力，不构成提权面；60s TTL 进一步缩小暴露窗口。
//
// sub 用裸用户名（idp_subject）：authz 判定面约定自行在 claims.sub 前补
// "user:" 前缀（checklogic/listobjectslogic → 引擎查询 user 即
// "user:<cas username>"，与 bootstrap 超管 user:admin 同一命名空间）。临时
// token 与正式 token 的 sub 必须同一口径（裸用户名），任一带前缀都会被 authz
// 拼成 "user:user:<name>"，查错人导致静默 deny——口径一致性由单测
// TestCasCallbackSubConsistency 守护。
// fetchPlatformRoles 不再派生 super_admin 角色：去角色化后权限码是唯一真源，
// me/permissions 实时查引擎 permission.can，不再从 claims.roles 取任何
// 权限。roles claim 保留为空数组（前端兼容字段，不再承载权限语义）。
func fetchPlatformRoles(ctx context.Context, sc *svc.ServiceContext, u *repo.User) ([]string, error) {
	return []string{}, nil
}

// IssueToken 签发正式平台 JWT（HS256，claims 为与 go-zero rest jwt 兼容的
// map 形式 {sub,name,tenant,roles,typ,exp}）：sub = 裸用户名（IdpSubject，
// 与临时 token 同一口径，见 fetchPlatformRoles 注释——authz 判定点统一补
// "user:" 前缀）、tenant 一期固定 platform、typ = access（与 temp 临时 token
// 区分，见 tokenType 常量）、exp = now + AccessExpire（秒）。签名密钥与有效期
// 取 svcCtx 配置（Auth.AccessSecret / Auth.AccessExpire）。供 Task 3（刷新）
// 与 Task 5 复用。注：简报签名为 IssueToken(ctx, u, roles)，实现需读取签发
// 密钥，故显式增加 svcCtx 参数——依赖显式传入而非包级可变状态。
func IssueToken(_ context.Context, sc *svc.ServiceContext, u *repo.User, roles []string) (types.TokenResp, error) {
	if roles == nil {
		// 无角色用户也输出空数组（非 null），与前端约定一致。
		roles = []string{}
	}
	expiresAt := time.Now().Add(time.Duration(sc.Config.Auth.AccessExpire) * time.Second)
	token, err := signToken(sc.Config.Auth.AccessSecret, jwt.MapClaims{
		"sub":    u.IdpSubject,
		"name":   u.Name,
		"tenant": tenantPlatform,
		"roles":  roles,
		"typ":    tokenTypeAccess,
		"exp":    expiresAt.Unix(),
	})
	if err != nil {
		return types.TokenResp{}, err
	}
	return types.TokenResp{Token: token, ExpiresAt: expiresAt.Unix()}, nil
}

// signToken 以 HS256 签发 JWT（secret 由调用方传入）；签名失败归一化为 500
// 语义错误（原始 err 只进日志）。
func signToken(secret string, claims jwt.MapClaims) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		logx.Errorf("JWT 签名失败: %v", err)
		return "", kiterrors.Internal("AUTH_TOKEN_SIGN_FAILED", "token 签发失败")
	}
	return s, nil
}
