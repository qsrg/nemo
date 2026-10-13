// menu.go 定义菜单注册表（菜单模型简化，docs/menu-simplify-design.md）的
// repo 接口与模型。行身份 = 自增 id，父子引用 = parent_id（顶级/服务首页
// 为 0）；权限码统一存 PermissionCode 列（菜单行可选：空 = 全员可见；页面
// 级按钮必填；资源级按钮为空）。
package repo

import (
	"context"
	"time"
)

// Menu 是一条菜单注册记录（菜单行 / 按钮行）。
type Menu struct {
	// ID 行主键（DB 自增；Create 后回填，接口路径以 id 定位行）。
	ID int64
	// ParentID 父行 id：顶级菜单行（服务首页）为 0；按钮行必填（指向所属
	// 菜单行）。按钮行必须挂在存在的菜单行下，否则授权界面无从展示。
	ParentID int64
	// Type 行类型：menu（菜单/导航）| button（按钮级授权实体）。按钮行不参与
	// 侧边栏，仅作为授权目录挂在父菜单下。默认 menu。
	Type string
	// Title 菜单展示名（如 实例管理）；按钮行 = 按钮名（如 新建 Topic）。
	Title string
	// Icon 图标名（Element Plus 图标名；子页可空 = 沿用服务图标）。
	Icon string
	// Path 完整路由（如 /kafka/topics）；按钮行可空（不入侧边栏）。
	Path string
	// Sort 排序值（升序）。
	Sort int
	// PermissionCode 权限标识（如 redis:read / kafka-topics:create）：三方
	// join key（授权界面勾选、引擎 permission:{code}#can 元组、微应用
	// hasPermission 硬编码）。菜单行可选——空 = 全员可见（me/menus 不过滤，
	// 准入兜底在网关路由/API 权限/按钮码层）；页面级按钮必填；资源级按钮为空
	//（授权跟随资源关系三档）。非空码全表唯一。
	PermissionCode string
	// ResourceType 关联的业务资源类型（如 kafka_topic，可逗号分隔多值）：空 =
	// 纯菜单（可见性仅按 PermissionCode 判定）；非空 = 资源驱动菜单/资源级
	// 按钮（用户对该类型下任一资源有 can_view 级关系即解锁入口，见 MeMenus）。
	ResourceType string
	// Relation 资源级按钮的资源关系（如 can_freeze）：仅 type=button 且资源级
	//（有实例操作）非空；页面级按钮与菜单行为空串。授权跟随「数据权限」三档。
	Relation string
	// Enabled 启用位：仅 enabled 的行进入 me/menus。
	Enabled bool
	// CreateBy 创建人（JWT sub；Create 时由 logic 层填充）。
	CreateBy string
	// UpdateBy 最近更新人（JWT sub；Update 时由 logic 层填充）。
	UpdateBy string
	// CreatedAt/UpdatedAt 时间戳（实现层填充）。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MenuRepo 是菜单注册表的存取接口（内存/MySQL 双实现，断言组模式）。
type MenuRepo interface {
	// Create 新建一行（ID 由存储层自增生成，create_by 取 m.CreateBy），
	// 返回落库后的完整记录。
	Create(ctx context.Context, m Menu) (Menu, error)
	// Update 按 ID 全量覆盖（PUT 语义；update_by 取 m.UpdateBy），
	// 未命中返回 ErrNotFound。
	Update(ctx context.Context, m Menu) (Menu, error)
	// List 返回全部记录，按 (sort, id) 升序保证输出确定性。
	List(ctx context.Context) ([]Menu, error)
	// Get 按 ID 取记录；未命中返回 ErrNotFound。
	Get(ctx context.Context, id int64) (Menu, error)
	// Delete 按 ID 删除；未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
}
