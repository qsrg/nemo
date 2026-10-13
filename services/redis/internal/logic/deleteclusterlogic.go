// deleteclusterlogic.go 承载删除集群的业务规则：存在性 404、非空拒绝 409
// （集群下仍有实例时不允许删——实例归属完整性由应用层保证）、审计先行删库。
// 删除权判定已上收路由守卫（redis:manage 码），业务代码只关注业务。
package logic

import (
	"context"

	"nemo/kit/errors"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteClusterLogic 承载删除集群接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type DeleteClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteClusterLogic 构造业务规则层实例。
func NewDeleteClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteClusterLogic {
	return &DeleteClusterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteCluster 删除集群（写权限已上收路由守卫 redis:manage）：未命中 404；
// 集群下仍有实例 409（不级联删除、不悬空归属——先迁走/删实例再删集群）。
// 与实例删除不同，集群删除**非幂等**：未命中直接 404（集群无引擎元组，无
// 重复删除的自愈诉求，显式报错更利于前端排查）。
func (l *DeleteClusterLogic) DeleteCluster(req *types.DeleteClusterReq) error {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	id, err := parseClusterID(req.Id)
	if err != nil {
		return err
	}

	if _, gerr := l.svcCtx.ClusterRepo.GetById(l.ctx, id); gerr != nil {
		l.Logger.Errorf("集群不存在 id=%d: %v", id, gerr)
		return errors.NotFound("REDIS_CLUSTER_NOT_FOUND", "集群不存在")
	}
	n, cerr := l.svcCtx.InstanceRepo.CountByClusterID(l.ctx, id)
	if cerr != nil {
		l.Logger.Errorf("集群实例计数失败 id=%d: %v", id, cerr)
		return errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "实例数据读取失败")
	}
	if n > 0 {
		return errors.Conflict("REDIS_CLUSTER_NOT_EMPTY", "集群下仍有实例，请先删除或迁移")
	}

	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_cluster", clusterObject(id), req, func() error {
		// 第一步：删库（实例非空已在预检 409 拒绝——归属完整性应用层保证）。
		if derr := l.svcCtx.ClusterRepo.Delete(l.ctx, id); derr != nil {
			return derr
		}
		// 第二步：库已删，再清 redis_cluster:{id} 全部元组（owner/viewer 等）。
		// 读取/删除均带退避重试；任何失败都不改变"集群已删除"的结果，仅记录
		// 残留（运维可经 /v1/authz/relations 清理，同实例删除的容错口径）。
		tuples, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, clusterObject(id), "")
		if rerr != nil {
			l.Logger.Errorf("cluster 已删，元组读取失败（残留待清理）%s: %v", clusterObject(id), rerr)
			return nil
		}
		for _, t := range tuples {
			if derr := deleteRelationWithRetry(l.ctx, l.svcCtx.Authz, t.Object, t.User, t.Relation); derr != nil {
				l.Logger.Errorf("cluster 已删，元组清理失败（残留待清理）tuple=%+v: %v", t, derr)
			}
		}
		return nil
	})
}
