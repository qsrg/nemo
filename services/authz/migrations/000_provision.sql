-- 000_provision.sql：server 级建库脚本（plan Task 4 原文语义）。
-- 由 cmd/provision 先以 server 级 DSN（不带库名）执行，确保两个库存在：
--   - nemo_authz      ：OpenFGA 引擎存储（Task 8 使用，本任务只建不碰）
--   - nemo_authz_biz  ：authz 业务库（规则/资源类型/模型版本/审计，001 建表）
-- 随后迁移 runner 会在业务库连接上重放本脚本（IF NOT EXISTS 幂等）并记录版本。
CREATE DATABASE IF NOT EXISTS nemo_authz;
CREATE DATABASE IF NOT EXISTS nemo_authz_biz;
