-- 002_clusters.sql：rocketmq 集群实体（topic 页"先选集群、再出 topic"的
-- 分组维度 + 集群级授权对象）。
-- 集群定位（与 redis 002 同款演进）：集群进权限引擎（rmq_cluster 资源类型，
-- 授权经页面配置 viewer/owner），集群可见（can_view）即该集群下全部 topic
-- 可见可操作（2026-10-12 用户定稿模型）。
-- topic 归属集群：cluster_id 由应用层保证一致性（禁外键规范）；存量 topic
-- 由本迁移收纳进自动创建的默认集群。整文件由 provision 版本表控制只执行一次。

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

-- topic 归属集群：DEFAULT 0 仅作迁移过渡（列 NOT NULL，无"未分组"业务态），
-- 本迁移随即把存量 topic 全部收纳进默认集群。
ALTER TABLE topics
    ADD COLUMN cluster_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD INDEX idx_cluster (cluster_id);

INSERT INTO clusters (code, remark, created_by, update_by)
VALUES ('default', '默认集群（迁移自动创建）', 'system', 'system');

UPDATE topics
SET cluster_id = (SELECT id FROM clusters WHERE code = 'default')
WHERE cluster_id = 0;
