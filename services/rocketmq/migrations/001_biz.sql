-- 001_biz.sql：rocketmq 独立业务库（nemo_rocketmq）初始 schema。
-- 2026-10-09 试点落地（内存 → MySQL）：topics 表承载 Topic 业务数据，
-- 含审计字段（AGENTS.md 数据库规范：create_by/create_time/update_by/
-- update_time，本项目时间字段沿用既有 created_at/updated_at 命名），
-- 显式 ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci。

-- Topic 业务表：主键 id 为 BIGINT UNSIGNED 自增无业务含义主键；name 业务
-- 唯一（UNIQUE 索引）；org_id 记录归属组织（列表可见性谓词，与引擎 org
-- 元组双写——真源是引擎，本列只是过滤加速）；created_by 记录创建人（引擎
-- 侧 owner 元组同源）。
CREATE TABLE IF NOT EXISTS topics (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(128)    NOT NULL,
    env         VARCHAR(32)     NOT NULL DEFAULT '',
    description VARCHAR(512)    NOT NULL DEFAULT '',
    org_id      VARCHAR(64)     NOT NULL DEFAULT '',
    created_by  VARCHAR(64)     NOT NULL DEFAULT '',
    update_by   VARCHAR(64)     NOT NULL DEFAULT '',
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 种子数据（试点演示）：6 条跨环境/归属组织的 Topic，与内存时代的种子
-- 对齐；归属元组（rmq_topic:{id} org org:xxx）由授权接口写入（见 README
-- 授权说明），业务表只存 org_id 列。
INSERT INTO topics (name, env, description, org_id, created_by) VALUES
    ('ops-alerts',    'prod',    '运维告警主题',     'tech',    'admin'),
    ('dev-internal',  'dev',     '开发内部事件',     'tech-frontend', 'admin'),
    ('verify-org-1',  'staging', '组织1验证主题',    'root',    'admin'),
    ('pagedemo-01',   'prod',    '分页演示1',        'tech',    'admin'),
    ('pagedemo-02',   'prod',    '分页演示2',        'tech',    'admin'),
    ('pagedemo-03',   'dev',     '分页演示3',        'tech-frontend', 'admin');
