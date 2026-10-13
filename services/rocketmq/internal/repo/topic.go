// topic.go 定义 rocketmq Topic 存储接口与错误语义。
package repo

import (
	"context"
	"errors"

	"nemo/services/rocketmq/internal/types"
)

// ErrNotFound 未命中错误（logic 层转 404）。
var ErrNotFound = errors.New("topic not found")

// ErrDuplicate 重名错误（logic 层转 409）。
var ErrDuplicate = errors.New("topic name duplicate")

// TopicRepo Topic 存取接口（MySQL 实现 + 内存演示实现）。
type TopicRepo interface {
	// Create 新建（ID 由实现自增生成），重名报错；返回落库后记录。
	Create(ctx context.Context, t types.RocketmqTopic) (types.RocketmqTopic, error)
	// Get 按 ID 取记录；未命中返回 ErrNotFound。
	Get(ctx context.Context, id int64) (types.RocketmqTopic, error)
	// GetByName 按名字取记录（创建路径重名预检）；未命中返回 ErrNotFound。
	GetByName(ctx context.Context, name string) (types.RocketmqTopic, error)
	// ListByCluster 返回某集群下的全部记录，按 name 升序保证输出确定性。
	ListByCluster(ctx context.Context, clusterId int64) ([]types.RocketmqTopic, error)
	// List 返回全部记录（对账等全局视角用），按 name 升序。
	List(ctx context.Context) ([]types.RocketmqTopic, error)
	// Update 按 ID 全量覆盖其余字段；未命中返回 ErrNotFound。
	Update(ctx context.Context, t types.RocketmqTopic) (types.RocketmqTopic, error)
	// Delete 按 ID 删除；未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
}
