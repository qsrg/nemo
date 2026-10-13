// createapikey.go 承载 API key 签发逻辑（主体；生成的 createapikeylogic.go
// 仅薄委托到此）。
//
// 安全设计（spec 约束）：
//   - key 原文 nemo_<32hex>（crypto/rand 16 字节），只在签发响应里一次性
//     返回；repo 只落存 SHA-256(key) 摘要——服务端任何位置都拿不到原文；
//   - 签发入口仅超管权限持有者——路由侧 AdminRole 中间件前置拦截（查
//     permission:*.*#can），logic 层再引擎 Check 一次纵深防御，即便路由
//     装配回归也不构成提权面；
//   - Prefixes（scope 路由前缀白名单，permission-design §5.4）一期只存
//     不消费；列表/撤销/管理界面为 spec 排除项。
package logic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"

	authzkit "nemo/kit/authz"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// adminAllPerm 是超管全通配权限点对象（与 bootstrap AdminAllPerm 同源）：
// Check(user:{sub}, can, permission:*.*) 判定超管全权。
const adminAllPerm = "permission:*.*"

// apiKeyPrefix 是 key 原文的命名前缀（识别用途 + 避免与其他凭据混淆）。
const apiKeyPrefix = "nemo_"

// apiKeyIDPrefix 是 key 记录 ID 的前缀（区别于 user: 等其他命名空间）。
const apiKeyIDPrefix = "ak_"

// createApiKey 执行签发：引擎 Check 超管权限 → Name 必填 → 生成随机 key →
// 落存 SHA-256 摘要 → 一次性返回原文。
func createApiKey(ctx context.Context, sc *svc.ServiceContext, req *types.CreateApiKeyReq) (*types.ApiKeyResp, error) {
	// 纵深防御：路由已挂 AdminRole 中间件，logic 再引擎 Check 一次。
	s, err := authzkit.IdentityFrom(ctx)
	if err != nil {
		// 无会话（未经 Identity 中间件）：fail-closed 拒绝。
		return nil, kiterrors.Forbidden("AUTH_FORBIDDEN", "需要平台管理员权限")
	}
	allowed, err := sc.Engine.Check(ctx, "user:"+s.Sub, "can", adminAllPerm)
	if err != nil {
		logx.WithContext(ctx).Errorf("API key 签发权限判定失败 sub=%s: %v", s.Sub, err)
		return nil, kiterrors.New(503, "AUTH_ROLE_FETCH_FAILED", "授权引擎不可用")
	}
	if !allowed {
		return nil, kiterrors.Forbidden("AUTH_FORBIDDEN", "需要平台管理员权限")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, kiterrors.BadRequest("AUTH_NAME_REQUIRED", "API key 名称不能为空")
	}

	key, err := randomHex(apiKeyPrefix, 16) // 16 字节 → 32 hex 字符
	if err != nil {
		logx.WithContext(ctx).Errorf("生成 API key 失败: %v", err)
		return nil, kiterrors.Internal("AUTH_APIKEY_CREATE_FAILED", "API key 签发失败")
	}
	id, err := randomHex(apiKeyIDPrefix, 8)
	if err != nil {
		logx.WithContext(ctx).Errorf("生成 API key ID 失败: %v", err)
		return nil, kiterrors.Internal("AUTH_APIKEY_CREATE_FAILED", "API key 签发失败")
	}

	stored, err := sc.ApiKeys.Create(ctx, repo.ApiKey{
		ID:       id,
		Name:     name,
		Prefixes: req.Prefixes,
		KeyHash:  sha256Hex(key),
	})
	if err != nil {
		// 原始错误只进日志不外泄；key 原文绝不进日志。
		logx.WithContext(ctx).Errorf("落存 API key 失败 id=%s name=%s: %v", id, name, err)
		return nil, kiterrors.Internal("AUTH_APIKEY_CREATE_FAILED", "API key 签发失败")
	}
	logx.WithContext(ctx).Infof("签发 API key id=%s name=%s prefixes=%d sub=%s",
		stored.ID, stored.Name, len(stored.Prefixes), s.Sub)

	return &types.ApiKeyResp{Key: key, Id: stored.ID}, nil
}

// hasRole 已移除：去角色化后超管鉴权统一走引擎 Check(can, permission:*.*)。

// randomHex 生成 prefix + n 字节随机数的 hex 形态（2n 个 hex 字符），
// 随机源为 crypto/rand。
func randomHex(prefix string, n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

// sha256Hex 返回 s 的 SHA-256 hex 摘要。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
