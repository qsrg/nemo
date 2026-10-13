// listclusterslogic.go 承载集群列表的业务规则：**双通道可见性**（集群已进
// 权限引擎，2026-09-30 起）——
//   - redis:manage 码持有者：全量可见（管理档维护视角，含空集群）；
//   - 普通用户：**引擎授权 ∪ 实例派生**两个通道取并集——
//     ① 引擎通道：ListObjects(redis_cluster, can_view)（owner 派生 can_view，
//     或被显式授予 viewer——空集群也能授权给指定用户，无需先造实例）；
//     ② 实例派生通道：可见实例（can_view redis_instance）归属的集群并集
//     （向后兼容：实例行内授权依然带出所属集群）。
//   - 行级徽标（owner/canManageGrants）批量取回，前端控制「集群权限」入口。
//
// 引擎/存储故障一律 503 fail-closed（与实例列表同口径）。
package logic

import (
	"context"
	"strings"

	"nemo/kit/errors"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

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

// ListClusters 集群列表（管理档全量；普通用户引擎授权 ∪ 实例派生双通道）。
func (l *ListClustersLogic) ListClusters() (*types.ClusterListResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	clusters, err := l.svcCtx.ClusterRepo.List(l.ctx)
	if err != nil {
		l.Logger.Errorf("集群列表读取失败: %v", err)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "集群数据读取失败")
	}

	// 管理档全量（含空集群）；普通用户走双通道过滤。
	manageOK, err := menuManageGranted(l.ctx, l.svcCtx.Authz, RedisManageCode)
	if err != nil {
		l.Logger.Errorf("manage 判定失败 sub=%s: %v", sub, err)
		return nil, instanceEngineErr()
	}

	// owner 徽标集合（manage/普通视角都需要：owner = 名单管理权，行内
	// 「集群权限」入口直接消费，不重复判定）。
	ownerSet, err := l.clusterSet("owner")
	if err != nil {
		l.Logger.Errorf("owner 集合加载失败 sub=%s: %v", sub, err)
		return nil, instanceEngineErr()
	}

	visibleClusterIDs := map[int64]bool{}
	if !manageOK {
		// ① 引擎通道：redis_cluster 的 can_view 对象集（owner 派生 + 显式 viewer）。
		viewObjects, lerr := l.svcCtx.Authz.ListObjects(l.ctx, "redis_cluster", "can_view")
		if lerr != nil {
			l.Logger.Errorf("集群 can_view 集合加载失败 sub=%s: %v", sub, lerr)
			return nil, instanceEngineErr()
		}
		for _, o := range viewObjects {
			if id, ok := parseClusterObjectID(o); ok {
				visibleClusterIDs[id] = true
			}
		}
		// ② 实例派生通道：可见实例归属的集群并集。
		instanceObjects, ierr := l.svcCtx.Authz.ListObjects(l.ctx, "redis_instance", "can_view")
		if ierr != nil {
			l.Logger.Errorf("实例 can_view 集合加载失败 sub=%s: %v", sub, ierr)
			return nil, instanceEngineErr()
		}
		clusterIDs, cerr := l.svcCtx.InstanceRepo.ClusterIDsByInstanceIDs(l.ctx, parseInstanceObjectIDs(instanceObjects))
		if cerr != nil {
			l.Logger.Errorf("可见实例集群归集失败 sub=%s: %v", sub, cerr)
			return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "集群数据读取失败")
		}
		for _, id := range clusterIDs {
			visibleClusterIDs[id] = true
		}
	}

	items := make([]types.ClusterResp, 0, len(clusters))
	for _, c := range clusters {
		if !manageOK && !visibleClusterIDs[c.ID] {
			continue // 双通道皆不可见 → 对该用户不存在
		}
		obj := clusterObject(c.ID)
		row := clusterResp(c)
		row.CanManageGrants = manageOK || ownerSet[obj]
		row.Owner = ownerSet[obj]
		items = append(items, row)
	}
	return &types.ClusterListResp{Items: items}, nil
}

// clusterSet 经 list-objects 批量取回当前用户在 redis_cluster 上持有 relation
// 的对象集合，返回以完整对象串为键的集合（与 ListInstancesLogic.relationSet 同构）。
func (l *ListClustersLogic) clusterSet(relation string) (map[string]bool, error) {
	objects, err := l.svcCtx.Authz.ListObjects(l.ctx, "redis_cluster", relation)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objects))
	for _, o := range objects {
		set[o] = true
	}
	return set, nil
}

// parseClusterObjectID 把 list-objects 返回的完整对象串（redis_cluster:3）
// 解析为 int64 ID；前缀不符、非法或非正数的条目跳过。
func parseClusterObjectID(o string) (int64, bool) {
	raw, ok := strings.CutPrefix(o, "redis_cluster:")
	if !ok {
		return 0, false
	}
	var id int64
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
		id = id*10 + int64(r-'0')
	}
	if id <= 0 {
		return 0, false
	}
	return id, true
}
