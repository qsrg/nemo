// listclusterlogic.go 集群列表（GET /v1/kafka/clusters）。
// 读取无需守卫：页面入口由菜单码 kafka-clusters:read 控制（me/menus 过滤），
// 列表接口本身不重复校验（与 topics 列表同口径）。
package logic

import (
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"
)

// ListClusterLogic 承载集群列表接口的业务规则。
type ListClusterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListClusterLogic 构造集群列表业务规则层实例。
func NewListClusterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListClusterLogic {
	return &ListClusterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListCluster 分页返回集群列表（id 降序，新集群在前）；页码越界返回空列表。
// 存储故障统一 503 fail-closed（列表读取失败不静默降级为空表）。
func (l *ListClusterLogic) ListCluster(req *types.ListClustersReq) (*types.ClusterListResp, error) {
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	items, total, err := l.svcCtx.ClusterRepo.Page(l.ctx, page, pageSize)
	if err != nil {
		l.Logger.Errorf("集群列表查询失败 page=%d pageSize=%d: %v", page, pageSize, err)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}
	resp := &types.ClusterListResp{Items: make([]types.ClusterResp, 0, len(items)), Total: total}
	for _, c := range items {
		resp.Items = append(resp.Items, types.ClusterResp{
			Id:        c.ID,
			Code:      c.Code,
			Dc:        c.Dc,
			Address:   c.Address,
			Remark:    c.Remark,
			CreatedBy: c.CreatedBy,
			UpdatedAt: c.UpdatedAt.Format(time.DateTime),
		})
	}
	return resp, nil
}
