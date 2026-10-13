// BatchCheckLogic 承载批量权限判定接口的业务规则。
// 与 CheckLogic 同口径：user 一律从 JWT claims.sub 取；明确无权 → 对应
// 位置 false，整体 200 返回；引擎/身份错误 → 401/503 fail-closed。
// ABAC 叠加同 Check：ReBAC 通过后过属性策略。
package logic

import (
	"context"
	"strconv"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// BatchCheckLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type BatchCheckLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewBatchCheckLogic 构造业务规则层实例。
func NewBatchCheckLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckLogic {
	return &BatchCheckLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// BatchCheck 批量权限判定：
//   - identity 缺失 → 401 fail-closed；
//   - items 为空或任一条 object/relation 为空 → 400；
//   - 引擎 BatchCheck → 对每个 allowed=true 叠加 ABAC（deny 命中即 false）；
//   - 引擎 error → 503 AUTHZ_ENGINE_ERROR fail-closed。
func (l *BatchCheckLogic) BatchCheck(req *types.BatchCheckReq) (*types.BatchCheckResp, error) {
	sub, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, errors.BadRequest("AUTHZ_BATCH_CHECK_INVALID", "items 不能为空")
	}

	// 入参校验：逐条 trim + 非空。
	engItems := make([]engine.BatchCheckItem, 0, len(req.Items))
	for i, it := range req.Items {
		obj := strings.TrimSpace(it.Object)
		rel := strings.TrimSpace(it.Relation)
		if obj == "" || rel == "" {
			return nil, errors.BadRequest("AUTHZ_BATCH_CHECK_INVALID",
				"items["+strconv.Itoa(i)+"].object/relation 不能为空")
		}
		engItems = append(engItems, engine.BatchCheckItem{Object: obj, Relation: rel})
	}

	results, err := l.svcCtx.Engine.BatchCheck(l.ctx, "user:"+sub, engItems)
	if err != nil {
		l.Logger.Errorf("引擎批量判定失败 items=%d: %v", len(engItems), err)
		return nil, errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
	}

	// ABAC 叠加：对每个 ReBAC 判定通过的 item 再过属性策略。
	for i, allowed := range results {
		if allowed {
			obj := engItems[i].Object
			if idx := strings.IndexByte(obj, ':'); idx > 0 {
				if abacBlocked(l.ctx, l.svcCtx, obj[:idx], obj[idx+1:], sub) {
					results[i] = false
				}
			}
		}
	}

	return &types.BatchCheckResp{Results: results}, nil
}
