#!/usr/bin/env bash
# 新环境一键初始化 + 启动（幂等，可重复执行）。
#
# 职责：数据库创建 + 迁移 + 基线数据 seed（菜单/按钮/资源类型）+ 启动服务。
# 与 scripts/restore-dev.sh 分工：
#   - 本脚本管"初始化"——建库（000_provision.sql）、全量迁移（含
#     migrations/008_menu_seed.sql 的菜单/按钮/资源类型基线 seed）；
#   - 日常恢复/重启只跑 restore-dev.sh（服务拉起 + 幂等对账）——菜单数据
#     在 MySQL（nemo_authz_biz）持久化，重启不丢、无需重灌。
#
# 运行时事实源（2026-10-11 起，deploy/menus.yaml + apply-config.py 已下线）：
#   - 菜单/按钮：authz 的 menus 表，生产经管理面 CRUD 维护；
#   - 资源类型：随菜单/按钮声明派生（隐式注册），注册表 ManageCode/
#     CreateCode 声明驱动（008 seed 固化当前基线；需要重做基线时从开发库
#     重新导出，mysqldump 命令见下）。
#
# 前置：MySQL 容器（3306，凭据见 .local-dev.md）、CAS 容器（8443）在运行。
# 用法：bash scripts/init-env.sh
set -euo pipefail

nemo_HOME="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MYSQL_DSN_LOCAL="${MYSQL_DSN_LOCAL:-root:abc123456@tcp(127.0.0.1:3306)/}"

log() { printf '[init-env] %s\n' "$*"; }

# ---------- 0. 前置检查 ----------
check_prereq() {
  log "检查 MySQL（3306）…"
  if ! lsof -ti :3306 >/dev/null 2>&1; then
    log "MySQL 容器未运行（3306）——请先启动（凭据见 .local-dev.md）" >&2
    return 1
  fi
  log "检查 CAS（8443）…"
  if ! curl -sf -m 3 -o /dev/null http://localhost:8443/login; then
    log "CAS 容器未就绪（8443）——请先启动" >&2
    return 1
  fi
}

# ---------- 1. 建库 + 迁移 + 基线 seed ----------
provision_authz() {
  # authz 业务库 nemo_authz_biz（+ 引擎库 nemo_authz）：
  #   - 000_provision.sql 建库；
  #   - 001-007 表结构与演进；
  #   - 008_menu_seed.sql 菜单/按钮 + 资源类型基线（INSERT IGNORE 幂等）。
  # 幂等：schema_migrations 记录已应用版本，重复执行安全。
  log "provision authz 库（nemo_authz_biz + 引擎库 nemo_authz，含菜单/资源类型 seed）…"
  (cd "$nemo_HOME/services/authz" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

provision_kafka() {
  log "provision kafka 库（nemo_kafka）…"
  (cd "$nemo_HOME/services/kafka" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

provision_redis() {
  log "provision redis 库（nemo_redis）…"
  (cd "$nemo_HOME/services/redis" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

provision_zookeeper() {
  log "provision zookeeper 库（nemo_zookeeper）…"
  (cd "$nemo_HOME/services/zookeeper" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

provision_rocketmq() {
  log "provision rocketmq 库（nemo_rocketmq）…"
  (cd "$nemo_HOME/services/rocketmq" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

# ---------- 2. 启动（复用 restore-dev.sh：构建 + 拉起 + 幂等对账） ----------
main() {
  check_prereq
  provision_authz
  provision_kafka
  provision_redis
  provision_zookeeper
  provision_rocketmq
  log "初始化完成，启动服务（restore-dev.sh）…"
  bash "$nemo_HOME/scripts/restore-dev.sh"
}

main "$@"
