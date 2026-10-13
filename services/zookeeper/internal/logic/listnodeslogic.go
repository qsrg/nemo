// listnodeslogic.go 承载节点浏览的业务规则：返回指定集群的 znode 树
// （只读浏览）。集群可见性已上收路由守卫（zk_cluster:can_view 主判定 +
// zookeeper:manage 回退——"看到集群才能浏览节点"，与集群列表同一口径）。
//
// 阶段一（本实现）：znode 数据为**确定性 mock 树**（不连真实 ZK，所有
// 集群同一演示结构）——接口签名即生产形态，阶段二按 clusters.hosts 连接
// 真实 ZK（go-zookeeper/zk）替换 mock 树实现，路由/守卫/前端不变。
package logic

import (
	"context"

	"nemo/kit/errors"
	"nemo/services/zookeeper/internal/svc"
	"nemo/services/zookeeper/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListNodesLogic 承载节点浏览接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListNodesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListNodesLogic 构造业务规则层实例。
func NewListNodesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListNodesLogic {
	return &ListNodesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// mockZnode 是 mock 演示树的静态节点定义（children 递归）。
type mockZnode struct {
	path string
	data string
	ch   []mockZnode
}

// mockTree 是确定性演示树（与真实 ZK 的常规结构对齐：/app、/brokers、
// /zookeeper 等），阶段二替换为真实连接读取。
var mockTree = mockZnode{
	path: "/", data: "",
	ch: []mockZnode{
		{path: "/app", data: "app root", ch: []mockZnode{
			{path: "/app/config", data: "db.url=jdbc:mysql://127.0.0.1:3306/biz\npool.size=20"},
			{path: "/app/queue", data: "consumers=[svc-order,svc-pay]"},
			{path: "/app/locks", data: ""},
		}},
		{path: "/brokers", data: "", ch: []mockZnode{
			{path: "/brokers/ids", data: "", ch: []mockZnode{
				{path: "/brokers/ids/1", data: "{\"host\":\"10.0.1.11\",\"port\":9092}"},
				{path: "/brokers/ids/2", data: "{\"host\":\"10.0.1.12\",\"port\":9092}"},
			}},
			{path: "/brokers/topics", data: "order-events,payment-events"},
		}},
		{path: "/zookeeper", data: "", ch: []mockZnode{
			{path: "/zookeeper/quota", data: ""},
			{path: "/zookeeper/config", data: "version=0"},
		}},
	},
}

// ListNodes 返回集群的 znode 树（集群可见性已由路由守卫判定；本方法只做
// 存在性校验 + mock 树组装）。
func (l *ListNodesLogic) ListNodes(req *types.ListNodesReq) (*types.ZnodeTreeResp, error) {
	id, err := parseClusterID(req.Id)
	if err != nil {
		return nil, err
	}
	c, gerr := l.svcCtx.ClusterRepo.GetById(l.ctx, id)
	if gerr != nil {
		l.Logger.Errorf("集群不存在 id=%d: %v", id, gerr)
		return nil, errors.NotFound("ZK_CLUSTER_NOT_FOUND", "集群不存在")
	}

	root := buildZnode(mockTree)
	return &types.ZnodeTreeResp{
		ClusterId: clusterResp(c).Id,
		Code:      c.Code,
		Root:      root,
	}, nil
}

// buildZnode 把静态 mock 定义转为 API 树（含 name 派生）。
func buildZnode(m mockZnode) types.ZnodeResp {
	name := m.path
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			name = name[i+1:]
			break
		}
	}
	if m.path == "/" {
		name = "/"
	}
	out := types.ZnodeResp{Path: m.path, Name: name, Data: m.data}
	out.Children = make([]types.ZnodeResp, 0, len(m.ch))
	for _, ch := range m.ch {
		out.Children = append(out.Children, buildZnode(ch))
	}
	return out
}
