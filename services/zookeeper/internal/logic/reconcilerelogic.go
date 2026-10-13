// reconcilerelogic.go 承载 Zookeeper 授权对账（补偿）逻辑：扫描 clusters
// 业务表，对"库中存在但引擎缺失 owner 元组"的孤儿集群（创建时元组写入
// 重试耗尽、或历史脏数据）按 created_by 批量补写 owner，恢复资源可管理性。
//
// 定位：跨服务写无法做本地事务，本端点是创建路径 owner 写入失败的最终兜底
// （重试 → 审计 failed → 对账补偿）。补写以**调用者身份**执行：缺失 owner
// 的对象对 zookeeper:manage 码持有者天然可管理（grantsManageable 管理码
// 通道）。仅运维操作，manage 码守卫已上收路由；引擎故障 fail-closed。
package logic

import (
	"context"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ReconcileLogic 承载对账接口的业务规则。
type ReconcileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReconcileLogic 构造对账业务规则层实例。
func NewReconcileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReconcileLogic {
	return &ReconcileLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Reconcile 扫描全部集群并补写缺失的 owner 元组（manage 码判定已上收路由
// 守卫，logic 只关注对账业务）。返回扫描总数与补写数。
func (l *ReconcileLogic) Reconcile() (*types.ReconcileResp, error) {
	clusters, lerr := l.svcCtx.ClusterRepo.List(l.ctx)
	if lerr != nil {
		l.Logger.Errorf("reconcile: clusters 表读取失败: %v", lerr)
		return nil, errors.New(503, "ZK_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	resp := &types.ReconcileResp{Scanned: len(clusters)}
	pending := make([]authzkit.Tuple, 0)
	// 创建失败的 target 集合（审计 failed）：业务行是"回滚失败的脏行"，
	// 对账不得补 owner——跳过交人工清理。
	failedTargets, ferr := l.svcCtx.AuditRepo.ListFailedTargets(l.ctx, []string{"create_cluster"})
	if ferr != nil {
		l.Logger.Errorf("reconcile: 失败审计读取失败: %v", ferr)
		return nil, errors.New(503, "ZK_SERVICE_UNAVAILABLE", "审计数据读取失败")
	}
	skipped := 0
	for _, c := range clusters {
		obj := clusterObject(c.ID)
		if strings.TrimSpace(c.CreatedBy) == "" {
			// 无创建人信息无法推断 owner，不能凭空指定——记录后交人工处理。
			l.Logger.Errorf("reconcile: cluster 无 created_by，无法补 owner，请人工处理 %s", obj)
			continue
		}
		owners, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, obj, "owner")
		if rerr != nil {
			l.Logger.Errorf("reconcile: owner 元组读取失败 %s: %v", obj, rerr)
			return nil, clusterEngineErr()
		}
		if len(owners) == 0 {
			if failedTargets["cluster:code:"+c.Code] {
				skipped++ // 创建审计 failed = 回滚失败的脏行,交人工清理
				continue
			}
			pending = append(pending, authzkit.Tuple{
				User: "user:" + c.CreatedBy, Relation: "owner", Object: obj,
			})
		}
	}
	if len(pending) > 0 {
		if _, werr := writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, pending); werr != nil {
			l.Logger.Errorf("reconcile: 批量补写 owner 失败（留待下次对账）: %v", werr)
			return nil, errors.New(503, "ZK_SERVICE_UNAVAILABLE", "授权服务不可用，补偿未生效")
		}
		logx.WithContext(l.ctx).Infof("reconcile: 已补写 %d 条 owner 元组", len(pending))
	}
	resp.Skipped = skipped
	resp.Repaired = len(pending)
	return resp, nil
}
