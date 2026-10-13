-- 001_biz.sql：kafka 独立业务库（nemo_kafka）初始 schema——两张表。
-- 2026-09-23 随 kafka 独立服务拆分建库（表定义与 authz 侧同构，独立记账；
-- authz 侧历史碎片迁移已整合删除，本库为 kafka 唯一 schema 来源）：
--   - topics：Topic 业务表（required_level 已随 ABAC 方案 A 移除）；
--   - audits：kafka 域操作审计（与 authz 审计表同构，独立记账）。
-- 全表含审计字段（AGENTS.md 数据库规范：create_by/create_time/update_by/
-- update_time，本项目时间字段沿用既有 created_at/updated_at 命名），
-- 显式 ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci。

-- Topic 业务表：主键 id 为 BIGINT UNSIGNED 自增无业务含义主键；name 业务
-- 唯一（UNIQUE 索引）；created_by 记录创建人（引擎侧 owner 元组同源）。
CREATE TABLE IF NOT EXISTS topics (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(128) NOT NULL,
    env         VARCHAR(32)  NOT NULL DEFAULT '',
    description VARCHAR(512) NOT NULL DEFAULT '',
    created_by  VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 审计表：kafka 域操作审计（写库/授权/撤销等动作的先行条目与结果标记）。
CREATE TABLE IF NOT EXISTS audits (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    operator   VARCHAR(64) NOT NULL,
    action     VARCHAR(64) NOT NULL,
    target     VARCHAR(255) NOT NULL,
    snapshot   TEXT        NOT NULL,
    status     ENUM('pending','committed','failed') NOT NULL DEFAULT 'pending',
    reason     VARCHAR(512) NOT NULL DEFAULT '',
    create_by  VARCHAR(64) NOT NULL DEFAULT '',
    update_by  VARCHAR(64) NOT NULL DEFAULT 'system',
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
