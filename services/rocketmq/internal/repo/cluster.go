// cluster.go 定义 rocketmq 集群存储接口与错误语义。
package repo

import (
	"context"
	"errors"
	"time"
)

// ErrClusterNotFound 未命中错误（logic 层转 404）。
var ErrClusterNotFound = errors.New("cluster not found")

// ErrClusterDuplicate 集群编码重复错误（logic 层转 409）。
var ErrClusterDuplicate = errors.New("cluster code duplicate")

// Cluster 集群记录（业务数据；code 业务唯一，创建后不可改——对象身份锚点）。
type Cluster struct {
	// ID 自增主键（落库时由 DB 生成；Create 入参 ID=0 视为新建）。
	ID int64
	// Code 集群编码（uk_code 唯一，创建后不可改）。
	Code string
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

// ClusterRepo 是 clusters 表的存取接口（MySQL 实现；内存实现供演示）。
type ClusterRepo interface {
	// Create 新建（ID=0 时由 DB 自增生成），code 重复报错；返回落库后记录。
	Create(ctx context.Context, c Cluster) (Cluster, error)
	// GetById 按 ID 取记录；未命中返回 ErrClusterNotFound。
	GetById(ctx context.Context, id int64) (Cluster, error)
	// GetByCode 按 code 取记录（uk_code 唯一索引点查——创建路径的重名预检）；
	// 未命中返回 ErrClusterNotFound。
	GetByCode(ctx context.Context, code string) (Cluster, error)
	// List 全量列表（ORDER BY id 升序——集群量级个位/十级，选择器场景不分页）。
	List(ctx context.Context) ([]Cluster, error)
	// Delete 按 ID 删除；未命中返回 ErrClusterNotFound。集群下是否仍有 topic
	// 由 logic 层经 TopicRepo 校验（跨表一致性不在此处，禁外键规范）。
	Delete(ctx context.Context, id int64) error
}
