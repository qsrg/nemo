// Package repo 提供 sample 服务的存储层：定义 Item 存储模型与 ItemRepo 接口，
// 并给出内存实现（NewMemory，骨架/演示期默认）与 MySQL 参考实现（NewMySQL）。
//
// 约定：
//   - repo.Item 是存储模型，与 internal/types.Item（API 出入参模型）严格分离，
//     转换发生在 logic 层，repo 包不得反向依赖 types；
//   - 资源不存在时统一返回 ErrNotFound（errors.Is 可判定），logic 层据此转 404；
//   - 所有 SQL 均使用占位符参数化，禁止字符串拼接用户输入。
package repo

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 表示目标资源不存在；实现方对未命中统一返回该哨兵错误，
// 调用方必须用 errors.Is 判定而非字符串比较。
var ErrNotFound = errors.New("item not found")

// Item 是 items 表的存储模型（与 types.Item 分离，不外泄到 API 层）。
type Item struct {
	// ID 自增主键（落库时由 DB 生成；Create 入参 ID=0 视为新建，与自增主键
	// 语义一致；API 层经 strconv 与字符串路径参数互转）。
	ID int64
	// Name 资源名称。
	Name string
	// Desc 资源描述。
	Desc string
	// CreateBy 创建人（无鉴权参考实现：调用方不传则为 '' = 未知操作人）。
	CreateBy string
	// UpdateBy 最近更新人（同上，未知操作人为 ''）。
	UpdateBy string
	// CreatedAt 创建时间，由实现方在 Create 时填充。
	CreatedAt time.Time
	// UpdatedAt 最近更新时间，由实现方在 Create/Update 时填充。
	UpdatedAt time.Time
}

// ItemRepo 是 Item 资源的存储接口；logic 层仅依赖该接口，
// 内存/MySQL 实现可替换。所有未命中的读写一律返回 ErrNotFound。
type ItemRepo interface {
	// Create 新增一条记录：ID 由实现方自增生成（调用方传入非 0 ID 时忽略，
	// 与自增主键语义一致），时间戳与审计字段（CreateBy/UpdateBy）由实现方
	// 填充并返回完整记录。
	Create(ctx context.Context, it Item) (Item, error)
	// Get 按 id 查询单条记录；未命中返回 ErrNotFound。
	Get(ctx context.Context, id int64) (Item, error)
	// List 返回全部记录，按 id 升序。
	List(ctx context.Context) ([]Item, error)
	// Update 按 it.ID 全量覆盖 Name/Desc 并刷新 UpdatedAt 与 UpdateBy
	// （CreatedAt/CreateBy 保留原值）；未命中返回 ErrNotFound，
	// 返回更新后的完整记录。
	Update(ctx context.Context, it Item) (Item, error)
	// Delete 按 id 删除记录；未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
}
