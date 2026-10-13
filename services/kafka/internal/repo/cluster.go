// cluster.go 定义集群（Cluster）业务模型与存取接口：机房/集群编码/地址/备注
// 基础信息维护，写操作经路由守卫 kafka-clusters:manage 门控（见 handler/guard.go）。
// 实现层分内存/MySQL 双实现（与 TopicRepo 同构），共享同一组接口契约。
package repo

import (
	"context"
	"time"
)

// Cluster 是一条集群记录。
type Cluster struct {
	// ID 自增主键（DB 生成，无业务含义）。
	ID int64
	// Code 集群编码（业务唯一，uk_code；创建后不可改——对象身份不可变，与
	// Topic name 同规则，编辑接口不接收 code）。
	Code string
	// Dc 机房。
	Dc string
	// Address 集群地址。
	Address string
	// Remark 备注。
	Remark string
	// CreatedBy 创建人（JWT sub）。
	CreatedBy string
	// UpdateBy 最近更新人（审计字段）。
	UpdateBy string
	// CreatedAt 创建时间。
	CreatedAt time.Time
	// UpdatedAt 最近更新时间。
	UpdatedAt time.Time
}

// ClusterRepo 是集群业务表的存取接口。
type ClusterRepo interface {
	// Create 新建集群：code 唯一冲突返回 ErrDuplicate（logic 层转 409）。
	Create(ctx context.Context, c Cluster) (Cluster, error)
	// Update 更新集群属性（code 不可改；未命中返回 ErrNotFound）。
	Update(ctx context.Context, c Cluster) error
	// Delete 删除集群：未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
	// GetById 按 ID 取单条；未命中返回 ErrNotFound。
	GetById(ctx context.Context, id int64) (Cluster, error)
	// GetByCode 按集群编码取单条；未命中返回 ErrNotFound（重名预检用）。
	GetByCode(ctx context.Context, code string) (Cluster, error)
	// Page 分页查询：返回当前页记录与总数（按 id 降序，新集群在前）。
	Page(ctx context.Context, page, pageSize int) ([]Cluster, int64, error)
}
