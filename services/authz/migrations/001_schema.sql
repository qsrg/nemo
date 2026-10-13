-- 001_schema.sql：业务库（nemo_authz_biz）完整 schema——一张文件全量表。
-- 由 cmd/provision 在业务库连接上执行（000_provision.sql 先建库），全部
-- CREATE TABLE IF NOT EXISTS 幂等，可重复执行；版本记录写入 schema_migrations。
--
-- 2026-09-23 整合说明：原 001~011 碎片迁移合并为本文件（含 008/009 的
-- ALTER 并入建表、010 的一次性改名数据迁移不再需要——新环境直接注册
-- kafka_topic 类型名）；kafka 相关历史迁移（003 topics 建表、005 DROP
-- required_level、010 topic→kafka_topic 改名）随 kafka 拆分独立服务已清理，
-- topics 表迁入 nemo_kafka 后从本库删除。
--
-- 全表含审计字段（AGENTS.md 数据库规范：create_by/create_time/update_by/
-- update_time，本项目时间字段沿用既有 created_at/updated_at 命名）；
-- 主键一律 BIGINT UNSIGNED 自增无业务含义（禁止业务字段做主键，唯一性走
-- UNIQUE 索引）；ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
-- COLLATE=utf8mb4_unicode_ci。

-- 规则表：角色 key 对资源前缀的授权（action 空 = 服务级授权）。
CREATE TABLE IF NOT EXISTS rules (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    role_key   VARCHAR(64)  NOT NULL,
    prefix     VARCHAR(255) NOT NULL,
    action     VARCHAR(64)  NOT NULL DEFAULT '',
    create_by  VARCHAR(64)  NOT NULL DEFAULT '',
    update_by  VARCHAR(64)  NOT NULL DEFAULT '',
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_role_key (role_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 资源类型注册表：name 为业务唯一键，dsl_block 供模型装配（注册机制消费）；
-- manage_code 为通用资源授权（/v1/authz/relations 读写名单）的类型级策略——
-- 注册时声明（如 kafka-topics:manage），持有者对该类型任意资源拥有名单
-- 查看/写/撤销权；空 = 不设全权码，名单只能由资源的 owner/editor 管理
-- （fail-closed，配套校验见 logic/grantpolicy.go）。
CREATE TABLE IF NOT EXISTS resource_types (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(64)  NOT NULL,
    dsl_block   MEDIUMTEXT   NOT NULL,
    manage_code VARCHAR(128) NOT NULL DEFAULT '',
    create_by   VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 模型版本表：授权模型 DSL 全文，按（created_at, id）倒序取最新；
-- engine_model_id 存 OpenFGA 引擎返回的 modelID（应用层经此列与引擎侧对应）。
CREATE TABLE IF NOT EXISTS model_versions (
    id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    engine_model_id  VARCHAR(64) NOT NULL,
    dsl              MEDIUMTEXT  NOT NULL,
    create_by        VARCHAR(64) NOT NULL DEFAULT '',
    update_by        VARCHAR(64) NOT NULL DEFAULT '',
    created_at       DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_engine_model_id (engine_model_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 审计表：审计先行三态（pending|committed|failed），snapshot 存 JSON 文本，
-- reason 记录 MarkFailed 的原因（排障用）；status 建索引供 ListPending 对账。
-- create_by = 操作人（operator 冗余）；update_by 固定 'system'（状态变更由
-- 写路径提交与对账进程完成，属系统行为）。
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

-- 菜单注册表：每行 = 一条可导航菜单项——服务根行（service_key 无冒号）或
-- 服务子页行（service_key 形如 authz:orgs）。service_key 全表唯一，创建后
-- 不可改（业务主键定位行）。resource_type 声明"该菜单关联的业务资源类型"
-- （菜单可见性改造，方案 B：资源驱动）——空 = 纯菜单（按 permission_code
-- 判定）；非空（如 kafka_topic）= 资源驱动菜单：用户对该类型下任一资源有
-- can_view 级关系即解锁菜单入口（me/menus 判定：permGranted OR 存在
-- can_view 关系）。
CREATE TABLE IF NOT EXISTS menus (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    service_key     VARCHAR(128) NOT NULL,
    title           VARCHAR(128) NOT NULL,
    icon            VARCHAR(64)  NOT NULL DEFAULT '',
    path            VARCHAR(128) NOT NULL DEFAULT '',
    sort            INT          NOT NULL DEFAULT 0,
    permission_code VARCHAR(128) NOT NULL DEFAULT '',
    resource_type   VARCHAR(64)  NOT NULL DEFAULT '',
    enabled         BOOL         NOT NULL DEFAULT TRUE,
    create_by       VARCHAR(64)  NOT NULL DEFAULT '',
    update_by       VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_service_key (service_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 组织元数据注册表：组织名称/父组织是展示元数据（引擎元组只存 org:{id}
-- 关系）；org_id 业务唯一键（展示名经此列反查）。
CREATE TABLE IF NOT EXISTS orgs (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    org_id      VARCHAR(255) NOT NULL,
    name        VARCHAR(128) NOT NULL,
    parent_id   VARCHAR(255) NOT NULL DEFAULT '',
    create_by   VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_org_id (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 鉴权属性存储表（ABAC 方案 A）：主体/资源共用一张表，owner 前缀区分——
-- 主体属性 owner 形如 user:zhangsan / org:tech-frontend#member；资源属性
-- owner 形如 topic:1（类型:id）。属性值为字符串（数字比较由策略层转换）。
CREATE TABLE IF NOT EXISTS attributes (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    owner      VARCHAR(128) NOT NULL,
    attr_key   VARCHAR(64)  NOT NULL,
    attr_value VARCHAR(256) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_owner_key (owner, attr_key),
    KEY idx_owner (owner)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ABAC 策略表（方案 A）：附加强约束（deny 优先于 ReBAC 授权），不替代资源
-- 关系。条件为二元比较：subject_attr op resource_attr（主体 vs 资源属性），
-- 或 subject_attr op attr_value（主体属性 vs 字面量，resource_attr 为空时）。
-- op 白名单 lt/lte/gt/gte/eq/neq；enabled=false 一律不评估（回滚开关）。
CREATE TABLE IF NOT EXISTS policies (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name          VARCHAR(128) NOT NULL,
    resource_type VARCHAR(64)  NOT NULL DEFAULT '',
    effect        VARCHAR(16)  NOT NULL DEFAULT 'deny',
    subject_attr  VARCHAR(64)  NOT NULL,
    op            VARCHAR(8)   NOT NULL,
    resource_attr VARCHAR(64)  NOT NULL DEFAULT '',
    attr_value    VARCHAR(256) NOT NULL DEFAULT '',
    enabled       TINYINT(1)   NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_resource_enabled (resource_type, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 用户档案表（服务合并：auth 并入 authz 落库）：按 IdP 身份（类型 + 主体）
-- 唯一映射平台用户，CAS 首登自动建档；id 为 DB 自增内部序号（对外 API
-- 一律输出 idp_subject，不外泄 id）。
CREATE TABLE IF NOT EXISTS users (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(255) NOT NULL,
    email       VARCHAR(255) NOT NULL,
    idp_type    VARCHAR(32)  NOT NULL,
    idp_subject VARCHAR(255) NOT NULL,
    create_by   VARCHAR(64)  NOT NULL DEFAULT '',
    update_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_idp (idp_type, idp_subject),
    KEY idx_name (name),
    KEY idx_idp_subject (idp_subject)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
