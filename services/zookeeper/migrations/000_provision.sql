-- 000_provision.sql：server 级建库脚本（zookeeper 独立服务专用库）。
-- 由 cmd/provision 以 server 级 DSN（不带库名）执行，确保业务库存在：
--   - nemo_zookeeper：zk 服务业务库（clusters 集群注册表，001 建表）
-- 随后迁移 runner 会在业务库连接上重放本脚本（IF NOT EXISTS 幂等）并记录版本。
CREATE DATABASE IF NOT EXISTS nemo_zookeeper;
