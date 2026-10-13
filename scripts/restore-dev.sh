#!/usr/bin/env bash
# 恢复本地开发环境（幂等，可重复执行）。
#
# 背景：authz（含原 auth 登录链路，2026-09-21 服务合并）由用户会话手动
# nohup 启动，注销/重启系统后进程被 launchd 清理（nohup 只免疫终端关闭，
# 不免疫用户注销）。服务合并 + MySQL 持久化后：
#   - 业务数据（菜单/资源类型/topic/授权元组/用户表）全部落库
#     （nemo_authz_biz + 引擎库 nemo_authz），重启不再丢；
#   - 恢复脚本只负责"拉起服务 + 幂等对账"：菜单/按钮/资源类型数据在
#     MySQL（nemo_authz_biz）持久化（基线 seed 见 services/authz/migrations/
#     008_menu_seed.sql，由 scripts/init-env.sh 在全新环境初始化时执行），
#     重启不再丢、无需重灌；演示 topic/实例重建幂等（接口冲突忽略）。
#     （2026-10-11：deploy/menus.yaml 与 scripts/apply-config.py 已下线，
#     菜单运行时事实源是 authz 的 menus 表，生产经管理面 CRUD 维护）
#
# 权限分层（铁律）：终端用户权限（菜单/功能码、资源 viewer/editor 元组）
# 一律经控制台配置——「人员授权」页授码、业务页行内「权限配置」授元组，
# 脚本不预置任何用户/演示账号的权限。脚本仅自举两类系统身份：
#   - admin：bootstrap 已写超管通配元组（permission:*.*#can@user:admin），
#     天然全权，无需再授；
#   - 创建路径的 owner 元组写入：authz 对空对象首写放行（调用者持声明在
#     resource_types.create_code 的创建码或管理码），无需脚本预置授权。
#
# 微服务拆分（2026-09-23）：kafka 独立为 services/kafka（:8085），业务数据
# 自管独立库 nemo_kafka（topics/audits 表，不再复用 authz 的 nemo_authz_biz），
# 权限判定与授权名单经 kit/authz 远程调用集中式 authz（:8084）；前端微应用
# 目录 console-frontend/apps/kafka（原 order，改名后端口 5275 不变），vite
# proxy 把 /api/v1/kafka 分流到 :8085。
#
# redis 拆分（2026-09-30）：redis 实例管理独立为 services/redis（:8086），
# 业务库 nemo_redis（instances/audits 表，无归属列——纯引擎可见性口径），
# 判定经 kit/authz 远程调用集中式 authz；前端微应用 apps/redis（:5277），
# vite proxy 把 /api/v1/redis 分流到 :8086。
#
# 前置：MySQL 容器（3306，凭据见 .local-dev.md）、CAS 容器（8443）在运行；
# 全新环境先跑 scripts/init-env.sh（建库 + 迁移 + 菜单/资源类型基线 seed），
# 日常恢复/重启只跑本脚本（脚本内 provision 幂等兜底，服务已在跑则跳过）。
set -euo pipefail

nemo_HOME="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AUTHZ_BIN="${AUTHZ_BIN:-/tmp/authz-api-new}"
KAFKA_BIN="${KAFKA_BIN:-/tmp/kafka-api-new}"
REDIS_BIN="${REDIS_BIN:-/tmp/redis-api-new}"
ZK_BIN="${ZK_BIN:-/tmp/zookeeper-api-new}"
ROCKETMQ_BIN="${ROCKETMQ_BIN:-/tmp/rocketmq-api-new}"
CAS_BASE="http://localhost:8443"
AUTHZ_ADDR="http://localhost:8084"
KAFKA_ADDR="http://localhost:8085"
REDIS_ADDR="http://localhost:8086"
SERVICE="http://localhost:5273/cas-ticket"
CAS_ADMIN_USER="${CAS_ADMIN_USER:-admin}"
CAS_ADMIN_PASS="${CAS_ADMIN_PASS:-admin123}"
MYSQL_DSN_LOCAL="${MYSQL_DSN_LOCAL:-root:abc123456@tcp(127.0.0.1:3306)/}"

log() { printf '[restore-dev] %s\n' "$*"; }

# ---------- 0. 构建二进制（authz + kafka 独立服务） ----------
build_authz() {
  # 默认每次重建（合并改造期二进制可能落后于代码；构建约数秒）。
  # 追求速度可设 AUTHZ_REUSE=1 复用已构建的二进制。
  if [ "${AUTHZ_REUSE:-0}" != "1" ]; then
    log "构建 authz（服务合并：含原 auth 登录/会话/用户目录链路）…"
    (cd "$nemo_HOME/services/authz" && go build -o "$AUTHZ_BIN" authz.go)
  else
    log "复用已有二进制 $AUTHZ_BIN（AUTHZ_REUSE=1）"
  fi
}

