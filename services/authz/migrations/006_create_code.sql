-- 006_create_code.sql：resource_types 增加创建码声明（创建路径自举死锁的
-- 放行依据）。背景：普通创建者对新对象无任何元组、也不持管理码，以用户
-- 身份写首个 owner 元组会被名单管理权判权拒绝——放行分支（relation 为
-- owner/org 且对象尚无 owner 元组）要求调用者持有本列声明的创建码或
-- 管理码，二选一。空串 = 该类型无创建码（创建入口仅管理码，如 zk_cluster）。
-- 存量类型在此一次性 UPDATE；新类型由部署清单维护（同 manage_code 口径）。

ALTER TABLE resource_types
    ADD COLUMN create_code VARCHAR(128) NOT NULL DEFAULT '' AFTER manage_code;

UPDATE resource_types SET create_code = 'redis:create',        update_by = 'ops-fix' WHERE name = 'redis_instance';
UPDATE resource_types SET create_code = 'kafka-topics:create', update_by = 'ops-fix' WHERE name = 'kafka_topic';
