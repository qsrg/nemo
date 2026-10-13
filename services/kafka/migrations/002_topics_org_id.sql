-- 002_topics_org_id.sql：Topic 表加归属部门列（列表分页优化）。
-- 定位：授权维度反范式进业务表——归属（org_id）与创建人（created_by）是
-- 列表可见性过滤的谓词来源，配合索引支撑"列谓词 + LIMIT"分页查询，
-- 替代"list-objects 全量展开 + IN 下推"（可见集合随资源总数膨胀，几十万
-- 量级失效）。引擎元组仍是权限真源，列是查询加速副本（创建时与 owner/org
-- 元组双写，漂移方向安全：列多显示的行会被列表页的引擎复核拦截）。
-- org_id 语义：资源挂接的部门（kafka_topic:{id} org org:{id} 元组的列投影），
-- 空字符串 = 未挂接（存量行，兼容）。
ALTER TABLE topics
    ADD COLUMN org_id VARCHAR(64) NOT NULL DEFAULT '' AFTER description,
    ADD KEY idx_org (org_id),
    ADD KEY idx_creator (created_by);
