-- 001_biz.sql：clusters 集群注册表（"看到集群才能浏览节点"的授权与分组载体）。
-- hosts 列为真实 ZK 连接串预留（阶段一节点数据为 mock，阶段二按 hosts 连接
-- 读取 znode；mock 阶段不消费该列）。审计字段遵循 AGENTS.md 规范。

CREATE TABLE IF NOT EXISTS clusters (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    code        VARCHAR(64)  NOT NULL,
    hosts       VARCHAR(512) NOT NULL DEFAULT '',
    remark      VARCHAR(512) NOT NULL DEFAULT '',
    created_by  VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- audits 审计表（写路径审计先行，与 redis/kafka 同构；独立记账）。
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
