// createclusterlogic.go 承载新建集群的业务规则：code 必填（400）、重码 409
// （uk_code 唯一键兜底并发竞态）、审计先行写库。写权限已上收路由守卫
// （zookeeper:manage 码——集群管理档维护），业务代码只关注业务。
//
// 集群进权限引擎：创建时写 owner 元组 `user:{sub} owner zk_cluster:{id}`——
// 创建人（manage 码持有者）天然可管理该集群授权名单（grantsManageable 的
// manageCode 通道放行首个 owner 元组写入，zk_cluster.manage_code 已在注册
// 表声明）。
package logic

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/zookeeper/internal/repo"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CreateClusterLogic 承载新建集群接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateClusterLogic 构造业务规则层实例。
func NewCreateClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateClusterLogic {
	return &CreateClusterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateCluster 新建集群（写权限已上收路由守卫 zookeeper:manage）：code
// 必填（400）、重码 409（并发竞态由唯一键兜底——竞态路径经审计 failed
// 记录后按 503 返回，fail-closed 不产生半写状态）。集群进权限引擎：创建
// 时以**服务身份**写 owner 元组（真实创建人保留在 user 字段）。
func (l *CreateClusterLogic) CreateCluster(req *types.CreateClusterReq) (*types.ClusterResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, errors.BadRequest("ZK_CLUSTER_INVALID", "code 不能为空")
	}

	// 重码预检（uk_code 点查）。
	if _, derr := l.svcCtx.ClusterRepo.GetByCode(l.ctx, code); derr == nil {
		return nil, errors.Conflict("ZK_CLUSTER_EXISTS", "集群已存在: "+code)
	} else if !stderrors.Is(derr, repo.ErrNotFound) {
		l.Logger.Errorf("集群重码预检失败 code=%s: %v", code, derr)
		return nil, errors.New(503, "ZK_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	in := repo.Cluster{
		Code:      code,
		Hosts:     strings.TrimSpace(req.Hosts),
		Remark:    strings.TrimSpace(req.Remark),
		CreatedBy: operator,
		UpdateBy:  operator, // 审计字段（AGENTS.md 规范）：新建即最近更新人。
		CreatedAt: time.Now(),
	}
	var saved repo.Cluster
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_cluster", "cluster:code:"+code, req, func() error {
		created, werr := l.svcCtx.ClusterRepo.Create(l.ctx, in)
		if werr != nil {
			return werr
		}
		saved = created
		// owner 元组（创建人可管理该集群授权名单）。以调用者 token 写入：
		// 新集群首写 owner 由 authz 创建引导放行（空对象 + zookeeper:manage
		// 码）。重试耗尽则回滚业务行（消灭半写）。
		_, werr = writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, []authzkit.Tuple{{
			User: "user:" + operator, Relation: "owner", Object: clusterObject(created.ID),
		}})
		if werr != nil {
			if derr := l.svcCtx.ClusterRepo.Delete(l.ctx, created.ID); derr != nil {
				l.Logger.Errorf("owner 元组写失败且回滚失败（脏行待清理）id=%d: %v", created.ID, derr)
			} else {
				l.Logger.Errorf("owner 元组写失败，已回滚业务行 id=%d: %v", created.ID, werr)
			}
		}
		return werr
	})
	if err != nil {
		return nil, err
	}
	resp := clusterResp(saved)
	// 创建人即 owner：响应徽标即时正确（列表重新拉取时以引擎集合为准）。
	resp.Owner = true
	resp.CanManageGrants = true
	return &resp, nil
}
