-- 007_menu_id_ref.sql：菜单模型简化（docs/menu-simplify-design.md §5）——
-- 键名删除（service_key/parent_key → id + parent_id）+ 双码合并一列
-- （permission_code 成为菜单/按钮共用的唯一码列）。
--
-- 迁移步骤（顺序关键：先改 schema 再回填数据，空串→NULL 必须在建 UNIQUE 前）：
--   ① 加 parent_id（按 parent_key 反查父行 id 回填）；
--   ② permission_code 改可空（为第③步设 NULL 铺路）；
--   ③ 码归一：
--      - 全部按钮行回填 permission_code = 原 service_key（页面级 = 功能码，
--        资源级 = 前端动作分派标识——授权仍走 relation 三档，码不参与判定）；
--      - 空串码行置 NULL（UNIQUE 允许多 NULL，但不允许多空串）；
--   ④ 建 UNIQUE 索引（非空码全表唯一，NULL 参与唯一校验）；
--   ⑤ 按 parent_key 反查 parent_id；
--   ⑥ 删 service_key/parent_key 列与 uk_service_key。
-- 兼容：引擎元组、前端硬编码码、审计历史（target menu:{key}）零改动；
-- me/menus 空码语义上线（存量全有码，行为无变化）。

-- ① 加 parent_id 列（顶级/服务首页为 NULL）。
ALTER TABLE menus
    ADD COLUMN parent_id BIGINT UNSIGNED NULL AFTER type;

-- ② 先改 permission_code 可空（第③步要设 NULL，必须列先改可空）。
ALTER TABLE menus
    MODIFY COLUMN permission_code VARCHAR(128) NULL;

-- ③ 码归一：全部按钮行回填 service_key（页面级 = 功能码、资源级 = 动作
-- 分派标识）；菜单行空码与按钮行残留空串置 NULL。
UPDATE menus SET permission_code = service_key
WHERE type = 'button' AND permission_code = '';
UPDATE menus SET permission_code = NULL WHERE permission_code = '';

-- ④ 空串已清为 NULL，建 UNIQUE（允许多个 NULL；非空码唯一）。
ALTER TABLE menus
    ADD UNIQUE KEY uk_permission_code (permission_code);

-- ⑤ 按 parent_key 反查父行 id 回填（键名删除前完成，逐行对应）。
UPDATE menus child
    JOIN menus parent ON child.parent_key = parent.service_key
    SET child.parent_id = parent.id
WHERE child.parent_key <> '';

-- ⑥ 删键名列与唯一键。
ALTER TABLE menus
    DROP KEY uk_service_key,
    DROP COLUMN service_key,
    DROP COLUMN parent_key;
