// createclusterlogic.go 新建集群（POST /v1/kafka/clusters）。
// 新建权判定在路由守卫（requireClusterManageCode：kafka-clusters:manage），
// logic 只做业务：code 必填、重名 409（DB 唯一键兜底）、审计先行。
package logic

import (
	"context"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// CreateClusterLogic 承载集群新建接口的业务规则。
type CreateClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateClusterLogic 构造集群新建业务规则层实例。
func NewCreateClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateClusterLogic {
	return &CreateClusterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateCluster 新建集群：code（集群编码）必填且唯一（uk_code 兜底并发
// 竞态），重名 409；写库走审计先行（writeWithAudit）。
func (l *CreateClusterLogic) CreateCluster(req *types.CreateClusterReq) (*types.ClusterResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, errors.BadRequest("AUTHZ_CLUSTER_INVALID", "集群编码不能为空")
	}

	// 重名预检（uk_code 点查；并发竞态由唯一键兜底）。
	if _, derr := l.svcCtx.ClusterRepo.GetByCode(l.ctx, code); derr == nil {
		return nil, errors.Conflict("AUTHZ_CLUSTER_EXISTS", "集群编码已存在: "+code)
	} else if !isNotFound(derr) {
		l.Logger.Errorf("重名预检失败 code=%s: %v", code, derr)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	in := repo.Cluster{
		Code:      code,
		Dc:        strings.TrimSpace(req.Dc),
		Address:   strings.TrimSpace(req.Address),
		Remark:    strings.TrimSpace(req.Remark),
		CreatedBy: operator,
		UpdateBy:  operator, // 审计字段：新建即最近更新人。
		CreatedAt: time.Now(),
	}
	var saved repo.Cluster
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_cluster", "cluster:code:"+code, req, func() error {
		created, werr := l.svcCtx.ClusterRepo.Create(l.ctx, in)
		if werr != nil {
			return werr
		}
		saved = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return clusterResp(saved), nil
}
