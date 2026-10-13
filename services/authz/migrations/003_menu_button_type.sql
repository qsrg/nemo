-- 003_menu_button_type.sql：菜单表按钮实体化——按钮成为一级授权实体。
-- 定位：按钮级授权此前是菜单行上的 action_codes 逗号字符串（002），按钮
-- 本身无实体、不能单独管理，且资源级按钮（有实例操作）与页面级按钮
-- （无实例全局操作）混在同一字符串里，注册点分散（菜单/资源类型/代码）。
-- 本次将按钮提升为 menus 表中的独立行（type=button），挂在父菜单下
-- （parent_key），两类按钮统一就近声明：
--   - 页面级按钮：relation 为空，service_key 即功能码（kafka-topics:create）；
--   - 资源级按钮：relation 为资源关系（can_freeze）+ resource_type 指明
--     资源类型，授权跟随「数据权限」三档。
-- 兼容：action_codes 列保留（历史数据待 IaC 迁移为按钮行后不再使用）。
ALTER TABLE menus
    ADD COLUMN type       VARCHAR(16)  NOT NULL DEFAULT 'menu' AFTER service_key,
    ADD COLUMN parent_key VARCHAR(128) NOT NULL DEFAULT '' AFTER type,
    ADD COLUMN relation   VARCHAR(64)  NOT NULL DEFAULT '' AFTER resource_type;
