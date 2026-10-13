// types.go 定义 kafka 独立服务的 HTTP 契约类型（与原 authz 内嵌 kafka
// 路由的 API 契约逐字一致，前端 apps/kafka 无需改动即可切换）。
package types

// CreateKafkaTopicReq 新建 Topic 请求。
type CreateKafkaTopicReq struct {
	Name        string `json:"name,optional"`
	Env         string `json:"env,optional"`
	Description string `json:"description,optional"`
	// OrgId 归属组织（可选）：指定则该组织（含子树成员）可见本 Topic，
	// 业务表 org_id 列与引擎 org 元组一致取该值；缺省回退创建人所在部门
	// （与存量行为一致）。
	OrgId string `json:"orgId,optional"`
}

// DeleteTopicReq 删除 Topic 请求（Id 取路径末段）。
type DeleteTopicReq struct {
	Id string `path:"id"`
}

// KafkaTopicListResp Topic 列表响应。
type KafkaTopicListResp struct {
	Items []KafkaTopicResp `json:"items"`
	Total int              `json:"total"`
}

// KafkaTopicResp 单条 Topic 响应（权限徽标为当前用户视角的能力字段，
// 与路由守卫语义同源：CanEdit/CanDelete 对应 can_edit/owner 关系 + 管理码回退；
// CanManageGrants 对应 owner 关系 + 管理码回退，与名单管理守卫同源；
// Owner 保留供"纯 owner 关系"场景使用）。
type KafkaTopicResp struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	Env             string `json:"env"`
	Description     string `json:"description"`
	OrgId           string `json:"orgId"`
	CreatedBy       string `json:"createdBy"`
	CanEdit         bool   `json:"canEdit"`
	CanDelete       bool   `json:"canDelete"`
	CanManageGrants bool   `json:"canManageGrants"`
	// CanView 行级只读出口（can_view，manage 码回退）：前端按菜单声明的
	// 按钮 relation 与 canX 字段匹配决定按钮显隐（声明驱动渲染）。
	CanView bool `json:"canView"`
	Owner   bool `json:"owner"`
}

// ListTopicsReq 列表请求（可见性过滤在 logic 层：列谓词分页 + 引擎逐行复核）。
type ListTopicsReq struct {
	Page     int `form:"page,optional"`
	PageSize int `form:"pageSize,optional"`
}

// UpdateKafkaTopicReq 更新 Topic 请求（name 不可改：对象身份锚点）。
type UpdateKafkaTopicReq struct {
	Id          string `path:"id"`
	Env         string `json:"env,optional"`
	Description string `json:"description,optional"`
}

// TopicGrantIdReq 撤销授予请求（目标与关系取自路径）。
type TopicGrantIdReq struct {
	Id         string `path:"id"`
	TargetType string `path:"targetType"`
	TargetId   string `path:"targetId"`
}

// TopicGrantReq 授予请求。
type TopicGrantReq struct {
	Id         string `path:"id"`
	TargetType string `json:"targetType,optional"`
	TargetId   string `json:"targetId,optional"`
	Relation   string `json:"relation,optional"`
}

// TopicGrantResp 授予记录（目标 + 关系三元组）。
type TopicGrantResp struct {
	TargetType string `json:"targetType"`
	TargetId   string `json:"targetId"`
	Relation   string `json:"relation"`
}

// TopicGrantsResp 授予列表响应。
type TopicGrantsResp struct {
	Items []TopicGrantResp `json:"items"`
	Total int              `json:"total"`
}

// TopicConsumerGroupResp 消费者组信息（生产环境来自 Kafka AdminClient
// DescribeConsumerGroups/ListConsumerGroupOffsets；demo 无集群时为空列表）。
type TopicConsumerGroupResp struct {
	GroupId string `json:"groupId"`
	State   string `json:"state"`
	Members int32  `json:"members"`
	Lag     int64  `json:"lag"`
}

// GetTopicConsumersResp 查看某 Topic 消费者组的响应。
type GetTopicConsumersResp struct {
	TopicId string                   `json:"topicId"`
	Items   []TopicConsumerGroupResp `json:"items"`
}

// TopicIdReq 按 ID 定位 Topic 的请求（名单查看/查看消费者）。
type TopicIdReq struct {
	Id string `path:"id"`
}

// ExportTopicsReq 导出 Topic 清单请求（无参数：导出范围为调用者可见行）。
type ExportTopicsReq struct{}

// ResetOffsetResp 重置位点响应（demo 语义：确认动作已受理）。
type ResetOffsetResp struct {
	Id    string `json:"id"`
	Reset bool   `json:"reset"`
}

// ReconcileResp 是 Topic 授权对账结果（POST /v1/kafka/reconcile）。
type ReconcileResp struct {
	// Scanned 扫描的 Topic 总数。
	Scanned int `json:"scanned"`
	// Repaired 补写缺失 owner 元组的 Topic 数。
	Repaired int `json:"repaired"`
}

// CreateClusterReq 新建集群请求（code 集群编码必填、业务唯一）。
type CreateClusterReq struct {
	Code    string `json:"code,optional"`
	Dc      string `json:"dc,optional"`
	Address string `json:"address,optional"`
	Remark  string `json:"remark,optional"`
}

// UpdateClusterReq 编辑集群请求（Id 取路径；code 为对象身份不可改，只改属性）。
type UpdateClusterReq struct {
	Id      int64  `path:"id"`
	Dc      string `json:"dc,optional"`
	Address string `json:"address,optional"`
	Remark  string `json:"remark,optional"`
}

// ClusterIdReq 集群 ID 路径参数（删除用）。
type ClusterIdReq struct {
	Id int64 `path:"id"`
}

// ListClustersReq 集群分页查询请求。
type ListClustersReq struct {
	Page     int `form:"page,optional"`
	PageSize int `form:"pageSize,optional"`
}

// ClusterResp 单条集群响应。
type ClusterResp struct {
	Id        int64  `json:"id"`
	Code      string `json:"code"`
	Dc        string `json:"dc"`
	Address   string `json:"address"`
	Remark    string `json:"remark"`
	CreatedBy string `json:"createdBy"`
	UpdatedAt string `json:"updatedAt"`
}

// ClusterListResp 集群分页结果。
type ClusterListResp struct {
	Items []ClusterResp `json:"items"`
	Total int64         `json:"total"`
}
