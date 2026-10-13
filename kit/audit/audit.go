// Package audit 提供"审计先行"写路径模板（kit 侧）：先把 pending 审计条目
// 落库，再执行业务写闭包；成功置 committed，失败置 failed（附原因）并按约定
// 错误体返回，失败条目留给对账进程重放。
//
// 定位：authz 与 kafka（及后续业务服务）的授权/资源写路径共用同一套
// "审计先行 + 三态标记 + fail-closed"语义，本包是该语义的唯一权威实现，
// 服务侧 repo 经 Sink 适配接入（各服务审计模型字段在适配器内映射）。
//
// 职责边界（kit 纪律）：不含业务逻辑、不出现业务名词；错误统一经 kit/errors
// 约定体输出；不做运行时治理（重试由调用方负责，如 kafka 的 withRemoteRetry）。
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// Entry 是写路径审计条目的最小字段集（服务侧各自的审计模型经 Sink 适配
// 转换为本视图；create_by 按 AGENTS.md 审计字段规范冗余为 Operator，由
// 适配器填充）。
type Entry struct {
	// Operator 操作人（用户 ID，JWT sub）。
	Operator string
	// Action 动作（如 create_topic）。
	Action string
	// Target 操作对象。
	Target string
	// Snapshot 操作时的快照（JSON 文本）。
	Snapshot string
}

// Sink 是审计存储的最小接口：三方法与各服务 AuditRepo 的同名方法语义一致
// （CreatePending 落 pending 返回自增 ID；MarkCommitted/MarkFailed 置状态，
// 状态标记属系统行为，update_by 由实现置固定系统标识）。
type Sink interface {
	// CreatePending 写入一条 pending 审计并返回其 ID。
	CreatePending(ctx context.Context, e Entry) (int64, error)
	// MarkCommitted 把审计条目标记为 committed。
	MarkCommitted(ctx context.Context, id int64) error
	// MarkFailed 把审计条目标记为 failed（附原因）。
	MarkFailed(ctx context.Context, id int64, reason string) error
}

// WriteWithAudit 是审计先行写路径的统一模板：
//  1. CreatePending 落 pending 条目（先于业务写）→ 失败 500 AUTHZ_INTERNAL
//     （审计不可用时拒绝写操作，不留无痕变更）；
//  2. 执行 write（业务库写与/或授权元组写，由调用方闭包组装——跨服务无
//     本地事务，闭包内的执行顺序即补偿语义，由调用方注释钉死）；
//  3. 成功 → MarkCommitted（标记失败只影响审计展示、不回滚业务，记日志）；
//     失败 → MarkFailed（附原因，标记失败仅记日志）并分类返回：
//     - 引擎校验类拒绝（Invalid tuple）是请求/配置问题，返回 400
//     AUTHZ_ENGINE_REJECTED 带真实原因——管理面据此修正授权对象，无需对账；
//     - 其余（引擎/授权服务/存储故障）按 503 AUTHZ_SERVICE_UNAVAILABLE
//     fail-closed 返回（操作未生效，已记录待对账）。
//
// 参数校验类错误（400/404/409）必须在进入本函数之前完成——本函数之后的
// 失败只可能来自授权服务/存储故障。
func WriteWithAudit(ctx context.Context, sink Sink, operator, action, target string, snapshot any, write func() error) error {
	// 快照记录请求原文（JSON 文本），供对账与排查；不可序列化的入参兜底
	// 记录 marshal 错误，不让审计因快照失败而阻塞写路径。
	snap, err := json.Marshal(snapshot)
	if err != nil {
		snap = []byte(fmt.Sprintf(`{"marshal_error":%q}`, err.Error()))
	}
	id, err := sink.CreatePending(ctx, Entry{
		Operator: operator,
		Action:   action,
		Target:   target,
		Snapshot: string(snap),
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("审计 pending 落库失败 action=%s target=%s: %v", action, target, err)
		return kiterrors.New(http.StatusInternalServerError, "AUTHZ_INTERNAL", "审计落库失败")
	}

	if err := write(); err != nil {
		// 标记 failed（原因落库）后按错误性质分类返回；标记失败不阻断
		// 错误上抛（原始错误才是根因），仅记日志。
		if mErr := sink.MarkFailed(ctx, id, err.Error()); mErr != nil {
			logx.WithContext(ctx).Errorf("审计 failed 标记失败 id=%d: %v", id, mErr)
		}
		logx.WithContext(ctx).Errorf("写路径失败 action=%s target=%s: %v", action, target, err)
		if strings.Contains(err.Error(), "Invalid tuple") {
			return kiterrors.New(http.StatusBadRequest, "AUTHZ_ENGINE_REJECTED", "授权元组被引擎拒绝: "+err.Error())
		}
		return kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "授权服务不可用，操作未生效（已记录待对账）")
	}

	if err := sink.MarkCommitted(ctx, id); err != nil {
		// 提交已成功，标记失败只影响审计展示，不回滚业务，记日志即可。
		logx.WithContext(ctx).Errorf("审计 committed 标记失败 id=%d: %v", id, err)
	}
	return nil
}
