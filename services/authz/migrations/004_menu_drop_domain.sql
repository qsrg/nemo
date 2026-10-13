-- 004_menu_drop_domain.sql：菜单表移除 domain（业务域分组）列。
-- 定位：domain 是手填的分组文本（如 消息域），与 sort 重复表达排序、且基座
-- 侧边栏/服务分组由静态注册表与 sort 承担；去掉配置面（yaml/表单/列表/API）
-- 一并删除该列，减少手填字段与数据不一致。
-- 兼容：已应用 001（含 domain 列）的环境执行本脚本删列；新环境按
-- 001→002→003→004 顺序执行，001 建含 domain 列、本脚本删除，幂等。
ALTER TABLE menus DROP COLUMN domain;
