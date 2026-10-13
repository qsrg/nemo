// reconcileredislogic.go 承载 Redis 授权对账（补偿）逻辑：扫描 instances 与
// clusters 业务表，对"库中存在但引擎缺失 owner 元组"的孤儿资源（创建时元组
// 写入重试耗尽、或历史脏数据）按 created_by 批量补写 owner，恢复资源可管理性。
//
// 定位：跨服务写无法做本地事务，本端点是创建路径元组写入失败的最终兜底
// （重试 → 审计 failed → 对账补偿）。补写以**调用者身份**执行：缺失 owner 的
// 对象对 redis:manage 码持有者天然可管理（grantsManageable 管理码通道），
// 无需服务凭证。仅运维操作，manage 码守卫已上收路由；引擎故障 fail-closed。
package logic

import (
	"context"
	"fmt"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ReconcileRedisLogic 承载授权对账接口的业务规则。
type ReconcileRedisLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReconcileRedisLogic 构造对账业务规则层实例。
func NewReconcileRedisLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReconcileRedisLogic {
	return &ReconcileRedisLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ReconcileRedis 扫描 instances + clusters 并补写缺失的 owner 元组（manage
// 码判定已上收路由守卫，logic 只关注对账业务）。
//
// 流程：全量业务记录 → 逐对象 ListRelations(obj,"owner")：无 owner 且
// created_by 非空 → 记入待补清单（created_by 空无法推断 owner，记日志跳过
// 交人工）→ 批量 WriteRelations 补写。返回扫描总数与补写数。
func (l *ReconcileRedisLogic) ReconcileRedis() (*types.ReconcileResp, error) {
	instances, ierr := l.svcCtx.InstanceRepo.List(l.ctx)
	if ierr != nil {
		l.Logger.Errorf("reconcile: instances 表读取失败: %v", ierr)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "实例数据读取失败")
	}
	clusters, cerr := l.svcCtx.ClusterRepo.List(l.ctx)
	if cerr != nil {
		l.Logger.Errorf("reconcile: clusters 表读取失败: %v", cerr)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	type orphan struct {
		obj   string
		owner string
		kind  string
	}
	var pending []orphan
	scanned := 0

	// 创建失败的 target 集合（审计 failed）：这些 target 的业务行是"回滚失败
	// 的脏行"，对账**不得补 owner**（否则脏行被洗白成有效资源）——跳过并
	// 计入 skipped，交人工清理（与补写是两种相反的处置）。
	failedTargets, ferr := l.svcCtx.AuditRepo.ListFailedTargets(l.ctx,
		[]string{"create_instance", "create_cluster"})
	if ferr != nil {
		l.Logger.Errorf("reconcile: 失败审计读取失败: %v", ferr)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "审计数据读取失败")
	}
	skipped := 0

	// 实例：孤儿 = 无 owner 元组且 created_by 非空。
	for _, t := range instances {
		scanned++
		obj := instanceObject(t.ID)
		if strings.TrimSpace(t.CreatedBy) == "" {
			// 无创建人信息无法推断 owner，不能凭空指定——记录后交人工处理。
			l.Logger.Errorf("reconcile: instance 无 created_by，无法补 owner，请人工处理 %s", obj)
			continue
		}
		owners, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, obj, "owner")
		if rerr != nil {
			l.Logger.Errorf("reconcile: owner 元组读取失败 %s: %v", obj, rerr)
			return nil, instanceEngineErr()
		}
		if len(owners) == 0 {
			if failedTargets["instance:name:"+t.Name] || failedTargets[fmt.Sprintf("instance:id:%d", t.ID)] {
				skipped++ // 创建审计 failed = 回滚失败的脏行,交人工清理
				continue
			}
			pending = append(pending, orphan{obj: obj, owner: t.CreatedBy, kind: "instance"})
		}
	}

	// 集群：同模式。
	for _, c := range clusters {
		scanned++
		obj := clusterObject(c.ID)
		if strings.TrimSpace(c.CreatedBy) == "" {
			l.Logger.Errorf("reconcile: cluster 无 created_by，无法补 owner，请人工处理 %s", obj)
			continue
		}
		owners, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, obj, "owner")
		if rerr != nil {
			l.Logger.Errorf("reconcile: owner 元组读取失败 %s: %v", obj, rerr)
			return nil, instanceEngineErr()
		}
		if len(owners) == 0 {
			if failedTargets[fmt.Sprintf("cluster:code:%s", c.Code)] {
				skipped++ // 创建审计 failed = 回滚失败的脏行,交人工清理
				continue
			}
			pending = append(pending, orphan{obj: obj, owner: c.CreatedBy, kind: "cluster"})
		}
	}

	// 批量补写（引擎侧读后写幂等；并发对账竞态由幂等兜底）。
	repaired := 0
	for _, o := range pending {
		tuples := []authzkit.Tuple{{
			User: "user:" + o.owner, Relation: "owner", Object: o.obj,
		}}
		if _, werr := writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, tuples); werr != nil {
			l.Logger.Errorf("reconcile: 补写 owner 失败（留待下次对账）kind=%s %s: %v", o.kind, o.obj, werr)
			continue
		}
		l.Logger.Infof("reconcile: 已补写 owner kind=%s %s -> user:%s", o.kind, o.obj, o.owner)
		repaired++
	}
	return &types.ReconcileResp{Scanned: scanned, Repaired: repaired, Skipped: skipped}, nil
}
