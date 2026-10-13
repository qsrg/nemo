-- 001_biz.sql：redis 独立业务库（nemo_redis）初始 schema——两张表。
--   - instances：Redis 实例业务表；
--   - audits：redis 域操作审计（与 kafka/authz 审计表同构，独立记账）。
-- 全表含审计字段（AGENTS.md 数据库规范：create_by/update_by，本项目时间字段
-- 沿用既有 created_at/updated_at 命名），显式 ENGINE=InnoDB utf8mb4。
--
-- 可见性设计（与 kafka 的 org_id 列谓词方案不同）：本表**不建 org_id 列**——
-- 实例量级小（个位/百级），列表可见性直接以引擎 list-objects 返回的可见 ID
-- 集合做 IN 下推（can_view = member_view or owner 的纯引擎口径），无反范式
-- 列即无双写与漂移问题（KISS：不为想象的规模提前优化）。

-- Redis 实例业务表：主键 id 为 BIGINT UNSIGNED 自增无业务含义主键；name 业务
-- 唯一（UNIQUE 索引）；created_by 记录创建人（引擎侧 owner 元组同源）。
CREATE TABLE IF NOT EXISTS instances (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(128) NOT NULL,
    addr        VARCHAR(255) NOT NULL DEFAULT '',
    description VARCHAR(512) NOT NULL DEFAULT '',
    created_by  VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 审计表：redis 域操作审计（写库/授权等动作的先行条目与结果标记）。
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
    created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
