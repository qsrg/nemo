// topic.go 定义 Kafka Topic 演示域（Phase C 改造③④）的 repo 接口与模型。
// topics 表是业务属性的事实源；关系（owner/viewer/editor）存引擎，不在此表
// ——两份数据由 topic logic 编排（写库 + 写元组，审计先行）。
// 注（ABAC 方案 A 改造）：required_level 已删除——鉴权属性统一由 authz
// 属性存储（subject/resource attributes）+ ABAC 策略管理，业务表不再承载
// 鉴权门槛字段。
package repo

import (
	"context"
	"time"
)

// Topic 是一条 Kafka Topic 记录（演示域业务属性）。
type Topic struct {
	// ID 自增主键（落库时由 DB 生成；Create 入参 ID=0 视为新建）。
	// 引擎 object id 形如 kafka_topic:{int64}——业务层用 strconv.FormatInt 拼接。
	ID int64
	// Name Topic 名（全表唯一）。
	Name string
	// Env 环境（如 prod/staging）。
	Env string
	// Description 描述。
	Description string
	// OrgID 归属部门 ID（org:{id}，创建时自动挂创建人所在部门）。权限真源
	// 是引擎的 org 关系元组，本列是其查询投影——列表按归属过滤走列谓词，
	// 漂移方向安全：列多包含的行会被列表页的引擎复核（Check can_view）拦截。
	OrgID string
	// CreatedBy 创建人（裸 userId，JWT sub）。
	CreatedBy string
	// UpdateBy 最近更新人（裸 userId，JWT sub；Update 时由 logic 层填充）。
	UpdateBy string
	// CreatedAt/UpdatedAt 时间戳（实现层填充）。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TopicPageFilter 是列表分页查询的过滤条件（可见性谓词，由 logic 层从
// 授权服务取回的用户维度小集合构造——数量级随用户自己的关系数增长，
// 不随 Topic 总数）：
//   - All：全量视角（manage 码持有者），忽略其余谓词；
//   - OrgIDs：用户所属部门（归属列 IN 匹配，覆盖 member from org 可见性）；
//   - IDs：用户直接持有关系的对象 ID（owner/viewer/editor 元组并集，
//     覆盖显式授予可见性）。
// 两路谓词取 OR，与模型 can_view = member_view or owner 的语义对齐；
// 页内行再经引擎 Check 逐行复核（真源 + ABAC 叠加）。
type TopicPageFilter struct {
	All      bool
	OrgIDs   []string
	IDs      []int64
	Page     int
	PageSize int
}

// TopicRepo 是 Topic 表的存取接口（内存/MySQL 双实现，共享断言组）。
type TopicRepo interface {
	// Create 新建（ID=0 时由 DB 自增生成），重名报错；返回落库后记录（含 DB 生成的 ID）。
	Create(ctx context.Context, t Topic) (Topic, error)
	// Get 按 ID 取记录；未命中返回 ErrNotFound。
	Get(ctx context.Context, id int64) (Topic, error)
	// GetByName 按名字取记录（uk_name 唯一索引点查——创建路径的重名预检，
	// 几十万量级下替代全表 List 预检）；未命中返回 ErrNotFound。
	GetByName(ctx context.Context, name string) (Topic, error)
	// List 返回全部记录，按 name 升序保证输出确定性（对账/补偿任务用；
	// 列表接口走 Page，勿在此扩分页）。
	List(ctx context.Context) ([]Topic, error)
	// Page 按可见性谓词分页查询（ORDER BY id 升序 + OFFSET），返回当页
	// 记录与满足谓词的总数（供前端分页条；ABAC 页内复核过滤后 total 为
	// 近似值——见 list logic 注释）。
	Page(ctx context.Context, f TopicPageFilter) ([]Topic, int, error)
	// Update 按 ID 全量覆盖其余字段；未命中返回 ErrNotFound。
	Update(ctx context.Context, t Topic) (Topic, error)
	// Delete 按 ID 删除；未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
}