build_kafka() {
  # kafka 独立服务二进制（判定经 kit/authz 远程调用 authz，不内嵌引擎）。
  if [ "${KAFKA_REUSE:-0}" != "1" ]; then
    log "构建 kafka 独立服务（:8085）…"
    (cd "$nemo_HOME/services/kafka" && go build -o "$KAFKA_BIN" kafka.go)
  else
    log "复用已有二进制 $KAFKA_BIN（KAFKA_REUSE=1）"
  fi
}

build_zookeeper() {
  # zookeeper 独立服务二进制（判定经 kit/authz 远程调用 authz，不内嵌引擎）。
  if [ "${ZK_REUSE:-0}" != "1" ]; then
    log "构建 zookeeper 独立服务（:8087）…"
    (cd "$nemo_HOME/services/zookeeper" && go build -o "$ZK_BIN" zookeeper.go)
  else
    log "复用已有二进制 $ZK_BIN（ZK_REUSE=1）"
  fi
}

provision_kafka() {
  # kafka 独立业务库 nemo_kafka 建库 + 迁移（幂等：schema_migrations 记录
  # 已应用版本，重复执行安全；authz 库的 provision 见 README/一次性动作）。
  log "provision kafka 库（nemo_kafka，幂等）…"
  (cd "$nemo_HOME/services/kafka" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

build_redis() {
  # redis 独立服务二进制（判定经 kit/authz 远程调用 authz，不内嵌引擎）。
  if [ "${REDIS_REUSE:-0}" != "1" ]; then
    log "构建 redis 独立服务（:8086）…"
    (cd "$nemo_HOME/services/redis" && go build -o "$REDIS_BIN" redis.go)
  else
    log "复用已有二进制 $REDIS_BIN（REDIS_REUSE=1）"
  fi
}

provision_redis() {
  # redis 独立业务库 nemo_redis 建库 + 迁移（幂等，同 kafka 口径）。
  log "provision redis 库（nemo_redis，幂等）…"
  (cd "$nemo_HOME/services/redis" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

build_rocketmq() {
  # rocketmq 独立服务二进制（判定经 kit/authz 远程调用 authz，不内嵌引擎）。
  if [ "${ROCKETMQ_REUSE:-0}" != "1" ]; then
    log "构建 rocketmq 独立服务（:8088）…"
    (cd "$nemo_HOME/services/rocketmq" && go build -o "$ROCKETMQ_BIN" rocketmq.go)
  else
    log "复用已有二进制 $ROCKETMQ_BIN（ROCKETMQ_REUSE=1）"
  fi
}

provision_rocketmq() {
  # rocketmq 独立业务库 nemo_rocketmq 建库 + 迁移（幂等，同 kafka 口径）。
  log "provision rocketmq 库（nemo_rocketmq，幂等）…"
  (cd "$nemo_HOME/services/rocketmq" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}

# ---------- 1. 拉起服务（已存在则跳过） ----------
start_in() { # start_in <dir> <port> <cmd...>
  local dir="$1" port="$2"; shift 2
  if lsof -ti ":$port" >/dev/null 2>&1; then
    log "端口 $port 已在运行，跳过启动"
  else
    log "启动($port): $* @ $dir"
    (cd "$dir" && nohup "$@" >/dev/null 2>&1 &)
    sleep 2
  fi
}

# authz：必须在 services/authz 下启动（相对路径读 etc/authz-api.yaml 与
# model.fga），且必须带 BOOTSTRAP=1（幂等引导：建 Store/模型/根组织/超管，
# MySQL 模式下首次必须，后续幂等可重复）。
build_authz
start_in "$nemo_HOME/services/authz" 8084 env BOOTSTRAP=1 "$AUTHZ_BIN"
# kafka 独立服务：必须在 services/kafka 下启动（相对路径读 etc/kafka-api.yaml）。
provision_kafka
build_kafka
start_in "$nemo_HOME/services/kafka" 8085 "$KAFKA_BIN"
# redis 独立服务：必须在 services/redis 下启动（相对路径读 etc/redis-api.yaml）。
provision_redis
build_redis
start_in "$nemo_HOME/services/redis" 8086 "$REDIS_BIN"
# zookeeper 独立服务：必须在 services/zookeeper 下启动（相对路径读
# etc/zookeeper-api.yaml）；集群表 provision（nemo_zookeeper）。
provision_zookeeper() {
  # zookeeper 业务库建库 + 迁移（幂等，同 redis/kafka 口径）。
  (cd "$nemo_HOME/services/zookeeper" && MYSQL_DSN="$MYSQL_DSN_LOCAL" go run ./cmd/provision)
}
provision_zookeeper
build_zookeeper
start_in "$nemo_HOME/services/zookeeper" 8087 "$ZK_BIN"
# rocketmq 独立服务：必须在 services/rocketmq 下启动（相对路径读
# etc/rocketmq-api.yaml）；业务库 provision（nemo_rocketmq）。
provision_rocketmq
build_rocketmq
start_in "$nemo_HOME/services/rocketmq" 8088 "$ROCKETMQ_BIN"
# 前端六件套：console 基座（5273）、kafka 微应用（5275，原 order 改名）、
# authz-admin（5276）、redis 微应用（5277）、zookeeper 微应用（5278）、
# rocketmq 微应用（5279）。
# 此前只起 8084/5276，console/kafka 靠手动 nohup，重启后失联——统一纳入脚本。
start_in "$nemo_HOME/console-frontend/apps/console" 5273 pnpm dev
start_in "$nemo_HOME/console-frontend/apps/kafka" 5275 pnpm dev
start_in "$nemo_HOME/console-frontend/apps/authz-admin" 5276 pnpm dev
start_in "$nemo_HOME/console-frontend/apps/redis" 5277 pnpm dev
start_in "$nemo_HOME/console-frontend/apps/zookeeper" 5278 pnpm dev
start_in "$nemo_HOME/console-frontend/apps/rocketmq" 5279 pnpm dev

# ---------- 2. CAS 登录换 token（回调统一走合并后的 authz :8084） ----------
get_token() {
  local enc ticket
  enc="$(python3 -c "import urllib.parse;print(urllib.parse.quote('$SERVICE',safe=''))")"
  ticket="$(curl -s -o /dev/null -w '%{redirect_url}' -X POST "$CAS_BASE/login" \
    -d "username=$CAS_ADMIN_USER&password=$CAS_ADMIN_PASS&service=$enc" | grep -oE 'ticket=[A-Za-z0-9-]+' | cut -d= -f2)"
  [ -n "$ticket" ] || { log "CAS 登录失败" >&2; return 1; }
  curl -s -m 5 -X POST "$AUTHZ_ADDR/v1/auth/cas/callback" -H 'Content-Type: application/json' \
    -d "{\"ticket\":\"$ticket\",\"service\":\"$SERVICE\"}" \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])'
}

# ---------- 3. 授权上下文（演示数据接口调用用；不预置任何终端用户权限） ----------
log "等待 authz 就绪…"
for _ in $(seq 1 20); do
  if lsof -ti :8084 >/dev/null 2>&1; then break; fi
  sleep 1
done
sleep 2
TOKEN="$(get_token)"
AUTH=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')


# 终端用户（lisi/zhangsan/wangwu/alice 等）不在此预置任何权限码——菜单可见、
# 页面按钮显隐、资源可见性全部经控制台配置：
#   - 菜单/功能码（redis:read / redis:create / redis:manage…）：
#     authz-admin「人员授权」页勾选授予；
#   - 资源 viewer/editor 元组：业务页行内「权限配置」/「批量权限配置」。
# 演示"差异化可见"场景的完整操作步骤见 services/redis/README.md。

# 演示 topic（创建幂等：已存在则接口返回冲突，忽略）——独立服务后走 :8085
log "重建演示 topic（kafka :8085）…"
for spec in "payment-events:支付事件流" "order-events:订单事件流" "user-events:用户事件流"; do
  NAME="${spec%%:*}"; DESC="${spec##*:}"
  curl -sf -m 5 -X POST "$KAFKA_ADDR/v1/kafka/topics" "${AUTH[@]}" \
    -d "{\"name\":\"$NAME\",\"env\":\"dev\",\"description\":\"$DESC\"}" >/dev/null || true
done

# 演示实例（创建幂等，admin 为 owner）——这是业务演示数据，不是授权：
# 谁能看到哪些实例由控制台行内「权限配置」授予的引擎元组决定，脚本不预置。
log "重建演示实例（redis :8086）…"
for spec in "cache-order:10.0.0.8:6379:订单缓存" "cache-user:10.0.0.9:6379:用户会话缓存" "cache-activity:10.0.0.10:6379:活动计数器"; do
  NAME="$(echo "$spec" | cut -d: -f1)"
  ADDR="$(echo "$spec" | cut -d: -f2-3)"
  DESC="$(echo "$spec" | cut -d: -f4)"
  curl -sf -m 5 -X POST "$REDIS_ADDR/v1/redis/instances" "${AUTH[@]}" \
    -d "{\"name\":\"$NAME\",\"addr\":\"$ADDR\",\"description\":\"$DESC\"}" >/dev/null || true
done

log "完成：authz + kafka + redis + zookeeper + rocketmq 已启动，菜单/资源类型从库读取（全新环境初始化见 scripts/init-env.sh），演示 topic/演示实例已就绪（admin 超管通配自举，MySQL 持久化重启不丢；终端用户权限请经控制台配置）。"
