-- 001_init.sql：sample 服务初始 schema（plan Task 6 原文）。
-- descr 命名规避 MySQL 保留字 desc。
--
-- 规范改造（2026-09-11）：
--   - 主键：id 由 VARCHAR 应用生成主键（it-<unixnano>）改为 BIGINT UNSIGNED
--     自增无业务含义主键（AGENTS.md 数据库规范：优先自增主键，业务唯一性
--     通过唯一索引实现；items 无业务唯一键，仅主键约束）；
--   - 审计字段：补齐 create_by/update_by（sample 为无鉴权参考实现，无操作人
--     概念，'' = 未知操作人，应用层可自行填充）；
--   - updated_at 加 ON UPDATE CURRENT_TIMESTAMP 自动更新；
--   - 显式 ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci。
CREATE TABLE IF NOT EXISTS items (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(128) NOT NULL,
    descr      VARCHAR(512) NOT NULL DEFAULT '',
    create_by  VARCHAR(64)  NOT NULL DEFAULT '',
    update_by  VARCHAR(64)  NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
