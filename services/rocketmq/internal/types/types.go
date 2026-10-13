// types.go 定义 rocketmq 独立服务的 HTTP 契约类型。
package types

// RocketmqTopic 单条 Topic 记录（业务数据）。
type RocketmqTopic struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Env         string `json:"env"`
	Description string `json:"description"`
	// ClusterId 归属集群（集群级授权：集群可见即该集群下 topic 全部可操作）。
	ClusterId int64  `json:"clusterId"`
	OrgId     string `json:"orgId"`
	CreatedBy string `json:"createdBy"`
}

// RocketmqTopicResp 单条 Topic 响应。集群级授权模型（2026-10-12 定稿）：
// 集群可见（rmq_cluster can_view）即该集群下 topic 全部可见且可操作，
// 故不再下发逐行 canEdit/canDelete 徽标——操作按钮随集群可见恒显示，
// 点击后由后端集群级守卫兜底（前端只做显隐，不做判定真源）。
type RocketmqTopicResp struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Env         string `json:"env"`
	Description string `json:"description"`
	ClusterId   string `json:"clusterId"`
	OrgId       string `json:"orgId"`
	CreatedBy   string `json:"createdBy"`
}

// RocketmqTopicListResp Topic 列表响应。
type RocketmqTopicListResp struct {
	Items []RocketmqTopicResp `json:"items"`
	Total int                 `json:"total"`
}

// ListRocketmqTopicsReq Topic 列表请求（clusterId 必传：先选集群再出列表；
// optional 仅把"未传"落到 logic 层做 400 参数校验，而非绑定期 500）。
type ListRocketmqTopicsReq struct {
	ClusterId int64 `form:"clusterId,optional"`
}

// CreateRocketmqTopicReq 新建 Topic 请求。
type CreateRocketmqTopicReq struct {
	Name        string `json:"name,optional"`
	Env         string `json:"env,optional"`
	Description string `json:"description,optional"`
	// ClusterId 归属集群（必传：topic 必须挂集群，集群级授权锚点）。
	ClusterId int64 `json:"clusterId"`
	// OrgId 归属组织（可选）：指定则该组织（含子树成员）可见本 Topic。
	OrgId string `json:"orgId,optional"`
}

// UpdateRocketmqTopicReq 更新 Topic 请求（name 不可改：对象身份锚点）。
type UpdateRocketmqTopicReq struct {
	Id          string `path:"id"`
	Env         string `json:"env,optional"`
	Description string `json:"description,optional"`
}

// DeleteRocketmqTopicReq 删除 Topic 请求（Id 取路径末段）。
type DeleteRocketmqTopicReq struct {
	Id string `path:"id"`
}

// CreateRocketmqClusterReq 新建集群请求（code 集群编码必填、业务唯一）。
type CreateRocketmqClusterReq struct {
	Code   string `json:"code,optional"`
	Remark string `json:"remark,optional"`
}

// DeleteRocketmqClusterReq 删除集群请求（Id 取路径末段）。
type DeleteRocketmqClusterReq struct {
	Id int64 `path:"id"`
}

// RocketmqClusterResp 单条集群响应（canManageGrants 徽标：管理码持有者或
// 该集群 owner 才有「集群权限」配置入口，前端直接消费不重复判定）。
type RocketmqClusterResp struct {
	Id              string `json:"id"`
	Code            string `json:"code"`
	Remark          string `json:"remark"`
	CreatedBy       string `json:"createdBy"`
	UpdatedAt       string `json:"updatedAt"`
	CanManageGrants bool   `json:"canManageGrants"`
}

// RocketmqClusterListResp 集群列表响应（全量不分页：选择器场景量级小）。
type RocketmqClusterListResp struct {
	Items []RocketmqClusterResp `json:"items"`
}

// ReconcileRocketmqTopicsResp 对账响应：Scanned 业务表扫描行数；
// Repaired 本次补写的元组数（owner 缺失补创建人 + org 归属缺失补业务行 org_id）。
type ReconcileRocketmqTopicsResp struct {
	Scanned  int `json:"scanned"`
	Repaired int `json:"repaired"`
}
