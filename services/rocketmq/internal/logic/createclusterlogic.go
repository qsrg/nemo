// createclusterlogic.go 实现新建集群：编码非空校验、重名预检（409）+
// owner 元组双写（创建人可管理该集群授权名单）+ 失败回滚业务行。
// 守卫在路由层（管理码），logic 只做业务与存储。
package logic

import (
	"context"
	stdErrors "errors"
	"net/http"
	"strconv"
	"strings"

	authzkit "nemo/kit/authz"
	kiterrors "nemo/kit/errors"
	"nemo/services/rocketmq/internal/repo"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CreateClusterLogic 承载新建集群接口的业务规则。
type CreateClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateClusterLogic 构造新建集群业务规则实例。
func NewCreateClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateClusterLogic {
	return &CreateClusterLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// CreateCluster 新建集群：code 必填（trim 后非空）、重名 409；返回落库记录。
func (l *CreateClusterLogic) CreateCluster(req *types.CreateRocketmqClusterReq) (*types.RocketmqClusterResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, kiterrors.New(http.StatusBadRequest, "AUTHZ_OBJECT_INVALID", "集群编码不能为空")
	}
	if _, err := l.svcCtx.ClusterRepo.GetByCode(l.ctx, code); err == nil {
		return nil, kiterrors.New(http.StatusConflict, "CLUSTER_CODE_DUPLICATE", "集群编码已存在")
	} else if !stdErrors.Is(err, repo.ErrClusterNotFound) {
		l.Logger.Errorf("集群编码预检失败 code=%s: %v", code, err)
		return nil, kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	c, err := l.svcCtx.ClusterRepo.Create(l.ctx, repo.Cluster{
		Code:      code,
		Remark:    strings.TrimSpace(req.Remark),
		CreatedBy: sub,
		UpdateBy:  sub,
	})
	if err != nil {
		if stdErrors.Is(err, repo.ErrClusterDuplicate) {
			return nil, kiterrors.New(http.StatusConflict, "CLUSTER_CODE_DUPLICATE", "集群编码已存在")
		}
		l.Logger.Errorf("新建集群失败 code=%s: %v", code, err)
		return nil, kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "集群创建失败")
	}

	// owner 元组（创建人可管理该集群授权名单——「集群权限」入口的 owner 通道）。
	// 以调用者 token 写入：新集群首写 owner 由 authz 创建引导放行（空对象 +
	// 管理码，rmq_cluster.manage_code 已在资源类型注册表声明）。
	// 重试耗尽则回滚业务行（同 redis 实例口径，消灭半写）。
	if _, terr := writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, []authzkit.Tuple{{
		User: "user:" + sub, Relation: "owner", Object: rmqClusterObject(c.ID),
	}}); terr != nil {
		if derr := l.svcCtx.ClusterRepo.Delete(l.ctx, c.ID); derr != nil {
			l.Logger.Errorf("owner 元组写失败且回滚失败（脏行待清理）id=%d: %v", c.ID, derr)
		} else {
			l.Logger.Errorf("owner 元组写失败，已回滚业务行 id=%d: %v", c.ID, terr)
		}
		return nil, kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "集群创建失败")
	}

	return &types.RocketmqClusterResp{
		Id:              strconv.FormatInt(c.ID, 10),
		Code:            c.Code,
		Remark:          c.Remark,
		CreatedBy:       c.CreatedBy,
		UpdatedAt:       c.UpdatedAt.Format("2006-01-02 15:04:05"),
		CanManageGrants: true,
	}, nil
}
