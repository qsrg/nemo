// admincommon.go 承载组织/成员/平台角色写路径的公共设施：
// 审计先行封装（薄封装，三态模板上收 kit/audit）、当前操作人提取、组织
// 存在性判定、沿 parent 元组的防环遍历、内置平台角色白名单与组织 ID 生成。
// 全部为 logic 包内小函数，供各 logic 复用。
package logic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/audit"
	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// 内置平台角色白名单（简报固定值）：key → 展示名。
// 未注册角色一律 BadRequest("AUTHZ_ROLE_UNKNOWN","角色未定义")。
// 注意：super_admin 已移除——去角色化后超管鉴权统一走 permission:*.* 权限码，
// 不再依赖 platform_role 类型。operator/readonly 仍在平台角色体系中使用。
var builtinRoles = map[string]string{
	"operator": "操作员",
	"readonly": "只读用户",
}

// roleName 返回角色的展示名（白名单外返回空串）。
func roleName(key string) string { return builtinRoles[key] }

// operatorFrom 从 ctx 提取当前操作人（JWT claims.sub，经 authzkit.IdentityFrom）。
// ctx 无身份（未经 Identity 中间件的异常路径）返回 401 fail-closed。
func operatorFrom(ctx context.Context) (string, error) {
	sess, err := authzkit.IdentityFrom(ctx)
	if err != nil || strings.TrimSpace(sess.Sub) == "" {
		return "", errors.Unauthorized("AUTHZ_UNAUTHORIZED", "缺少用户身份")
	}
	return sess.Sub, nil
}

// writeWithAudit 是审计先行写路径的统一封装（语义逐字不变，三态模板上收
// kit/audit.WriteWithAudit——与 kafka 共用同一权威实现；本函数保留签名，
// 调用点零改动）：
//  1. AuditRepo.CreatePending 落 pending 条目（先于引擎写）；
//  2. 执行 write（对引擎的元组写/删与注册表落库）；
//  3. 成功 → MarkCommitted；失败 → MarkFailed（附原因）并按错误性质分类：
//     引擎校验拒绝（Invalid tuple）返回 400 AUTHZ_ENGINE_REJECTED 带真实
//     原因（请求/配置问题，管理面据此修正授权对象，无需对账）；其余按
//     503 AUTHZ_SERVICE_UNAVAILABLE fail-closed（操作未生效，已记录待对账）。
//
// 参数校验类错误（400/404/409）必须在进入本函数之前完成——本函数之后
// 的失败只可能来自引擎/存储故障。
func writeWithAudit(ctx context.Context, sc *svc.ServiceContext, operator, action, target string, snapshot any, write func() error) error {
	return audit.WriteWithAudit(ctx, auditSink{r: sc.AuditRepo}, operator, action, target, snapshot, write)
}

// auditSink 把本服务 AuditRepo 适配为 kit audit.Sink（字段映射；
// create_by 按 AGENTS.md 审计字段规范冗余为操作人）。
type auditSink struct {
	// r 本服务审计存取（内存/MySQL 双实现）。
	r repo.AuditRepo
}

// CreatePending 落一条 pending 审计并返回自增 ID（委托本服务 repo）。
func (s auditSink) CreatePending(ctx context.Context, e audit.Entry) (int64, error) {
	return s.r.CreatePending(ctx, repo.AuditEntry{
		Operator: e.Operator,
		CreateBy: e.Operator, // 审计字段规范：create_by = 操作人（写路径唯一入口，恒有身份）。
		Action:   e.Action,
		Target:   e.Target,
		Snapshot: e.Snapshot,
	})
}

// MarkCommitted 把审计条目标记为 committed。
func (s auditSink) MarkCommitted(ctx context.Context, id int64) error {
	return s.r.MarkCommitted(ctx, id)
}

// MarkFailed 把审计条目标记为 failed（附原因）。
func (s auditSink) MarkFailed(ctx context.Context, id int64, reason string) error {
	return s.r.MarkFailed(ctx, id, reason)
}

// orgExists 判定组织是否已存在：注册表命中，或引擎中存在以 org:{id} 为
// object 的任意元组（覆盖 bootstrap 直写引擎的 org:root 等）。
// 引擎读失败按错误透传（调用方转 503 fail-closed）。
func orgExists(ctx context.Context, sc *svc.ServiceContext, orgID string) (bool, error) {
	if _, err := sc.Orgs.Get(ctx, orgID); err == nil {
		return true, nil
	} else if !stderrors.Is(err, repo.ErrNotFound) {
		return false, fmt.Errorf("查组织注册表: %w", err)
	}
	tuples, err := sc.Engine.ReadTuples(ctx, "org:"+orgID, "")
	if err != nil {
		return false, fmt.Errorf("读组织元组: %w", err)
	}
	return len(tuples) > 0, nil
}

// ensureNoCycle 沿 parent 元组自 newParentID 向上遍历全部祖先链（BFS）：
// 模型 parent: [org] 为集合语义，一个组织可挂多条 parent 元组，故必须
// 遍历每条 parent 边并访问集去重——任一链见 orgID 自身即成环返回 true；
// 遍历到根或既有节点自然终止。引擎读失败按错误透传。
func ensureNoCycle(ctx context.Context, h *engine.Handle, orgID, newParentID string) (bool, error) {
	seen := map[string]bool{}
	queue := []string{newParentID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == "" {
			continue
		}
		if cur == orgID {
			return true, nil
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		tuples, err := h.ReadTuples(ctx, "org:"+cur, "parent")
		if err != nil {
			return false, fmt.Errorf("沿 parent 链向上遍历(%s): %w", cur, err)
		}
		// 全部 parent 边入队（多父场景任取一条会漏判环）。
		for _, t := range tuples {
			queue = append(queue, strings.TrimPrefix(t.User, "org:"))
		}
	}
	return false, nil
}

// newResourceID 生成带前缀的随机资源 ID（crypto/rand 8 字节 hex），
// 用于组织 ID 等管理面对象。生成失败按内部错误返回。
func newResourceID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成随机 ID: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(b), nil
}

// engineErrTo503 把引擎读路径错误（存在性判定/防环遍历等前置读）统一转
// 503 fail-closed：管理面写操作的前置读失败时宁可拒绝也不放行。
func engineErrTo503(err error) error {
	if err == nil {
		return nil
	}
	logx.Errorf("引擎前置读失败: %v", err)
	return errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "授权引擎不可用，操作被拒绝（fail-closed）")
}

// trimOrgID 归一化路径参数里的组织/用户 ID（去首尾空白）。
func trimOrgID(s string) string { return strings.TrimSpace(s) }
