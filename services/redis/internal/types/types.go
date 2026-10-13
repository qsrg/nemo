// Package types 定义 redis 服务的 API 契约类型（go-zero rest handler 的
// 请求/响应结构，JSON tag 全小驼峰，与前端 api/redis.ts 一一对应）。
package types

// ListInstancesReq 是实例列表请求（GET /v1/redis/instances）。
type ListInstancesReq struct {
	// ClusterId 集群 ID（必填语义——先选集群再出实例；optional 仅为让"缺失"
	// 进入 logic 的 400 业务分支，而非 go-zero 解析层 500 兜底）。
	ClusterId int64 `form:"clusterId,optional"`
	// Page 页码，从 1 开始。
	Page int `form:"page"`
	// PageSize 每页条数。
	PageSize int `form:"pageSize"`
}

// CreateInstanceReq 是新建实例请求（POST /v1/redis/instances）。
type CreateInstanceReq struct {
	// ClusterId 归属集群 ID（字符串形态——全服务 ID 契约统一，前端 JSON
	// 数字精度兼容；须为已存在集群）。
	ClusterId string `json:"clusterId"`
	// Name 实例名（全表唯一）。
	Name string `json:"name"`
	// Addr 实例地址（host:port）。
	Addr string `json:"addr"`
	// Description 描述。
	Description string `json:"description"`
}

// UpdateInstanceReq 是更新实例请求（PUT /v1/redis/instances/:id；name 为
// 对象身份不可改，addr/description 全量覆盖）。
type UpdateInstanceReq struct {
	// Addr 实例地址。
	Addr string `json:"addr"`
	// Description 描述。
	Description string `json:"description"`
}

// DeleteInstanceReq 是删除实例请求（DELETE /v1/redis/instances/:id）。
type DeleteInstanceReq struct {
	// Id 路径参数：实例 ID（字符串形态，前端 JSON 数字精度兼容）。
	Id string `path:"id"`
}

// InstanceResp 是实例行响应（含逐条判定徽标；canManageGrants = owner 关系 +
// 管理码回退，与后端名单管理守卫同源，前端直接消费、不再用 owner 自行推导；
// owner 为纯 owner 关系，授权对话框初始态用）。
type InstanceResp struct {
	// Id 实例 ID（字符串形态）。
	Id string `json:"id"`
	// Name 实例名。
	Name string `json:"name"`
	// Addr 实例地址。
	Addr string `json:"addr"`
	// Description 描述。
	Description string `json:"description"`
	// CreatedBy 创建人。
	CreatedBy string `json:"createdBy"`
	// CanEdit 可编辑（owner or editor + 管理码回退）。
	CanEdit bool `json:"canEdit"`
	// CanDelete 可删除（owner + 管理码回退）。
	CanDelete bool `json:"canDelete"`
	// CanManageGrants 可配置授权名单（owner + 管理码回退）。
	CanManageGrants bool `json:"canManageGrants"`
	// Owner 是否 owner 关系。
	Owner bool `json:"owner"`
}

// InstanceListResp 是实例列表响应。
type InstanceListResp struct {
	// Items 当页实例行。
	Items []InstanceResp `json:"items"`
	// Total 满足谓词的近似总数（ABAC 复核过滤后页内可能少于 pageSize）。
	Total int `json:"total"`
}

// CreateClusterReq 是新建集群请求（POST /v1/redis/clusters）。
type CreateClusterReq struct {
	// Code 集群编码（全表唯一，创建后不可改——对象身份不可变）。
	Code string `json:"code"`
	// Remark 备注（用途/机房等说明）。
	Remark string `json:"remark"`
}

// DeleteClusterReq 是删除集群请求（DELETE /v1/redis/clusters/:id）。
type DeleteClusterReq struct {
	// Id 路径参数：集群 ID（字符串形态，前端 JSON 数字精度兼容）。
	Id string `path:"id"`
}

// ClusterResp 是集群行响应（选择器与列表展示用；含行级徽标，与实例同口径）。
type ClusterResp struct {
	// Id 集群 ID（字符串形态）。
	Id string `json:"id"`
	// Code 集群编码。
	Code string `json:"code"`
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
	// Items 全部集群（id 升序）。
	Items []ClusterResp `json:"items"`
}

// ReconcileResp 是授权对账响应（POST /v1/redis/reconcile）。
type ReconcileResp struct {
	// Scanned 扫描的业务记录总数（instances + clusters）。
	Scanned int `json:"scanned"`
	// Repaired 本次补写 owner 元组的数量（孤儿资源恢复可管理性）。
	Repaired int `json:"repaired"`
	// Skipped 跳过的数量：创建审计 failed 的 target（回滚失败的脏行）不补
	// owner——补写会把脏行洗白成有效资源，交人工清理。
	Skipped int `json:"skipped"`
}
