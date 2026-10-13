// Package types 定义 zookeeper 服务的 API 契约类型（go-zero rest handler 的
// 请求/响应结构，JSON tag 全小驼峰，与前端 api/zookeeper.ts 一一对应）。
package types

// CreateClusterReq 是新建集群请求（POST /v1/zookeeper/clusters）。
type CreateClusterReq struct {
	// Code 集群编码（全表唯一，创建后不可改——对象身份不可变）。
	Code string `json:"code"`
	// Hosts ZK 集群连接串（host:port 逗号分隔；阶段一 mock 不消费，阶段二启用）。
	Hosts string `json:"hosts"`
	// Remark 备注。
	Remark string `json:"remark"`
}

// DeleteClusterReq 是删除集群请求（DELETE /v1/zookeeper/clusters/:id）。
type DeleteClusterReq struct {
	// Id 路径参数：集群 ID（字符串形态，前端 JSON 数字精度兼容）。
	Id string `path:"id"`
}

// ClusterResp 是集群行响应（选择器展示用；含行级徽标，与实例同口径）。
type ClusterResp struct {
	// Id 集群 ID（字符串形态）。
	Id string `json:"id"`
	// Code 集群编码。
	Code string `json:"code"`
	// Hosts ZK 集群连接串。
	Hosts string `json:"hosts"`
	// Remark 备注。
	Remark string `json:"remark"`
	// CreatedBy 创建人。
	CreatedBy string `json:"createdBy"`
	// UpdatedAt 最近更新时间。
	UpdatedAt string `json:"updatedAt"`
	// CanManageGrants 可配置集群授权名单（owner 关系 + 管理码回退，前端直接消费）。
	CanManageGrants bool `json:"canManageGrants"`
	// Owner 是否 owner 关系。
	Owner bool `json:"owner"`
}

// ClusterListResp 是集群列表响应（全量，不分页）。
type ClusterListResp struct {
	// Items 可见集群（id 升序；按引擎 can_view 过滤——看到集群才能浏览）。
	Items []ClusterResp `json:"items"`
}

// ListNodesReq 是节点浏览请求（GET /v1/zookeeper/clusters/:id/nodes）。
type ListNodesReq struct {
	// Id 路径参数：集群 ID（守卫按该集群 can_view 放行——看到集群才能浏览）。
	Id string `path:"id"`
}

// ZnodeResp 是单个 znode 节点（阶段一 mock 数据；阶段二由真实 ZK 读取）。
type ZnodeResp struct {
	// Path 节点绝对路径（如 /app/config）。
	Path string `json:"path"`
	// Name 节点名（路径末段；根为 "/"）。
	Name string `json:"name"`
	// Data 节点数据（演示文本）。
	Data string `json:"data"`
	// Children 子节点。
	Children []ZnodeResp `json:"children"`
}

// ZnodeTreeResp 是节点树响应（集群守卫通过后返回全树，只读浏览）。
type ZnodeTreeResp struct {
	// ClusterId 集群 ID（字符串形态）。
	ClusterId string `json:"clusterId"`
	// Code 集群编码。
	Code string `json:"code"`
	// Root 节点树根（/）。
	Root ZnodeResp `json:"root"`
}

// ReconcileResp 是授权对账响应（POST /v1/zookeeper/reconcile）。
type ReconcileResp struct {
	// Scanned 扫描的业务记录总数（clusters）。
	Scanned int `json:"scanned"`
	// Repaired 本次补写 owner 元组的数量（孤儿资源恢复可管理性）。
	Repaired int `json:"repaired"`
	// Skipped 跳过的数量：创建审计 failed 的 target（回滚失败的脏行）不补
	// owner——补写会把脏行洗白成有效资源，交人工清理。
	Skipped int `json:"skipped"`
}
