// cluster.go 定义 Redis 集群的 repo 接口与模型。
// 集群是实例管理页"先选集群、再出实例"的分组维度，定位为基础配置数据
//（照 kafka clusters 模式）：不进权限引擎（无 resourceType/owner 元组），
// 读无守卫，写挂 redis:manage 码（路由层）。实例归属经 instances.cluster_id
// 关联（应用层保证一致性，禁外键规范）。
package repo

import (
	"context"
	"time"
)

// Cluster 是一条 Redis 集群记录。
type Cluster struct {
	// ID 自增主键（落库时由 DB 生成；Create 入参 ID=0 视为新建）。
	ID int64
	// Code 集群编码（全表唯一，创建后不可改——对象身份不可变）。
	Code string
	// Hosts ZK 集群连接串（host:port 逗号分隔；阶段一 mock 不消费，阶段二节点浏览按此连接）。
	Hosts string
	// Remark 备注（用途/机房等说明）。
	Remark string
	// CreatedBy 创建人（裸 userId，JWT sub）。
	CreatedBy string
	// UpdateBy 最近更新人。
	UpdateBy string
	// CreatedAt/UpdatedAt 时间戳（实现层填充）。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ClusterRepo 是 clusters 表的存取接口（内存/MySQL 双实现）。
type ClusterRepo interface {
	// Create 新建（ID=0 时由 DB 自增生成），code 重复报错；返回落库后记录（含 DB 生成的 ID）。
	Create(ctx context.Context, c Cluster) (Cluster, error)
	// GetById 按 ID 取记录；未命中返回 ErrNotFound。
	GetById(ctx context.Context, id int64) (Cluster, error)
	// GetByCode 按 code 取记录（uk_code 唯一索引点查——创建路径的重名预检）；
	// 未命中返回 ErrNotFound。
	GetByCode(ctx context.Context, code string) (Cluster, error)
	// List 全量列表（ORDER BY id 升序——集群量级个位/十级，选择器场景不分页）。
	List(ctx context.Context) ([]Cluster, error)
	// Delete 按 ID 删除；未命中返回 ErrNotFound。集群下是否仍有实例由 logic 层
	// 经 InstanceRepo.CountByClusterID 校验（跨表一致性不在此处）。
	Delete(ctx context.Context, id int64) error
}
