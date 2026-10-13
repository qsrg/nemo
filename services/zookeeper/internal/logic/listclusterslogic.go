// listclusterslogic.go 承载集群列表的业务规则：**纯引擎可见性**——
//   - zookeeper:manage 码持有者：全量可见（管理档维护视角，含空集群）；
//   - 普通用户：ListObjects(zk_cluster, can_view)（owner 派生 + 显式
//     viewer，空集群也可授权）——"看到集群才能浏览节点"：节点浏览接口
//     按同一口径守卫，列表与节点判定同源。
//
// 行级徽标（owner/canManageGrants）批量取回，前端控制「集群权限」入口。
// 引擎/存储故障一律 503 fail-closed。
package logic

import (
	"context"
	"strconv"
	"strings"

	"nemo/kit/errors"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListClustersLogic 承载集群列表接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListClustersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListClustersLogic 构造业务规则层实例。
func NewListClustersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListClustersLogic {
	return &ListClustersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListClusters 集群列表（管理档全量；普通用户引擎 can_view 过滤）。
func (l *ListClustersLogic) ListClusters() (*types.ClusterListResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	clusters, err := l.svcCtx.ClusterRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("集群列表读取失败: %v", err)
		return nil, errors.New(503, "ZK_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	manageOK, err := menuManageGranted(l.ctx, l.svcCtx.Authz, ZookeeperManageCode)
	if err != nil {
		l.Logger.Errorf("manage 判定失败 sub=%s: %v", sub, err)
		return nil, clusterEngineErr()
	}

	// owner 徽标集合（manage/普通视角都需要：owner = 名单管理权，行内
	// 「集群权限」入口直接消费，不重复判定）。
	ownerObjects, oerr := l.svcCtx.Authz.ListObjects(l.ctx, "zk_cluster", "owner")
	if oerr != nil {
		l.Logger.Errorf("owner 集合加载失败 sub=%s: %v", sub, oerr)
		return nil, clusterEngineErr()
	}
	ownerSet := make(map[string]bool, len(ownerObjects))
	for _, o := range ownerObjects {
		ownerSet[o] = true
	}

	// 普通用户：引擎 can_view 对象集过滤（manage 持有者全量）。
	visible := map[int64]bool{}
	if !manageOK {
		viewObjects, verr := l.svcCtx.Authz.ListObjects(l.ctx, "zk_cluster", "can_view")
		if verr != nil {
			l.Logger.Errorf("can_view 集合加载失败 sub=%s: %v", sub, verr)
			return nil, clusterEngineErr()
		}
		for _, o := range viewObjects {
			if raw, ok := strings.CutPrefix(o, "zk_cluster:"); ok {
				if id, perr := strconv.ParseInt(raw, 10, 64); perr == nil && id > 0 {
					visible[id] = true
				}
			}
		}
	}

	items := make([]types.ClusterResp, 0, len(clusters))
	for _, c := range clusters {
		if !manageOK && !visible[c.ID] {
			continue // 引擎不可见 → 对该用户不存在
		}
		obj := clusterObject(c.ID)
		row := clusterResp(c)
		row.CanManageGrants = manageOK || ownerSet[obj]
		row.Owner = ownerSet[obj]
		items = append(items, row)
	}
	return &types.ClusterListResp{Items: items}, nil
}
