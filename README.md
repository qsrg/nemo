# nemo

nemo 是一个 Go 后端 monorepo：以 go-zero（api 服务形态）为服务框架，`kit/` 承载团队约定的代码化（错误体、日志、goctl 模板等），各业务服务独立成模块、经 `go.work` 组成 workspace 统一构建。

## 工具链

| 组件 | 版本 | 安装命令 |
|---|---|---|
| Go | 1.26.3 | 见 [go.dev/dl](https://go.dev/dl/)（本仓以 `go.work` 组织，需 Go ≥ 1.18 支持 workspace） |
| go-zero | v1.10.3 | `cd kit && go get github.com/zeromicro/go-zero@v1.10.3` |
| goctl | 1.10.2 | `go install github.com/zeromicro/go-zero/tools/goctl@v1.10.2` |

> go-zero v1.10.3 已由 kit/errors、kit/logging 实际引用；各服务模块经 kit 间接共享该版本。

## 常用命令（根目录执行）

```bash
make build            # 编译 workspace 全部模块（kit + services/*）
make test             # 全部测试
make lint             # go vet 全模块
make contract-check   # .api 契约与生成代码一致性校验（重新生成到临时目录后 diff）
make gen              # 重新生成全部服务的 goctl 代码（新增服务后在此登记）
```

说明：本仓为 Go workspace，`services/` 目录本身不是模块，Makefile 统一使用模块路径通配（`nemo/...`）而不是目录通配（`./services/...`）。`gen` 与 `contract-check` 依赖 Phase 0 后续任务交付的 `kit/template/goctl`（goctl 定制模板）与 `services/sample`（标准样板服务），在对应任务合入前不可用。

## 目录结构

- `kit/`：团队基础库，Phase 0 收缩为三包：errors（错误体与统一出口）、logging（日志初始化与中间件）、template（goctl 定制模板，后续任务新增）；不含业务逻辑，非强制依赖；
- `services/`：业务服务，一目录一模块。user/order demo 已移出至 /tmp/nemo-demo-backup（2026-09-06 决策，见 backend-design.md 迁移说明）；标准样板 `services/sample` 由 Phase 0 Task 5 创建；
- `docs/`：设计文档与规范（`architecture.md`、`backend-design.md` 服务接入指南、`permission-design.md` 等）；
- `console-frontend/`：控制台前端。

## 本地开发

依赖：MySQL 容器（3306，凭据见 `.local-dev.md`）与 CAS 容器（8443）需在运行。

首次/建库迁移（authz 业务库 `nemo_authz_biz` + 引擎库 `nemo_authz`；kafka 独立库 `nemo_kafka` 由 `scripts/restore-dev.sh` 内 provision 自动完成，均幂等）：

```bash
cd services/authz
MYSQL_DSN="root:<密码>@tcp(127.0.0.1:3306)/" go run ./cmd/provision
```

一键恢复本地环境（服务已合并：auth 登录链路并入 authz，业务数据全量 MySQL 持久化，重启不丢；菜单/按钮/资源类型数据在库，本脚本只负责拉起服务 + 幂等对账）：

```bash
bash scripts/restore-dev.sh
```

全新环境初始化（建库 + 迁移 + 菜单/资源类型基线 seed + 启动）：
```bash
bash scripts/init-env.sh
```

> 2026-09-21 服务合并：auth（原 :8083）并入 authz（:8084），登录/会话/用户目录链路收敛到 authz 单服务单端口；前端 vite 代理同步改指 8084。
> 2026-09-23 kafka 拆独立服务：Topic 管理演示域从 authz 拆出为 `services/kafka`（:8085），权限判定/授权名单经 `kit/authz` 远程调用集中式 authz（:8084）；业务数据自管独立库 `nemo_kafka`（topics/audits，不再复用 authz 的 nemo_authz_biz）；authz 内旧 kafka 代码与 nemo_authz_biz.topics 表已清理。前端微应用目录 `apps/order` → `apps/kafka`（端口 5275 不变），vite 代理 `/api/v1/kafka` 改指 8085。

## 文档索引

- **服务接入指南**：`docs/backend-design.md`（新建/接一个服务怎么写、怎么调、怎么测）
- **权限配置指南**：`docs/permission-guide.md`（菜单级/资源级/ABAC 策略怎么配、在哪配、场景速查）
- **设计文档**：`docs/superpowers/specs/`（含 Phase 0 的 `2026-09-06-backend-foundation-design.md`）
