-- 002_clusters.sql：集群实体（实例管理页"先选集群、再出实例"的分组维度）。
-- 集群定位为基础配置数据（照 kafka clusters 模式）：不进权限引擎、无
-- resourceType/元组；读无守卫（页面可见即可读），写挂 redis:manage 码（路由层）。
-- 实例归属集群：cluster_id 由应用层保证一致性（禁外键规范）；存量实例由
-- 本迁移收纳进自动创建的默认集群。整文件由 provision 版本表控制只执行一次。

CREATE TABLE IF NOT EXISTS clusters (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    code        VARCHAR(64)  NOT NULL,
    remark      VARCHAR(512) NOT NULL DEFAULT '',
    created_by  VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 实例归属集群：DEFAULT 0 仅作迁移过渡（列 NOT NULL，无"未分组"业务态），
-- 本迁移随即把存量实例全部收纳进默认集群。
ALTER TABLE instances
    ADD COLUMN cluster_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD INDEX idx_cluster (cluster_id);

INSERT INTO clusters (code, remark, created_by, update_by)
VALUES ('default', '默认集群（迁移自动创建）', 'system', 'system');

UPDATE instances
SET cluster_id = (SELECT id FROM clusters WHERE code = 'default')
WHERE cluster_id = 0;
