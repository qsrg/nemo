// instance.go 定义 Redis 实例的 repo 接口与模型。
// instances 表是业务属性的事实源；关系（owner/viewer/editor）存引擎，不在此表
// ——两份数据由 instance logic 编排（写库 + 写元组，审计先行）。
//
// 可见性设计（与 kafka topics 的 org_id 列谓词方案不同）：**不建归属列**——
// 实例量级小（个位/百级），列表可见性直接用引擎 list-objects 返回的可见 ID
// 集合做 IN 下推（纯引擎口径，can_view = member_view or owner），无双写与
// 漂移问题。量级涨到几万再考虑列谓词/游标分页（不为想象的规模提前优化）。
package repo

import (
	"context"
	"time"
)

// Instance 是一条 Redis 实例记录。
type Instance struct {
	// ID 自增主键（落库时由 DB 生成；Create 入参 ID=0 视为新建）。
	// 引擎 object id 形如 redis_instance:{int64}——业务层用 strconv.FormatInt 拼接。
	ID int64
	// Name 实例名（全表唯一）。
	Name string
	// Addr 实例地址（host:port，如 127.0.0.1:6379）。
	Addr string
	// Description 描述。
	Description string
	// ClusterID 归属集群 ID（clusters 表自增主键；分组维度，一致性由应用层保证）。
	ClusterID int64
	// CreatedBy 创建人（裸 userId，JWT sub）。
	CreatedBy string
	// UpdateBy 最近更新人（裸 userId，JWT sub；Update 时由 logic 层填充）。
	UpdateBy string
	// CreatedAt/UpdatedAt 时间戳（实现层填充）。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// InstancePageFilter 是列表分页查询的过滤条件（可见性谓词，由 logic 层从
// 授权服务取回的用户维度 ID 集合构造——量级随用户自己的关系数增长）：
//   - All：全量视角（manage 码持有者），忽略 ID 谓词；
//   - IDs：用户可见（can_view）的对象 ID 集合（owner/viewer/editor 元组
//     经引擎模型的并集投影）。
//
// 页内行仍由 logic 层经引擎 BatchCheck 逐行复核（真源 + ABAC 叠加）。
type InstancePageFilter struct {
	// All 全量视角（manage 码持有者）。
	All bool
	// IDs 可见对象 ID 集合（IN 下推）。
	IDs []int64
	// ClusterID 集群过滤（>0 时与可见性谓词 AND 组合；列表页恒传——先选集群再出实例）。
	ClusterID int64
	// Page 页码，从 1 开始。
	Page int
	// PageSize 每页条数。
	PageSize int
}

// InstanceRepo 是 instances 表的存取接口（内存/MySQL 双实现）。
type InstanceRepo interface {
	// Create 新建（ID=0 时由 DB 自增生成），重名报错；返回落库后记录（含 DB 生成的 ID）。
	Create(ctx context.Context, t Instance) (Instance, error)
	// Get 按 ID 取记录；未命中返回 ErrNotFound。
	Get(ctx context.Context, id int64) (Instance, error)
	// GetByName 按名字取记录（uk_name 唯一索引点查——创建路径的重名预检）；
	// 未命中返回 ErrNotFound。
	GetByName(ctx context.Context, name string) (Instance, error)
	// List 全量记录（id 升序）——对账端点遍历业务表比对引擎元组用
	//（实例量级个位/百级，全量可接受）。
	List(ctx context.Context) ([]Instance, error)
	// Page 按可见性谓词分页查询（ORDER BY id 升序 + OFFSET），返回当页
	// 记录与满足谓词的总数（供前端分页条；ABAC 页内复核过滤后 total 为
	// 近似值——见 list logic 注释）。
	Page(ctx context.Context, f InstancePageFilter) ([]Instance, int, error)
	// Update 按 ID 全量覆盖其余字段；未命中返回 ErrNotFound。
	Update(ctx context.Context, t Instance) (Instance, error)
	// Delete 按 ID 删除；未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
	// CountByClusterID 统计某集群下的实例数（删除集群的非空校验用）。
	CountByClusterID(ctx context.Context, clusterID int64) (int, error)
	// ClusterIDsByInstanceIDs 查给定实例 ID 集合归属的集群 ID 去重集合
	//（集群列表对普通用户做"派生可见性"过滤用：用户可见实例所在集群的并集）。
	ClusterIDsByInstanceIDs(ctx context.Context, ids []int64) ([]int64, error)
}
