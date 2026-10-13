-- 003_clusters.sql：kafka 集群管理业务表。
-- 2026-09-29 新增集群管理页面（菜单 kafka:clusters，菜单注册见 authz 库 menus 表，
-- 本库只负责业务数据）：
--   - clusters：集群基础信息（机房/集群编码/地址/备注），写操作由
--     kafka-clusters:manage 菜单码守卫（路由层），列表读取无守卫（页面可见即可）。
-- 审计字段（AGENTS.md 数据库规范）随 topics 表同构：created_by/update_by/
-- created_at/updated_at。主键自增无业务含义；code（集群编码）业务唯一（唯一索引）。

CREATE TABLE IF NOT EXISTS clusters (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    code       VARCHAR(64)  NOT NULL,
    dc         VARCHAR(64)  NOT NULL DEFAULT '',
    address    VARCHAR(255) NOT NULL DEFAULT '',
    remark     VARCHAR(512) NOT NULL DEFAULT '',
    created_by VARCHAR(64)  NOT NULL DEFAULT '',
    update_by  VARCHAR(64)  NOT NULL DEFAULT '',
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
