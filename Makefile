# 根 Makefile：全仓构建与校验入口（版本与工具链说明见 README.md）
# GOZERO_VERSION：kit 解析到的 go-zero 版本（用 go list 读取，与 go.mod 书写格式无关）
GOZERO_VERSION := $(shell cd kit && go list -m -f '{{.Version}}' github.com/zeromicro/go-zero)

# 说明：本仓为 Go workspace，kit 与各 service 均为独立模块，services/ 本身不是模块；
# 目录通配 ./services/... 在 workspace 模式下无法解析（前缀不在任何模块内），
# 故统一使用模块路径通配 nemo/...（新增服务沿用 nemo/services/<name> 命名即自动覆盖）。
.PHONY: gen build test lint contract-check

# gen: 重新生成全部服务的 goctl 代码（新增服务后在此登记）
gen:
	cd services/sample && goctl api go -api sample.api -dir . --home ../../kit/template/goctl
	cd services/authz && goctl api go -api authz.api -dir . --home ../../kit/template/goctl

# build: 编译 workspace 全部模块（kit + services/*，新增服务自动纳入）
build:
	go build nemo/...

# test: 全部测试
test:
	go test nemo/...

# lint: go vet 全部模块
lint:
	go vet nemo/...

# contract-check: .api 契约与生成代码一致性（重新生成到临时目录后 diff）
# 实测约束（goctl 1.10.2，Task 6 修复的两个缺陷 + Task 7 修正比对范围）：
#   1. -dir 必须用真实绝对路径（/private/tmp/...）：/tmp 是符号链接，goctl 会把
#      路径相对化拼错（stat <cwd>/private/tmp/...）；仓库内相对路径 + 预置 go.mod
#      的组合会触发 go mod init 冲突直接中止生成，同样不可用。
#   2. 生成前必须在目标目录预置同 module 名的 go.mod，否则 goctl 自动生成的
#      go.mod 用目录 basename 做 module 名，import 前缀漂移导致 diff 必败。
#   3. 比对范围：sample 的 handler/types 全量 diff（无手写偏离）；authz 的
#      handler 函数存在性校验 + types 结构化校验。logic 标注 Safe to edit，
#      填充业务后与生成桩永不一致，契约由 .api → types/handler 传递覆盖。
#      diff 排除 *_test.go（goctl 不生成测试，logic 手写单测在被比对目录旁）。
#   4. auth 已于 2026-09-21 并入 authz（services/auth 移至 /tmp，库已删），
#      .api 契约随 authz 校验，auth 不再单独参与 contract-check。
#   5. authz 的 handler 不做文件级 diff：policy/attribute/syncorgs 系列按业务
#      聚合为手写文件（policyhandler.go/attributeshandler.go/syncorghandler.go），
#      routes.go 为手写注册（含 2026-09-21 服务合并迁入的 auth 路由），文件组织
#      与 goctl 默认"一 handler 一文件"必然不同。改为函数存在性校验：.api 的
#      @handler 集合必须 ⊆ 真实 handler 目录已实现函数（编译已保证可注册）。
#   6. authz 的 types 对比用 scripts/check-types.py 做结构化校验：
#      忽略注释与空白（goctl 不复制字段注释；手写字段未对齐 vs 生成已对齐），
#      严格对比类型集合、字段名/tag/顺序。
# mktemp 每次唯一目录，并行互不干扰；trap 保证中途失败也会清理临时目录。
contract-check:
	@set -e; \
	work=$$(mktemp -d /private/tmp/contract-check.XXXXXX); \
	trap 'rm -rf "$$work"' EXIT INT TERM; \
	mkdir -p "$$work/sample" "$$work/authz"; \
	printf 'module nemo/services/sample\n\ngo 1.26.3\n' > "$$work/sample/go.mod"; \
	printf 'module nemo/services/authz\n\ngo 1.26\n' > "$$work/authz/go.mod"; \
	cd services/sample && goctl api go -api sample.api -dir "$$work/sample" --home ../../kit/template/goctl >/dev/null; \
	cd ../authz && goctl api go -api authz.api -dir "$$work/authz" --home ../../kit/template/goctl >/dev/null; \
	cd ../..; \
	diff -r -x '*_test.go' services/sample/internal/handler "$$work/sample/internal/handler"; \
	diff -r -x '*_test.go' services/sample/internal/types "$$work/sample/internal/types"; \
	python3 scripts/check-types.py services/authz/internal/types/types.go "$$work/authz/internal/types/types.go"; \
	grep -oE '@handler [A-Za-z]+' services/authz/authz.api | awk '{print $$2}' | sort -u > "$$work/authz_api_handlers"; \
	grep -hoE '^func [A-Za-z]+Handler' services/authz/internal/handler/*.go | sed -E 's/^func ([A-Za-z]+)Handler.*/\1/' | sort -u > "$$work/authz_real_handlers"; \
	missing=$$(comm -23 "$$work/authz_api_handlers" "$$work/authz_real_handlers"); \
	if [ -n "$$missing" ]; then \
		echo "authz: .api 声明但未实现的 handler:"; printf '%s\n' "$$missing"; exit 1; \
	fi; \
	echo "contract-check OK"