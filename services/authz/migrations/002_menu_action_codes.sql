-- 002_menu_action_codes.sql：菜单表加附加动作码（按钮级功能权限目录）。
-- 定位：人员授权矩阵的码目录此前只有菜单注册表派生的 read/manage 两档；
-- 本列登记菜单（页面）的附加动作码（如 kafka-topics:export /
-- kafka-topics:reset-offset），矩阵据此展开动作勾选框，页面即可完成
-- 按钮级授权——码的授予/继承/通配语义与 read/manage 完全一致。
-- 存储：逗号分隔（每菜单动作码通常个位数，512 字符约可容纳 15 个）。
ALTER TABLE menus
    ADD COLUMN action_codes VARCHAR(512) NOT NULL DEFAULT '' AFTER resource_type;
