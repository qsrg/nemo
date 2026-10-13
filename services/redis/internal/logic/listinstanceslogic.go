// listinstanceslogic.go 承载实例列表的业务规则：**纯引擎可见性口径**
// （与 kafka 的"列谓词 + 引擎复核"两段式不同，本服务不建归属列）：
//
//   - manage 码持有者（redis:manage）：全量翻页（无谓词）；
//   - 普通用户：list-objects 一次取回该用户在 redis_instance 上的 can_view
//     对象集合（owner/viewer/editor 元组经引擎模型的并集投影，含 ABAC），
//     作为 id IN (...) 谓词下推 SQL 分页——谓词量级随用户自己的关系数增长，
//     实例量级小（个位/百级），IN 全量下推无性能顾虑（见 repo/instance.go
//     注释：量级涨到几万再考虑列谓词/游标分页）；
//   - 页内行经 BatchCheck(can_view) 批量复核（真源 + ABAC 叠加，一次 RPC
//     替代逐行 Check）；复核后 total 为近似值（COUNT 是谓词口径），可接受。
//
// 授权服务故障 → 503 fail-closed（判定集合加载与批量复核任一失败整页拒绝）。
package logic

import (
	"context"
	"strconv"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/redis/internal/repo"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListInstancesLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListInstancesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListInstancesLogic 构造业务规则层实例。
func NewListInstancesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListInstancesLogic {
	return &ListInstancesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListInstances 实例列表（纯引擎可见性 + 批量复核）：
//   - 视角：redis:manage 持有者全量翻页（无谓词）；普通用户 can_view 集合
//     IN 下推（见文件头注释）；
//   - 徽标集合（owner/can_edit/can_manage_grants）同样经 list-objects 批量
//     取回，不逐行 Check；can_delete 与 can_edit 独立出口（can_delete=owner，
//     can_edit=editor），canManageGrants = owner + manage 码回退（与路由
//     守卫 requireInstanceManagePerm 同源，不含 editor）。
func (l *ListInstancesLogic) ListInstances(req *types.ListInstancesReq) (*types.InstanceListResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	// 集群维度必传（先选集群再出实例——列表恒在具体集群上下文内）。
	if req.ClusterId <= 0 {
		return nil, errors.BadRequest("REDIS_CLUSTER_REQUIRED", "请先选择集群")
	}
	manageOK, err := menuManageGranted(l.ctx, l.svcCtx.Authz, RedisManageCode)
	if err != nil {
		l.Logger.Errorf("manage 判定失败 sub=%s: %v", sub, err)
		return nil, instanceEngineErr()
	}

	// owner 徽标集合：owner 两种视角都需要（管理员未必是每条的 owner）；
	// editor 仅普通用户需要（manage 持有者 can_edit 恒真）。
	ownerSet, err := l.relationSet("owner")
	if err != nil {
		l.Logger.Errorf("owner 集合加载失败 sub=%s: %v", sub, err)
		return nil, instanceEngineErr()
	}
	var editSet map[string]bool
	if !manageOK {
		if editSet, err = l.relationSet("editor"); err != nil {
			l.Logger.Errorf("editor 集合加载失败 sub=%s: %v", sub, err)
			return nil, instanceEngineErr()
		}
	}

	// 可见性谓词（纯引擎）：普通用户取 can_view 对象集合，空集合 = 无任何
	// 可见实例（repo 谓词恒假返回空页，语义与内存实现一致）；集群维度恒下推
	//（与可见性谓词 AND 组合）。
	filter := repo.InstancePageFilter{All: manageOK, ClusterID: req.ClusterId, Page: req.Page, PageSize: req.PageSize}
	if !manageOK {
		viewSet, verr := l.relationSet("can_view")
		if verr != nil {
			l.Logger.Errorf("can_view 集合加载失败 sub=%s: %v", sub, verr)
			return nil, instanceEngineErr()
		}
		filter.IDs = parseInstanceObjectIDs(keysOf(viewSet))
	}

	instances, total, err := l.svcCtx.InstanceRepo.Page(l.ctx, filter)
	if err != nil {
		l.Logger.Errorf("实例分页查询失败: %v", err)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "实例数据读取失败")
	}

	// 引擎批量复核（真源 + ABAC）：普通用户 can_view 批量 Check 不过的行不
	// 展示；manage 持有者全权（含可见），跳过复核——与第一段 All=true 取页
	// 口径一致。用 BatchCheck 一次 RPC 替代 N 次逐行 Check。
	var viewResults []bool
	if !manageOK && len(instances) > 0 {
		batchItems := make([]authzkit.BatchCheckItem, 0, len(instances))
		for _, t := range instances {
			batchItems = append(batchItems, authzkit.BatchCheckItem{
				Object:   instanceObject(t.ID),
				Relation: "can_view",
			})
		}
		var berr error
		viewResults, berr = l.svcCtx.Authz.BatchCheck(l.ctx, batchItems)
		if berr != nil {
			l.Logger.Errorf("can_view 批量复核失败 instances=%d: %v", len(instances), berr)
			return nil, instanceEngineErr()
		}
	}

	items := make([]types.InstanceResp, 0, len(instances))
	for i, t := range instances {
		obj := instanceObject(t.ID)
		if !manageOK && !viewResults[i] {
			continue
		}
		// CanManageGrants 与路由守卫 requireInstanceManagePerm 同源：
		// can_manage_grants = owner + manage 码回退，不含 editor。
		canManageGrants := manageOK || ownerSet[obj]
		items = append(items, types.InstanceResp{
			Id:              strconv.FormatInt(t.ID, 10),
			Name:            t.Name,
			Addr:            t.Addr,
			Description:     t.Description,
			CreatedBy:       t.CreatedBy,
			CanEdit:         manageOK || ownerSet[obj] || editSet[obj],
			CanDelete:       manageOK || ownerSet[obj],
			CanManageGrants: canManageGrants,
			Owner:           ownerSet[obj],
		})
	}
	return &types.InstanceListResp{Items: items, Total: total}, nil
}

// relationSet 经 list-objects 批量取回当前用户在 redis_instance 上持有
// relation（owner/editor/can_view 等）的对象集合，返回以完整对象串为键的集合。
func (l *ListInstancesLogic) relationSet(relation string) (map[string]bool, error) {
	objects, err := l.svcCtx.Authz.ListObjects(l.ctx, "redis_instance", relation)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objects))
	for _, o := range objects {
		set[o] = true
	}
	return set, nil
}

// keysOf 取集合的全部键。
func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// parseInstanceObjectIDs 把 list-objects 返回的完整对象串（redis_instance:13）
// 解析为 int64 ID；前缀不符、非法或非正数的条目跳过。
func parseInstanceObjectIDs(objects []string) []int64 {
	ids := make([]int64, 0, len(objects))
	for _, o := range objects {
		raw, ok := strings.CutPrefix(o, "redis_instance:")
		if !ok {
			continue
		}
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}
