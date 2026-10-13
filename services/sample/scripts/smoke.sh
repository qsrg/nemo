#!/usr/bin/env bash
# sample 服务冒烟脚本：内存模式起服务 → CRUD 全链路 curl → 错误体/请求头断言 → PASS/FAIL。
# 自身可重复执行：服务进程与临时文件由 trap 统一清理；任一断言失败即输出 FAIL 并以非 0 退出。
set -u
cd "$(dirname "$0")/.." # 切到 services/sample 根（main 包所在），相对路径才可用

PORT=$(awk '/^Port:/{print $2}' etc/sample-api.yaml)
BASE="http://127.0.0.1:${PORT}"

work="$(mktemp -d /tmp/nemo-sample-smoke.XXXXXX)"
server_pid=""
failures=0

# cleanup 停服务并清理临时目录，保证脚本可重复执行。
cleanup() {
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null
		wait "$server_pid" 2>/dev/null
	fi
	rm -rf "$work"
}
trap cleanup EXIT INT TERM

# fail 记录一项失败，最终统一汇报。
fail() {
	echo "FAIL: $*"
	failures=$((failures + 1))
}

# check 断言 $1 与期望 $2 相等，不等时报 $3。
check() {
	if [ "$1" != "$2" ]; then
		fail "$3（实得：$1）"
	fi
}

echo "== sample-api 冒烟 @ ${BASE}（内存模式）=="

# 端口预检：被占用（残留服务）时快速失败，避免误测到别人的服务。
if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then
	echo "FAIL: 端口 $PORT 已有服务响应（疑似残留进程），请先处理后重试"
	exit 1
fi

# 起服务：etc/sample-api.yaml 的 Mysql.Dsn 为空 → 内存模式，不依赖真实库。
go build -o "$work/sample-api" . || {
	echo "FAIL: go build 失败"
	exit 1
}
"$work/sample-api" -f etc/sample-api.yaml >"$work/server.log" 2>&1 &
server_pid=$!

# 等就绪：/healthz 探活，最多约 6s。
ready=0
for _ in $(seq 1 30); do
	if curl -fsS "$BASE/healthz" >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 0.2
done
if [ "$ready" -ne 1 ]; then
	fail "服务未在预期时间内就绪"
	echo "== 服务日志（尾部）=="
	tail -20 "$work/server.log"
	exit 1
fi

# 1. 健康探针。
check "$(curl -fsS "$BASE/healthz")" "ok" "/healthz 应返回 ok"
check "$(curl -fsS "$BASE/readyz")" "ok" "/readyz 应返回 ok"

# 2. Create 校验：空 name 应 400 SAMPLE_NAME_REQUIRED（校验先于存储）。
code=$(curl -s -o "$work/resp" -w '%{http_code}' -X POST "$BASE/v1/items" \
	-H 'Content-Type: application/json' -d '{"desc":"no name"}')
check "$code" "400" "空 name 创建应 400"
grep -q '"code":"SAMPLE_NAME_REQUIRED"' "$work/resp" ||
	fail "400 错误体应含 SAMPLE_NAME_REQUIRED（实得：$(cat "$work/resp")）"

# 3. Create 成功。
code=$(curl -s -o "$work/resp" -w '%{http_code}' -X POST "$BASE/v1/items" \
	-H 'Content-Type: application/json' -d '{"name":"smoke","desc":"demo"}')
check "$code" "200" "创建应 200"
ID=$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$work/resp")
[ -n "$ID" ] || fail "创建响应应含非空 id（实得：$(cat "$work/resp")）"

# 4. Get：读回创建内容。
code=$(curl -s -o "$work/resp" -w '%{http_code}' "$BASE/v1/items/$ID")
check "$code" "200" "查询应 200"
grep -q '"name":"smoke"' "$work/resp" ||
	fail "查询应返回创建内容（实得：$(cat "$work/resp")）"

# 5. Update：全量覆盖生效。
code=$(curl -s -o "$work/resp" -w '%{http_code}' -X PUT "$BASE/v1/items/$ID" \
	-H 'Content-Type: application/json' -d '{"name":"smoke2","desc":"demo2"}')
check "$code" "200" "更新应 200"
grep -q '"name":"smoke2"' "$work/resp" ||
	fail "更新应生效（实得：$(cat "$work/resp")）"

# 6. List：total 与现存记录数一致。
code=$(curl -s -o "$work/resp" -w '%{http_code}' "$BASE/v1/items")
check "$code" "200" "列表应 200"
grep -q '"total":1' "$work/resp" ||
	fail "列表 total 应为 1（实得：$(cat "$work/resp")）"

# 7. Delete 后复查：404 + 统一错误体（业务码 SAMPLE_NOT_FOUND，与 TEST 无关）。
code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$BASE/v1/items/$ID")
check "$code" "200" "删除应 200"
code=$(curl -s -o "$work/resp" -w '%{http_code}' "$BASE/v1/items/$ID")
check "$code" "404" "删除后查询应 404"
grep -q '"code":"SAMPLE_NOT_FOUND"' "$work/resp" ||
	fail "404 错误体应含 SAMPLE_NOT_FOUND（实得：$(cat "$work/resp")）"

# 8. X-Request-ID 回写：入站自定义 id 必须原样出现在响应头。
headers=$(curl -s -D - -o /dev/null -H 'X-Request-ID: smoke-echo-check' "$BASE/healthz")
echo "$headers" | grep -qi '^X-Request-ID: smoke-echo-check' ||
	fail "响应头应回写入站 X-Request-ID"

# 汇总。
if [ "$failures" -eq 0 ]; then
	echo "PASS"
else
	echo "== 共 $failures 项失败，服务日志（尾部）=="
	tail -20 "$work/server.log"
	exit 1
fi
