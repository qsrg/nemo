-- 004_menu_multi_resource_type.sql：菜单支持多个资源类型（逗号分隔）。
-- 定位：一个页面可能展示多种业务资源（如 topic 列表 + 消费者组列表），
-- 菜单的 resource_type 从单值扩展为多值（逗号分隔），me/menus 解锁语义
-- = 用户对任一声明类型下有任一资源可见即解锁页面入口。
-- 兼容：存量单值行不受影响（单值 = 列表长度为 1）；资源级按钮同样允许
-- 多值（一个按钮操作同一类型的多资源场景罕见，但解析函数统一兼容）。
ALTER TABLE menus
    MODIFY COLUMN resource_type VARCHAR(255) NOT NULL DEFAULT '';
