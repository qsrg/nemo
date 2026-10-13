# 后端开发规范（服务接入指南）

面向各服务团队的后端开发指南。架构决策见 `architecture.md`（§3 选型、§4 网关、§5 kit、§6 结构与边界），本文回答"我要新建/接一个服务，怎么写、怎么调、怎么测"。

> **迁移说明（v2.3）**：本文描述 go-zero 目标结构。`services/user`、`services/order` 为 v2.1 验证 demo，**2026-09-06 决策废弃并移出**（备份于 `/tmp/nemo-demo-backup`，git 基线 39eabbc 亦可考）；kit v2.1 冻结包（auth/config/httpx/middleware/reqid）随之退役删除，kit 收缩为 `errors`/`logging`/`template`（+规划中的 `authz`）。新服务一律按本文创建，标准样板见 `services/sample`（Phase 0 交付，specs/2026-09-06-backend-foundation-design.md）。

## 1. 技术栈与仓内角色

Go 1.26.3 + **go-zero v1.10.3**（api 服务形态，不引入 zRPC / proto），骨架生成用 **goctl 1.10.2**（安装命令见根 README）。服务骨架由 goctl 生成（模板为 kit 定制版，含本团队的 repo/client 分层与 kit 接线）。存储访问用 `database/sql` 或对应官方驱动，不用 ORM。版本由 `kit/go.mod` 统一锁定，各服务模块经 kit 间接共享，不单独 require。

| 目录 | 角色 | 谁来改 |
|---|---|---|
| `kit/`（`errors/` `logging/` `template/` `authz/` 均已交付） | 团队约定的代码化：错误体、日志字段与轮转、goctl 模板、authz 判定 client（Check / ListObjects 封装，permission-design.md §5.3） | 双人审批（架构 §5），业务团队不改 |
| `services/<name>/` | 业务服务，一个目录一个服务（`gateway`、`auth` 为规划中的平台服务） | 各服务团队自治 |
| `go.work` | workspace，本地直接 `go run .` | 新增服务时确认登记（见 §3） |

`nemo/kit` 仅在 workspace 内可解析（无版本发布，monorepo 约束）：仓库内服务 go.mod **不写** kit 的 require/replace；workspace 外临时实验才需 `replace nemo/kit => <本仓绝对路径>/kit`。

kit 三个纪律（PR 评审硬性检查）：不含业务逻辑、不出现业务名词（`user`/`order` 出现在 kit 中直接拒绝）；不做运行时治理；不是强制依赖。

## 2. 服务目录结构（go-zero 骨架 + 本团队约定）

```
services/<name>/
├── <name>.api              # 契约：路由与类型的单一事实源（goctl 输入，归本服务所有）
├── <name>.go               # goctl 生成入口：以 .api 文件名命名（sample.api → sample.go），非 main.go
├── etc/<name>-api.yaml     # go-zero 配置：以 .api 的 service 块名命名（惯例 <name>-api，如 sample-api.yaml）
├── cmd/migrate/            # SQL 迁移 runner（可选）：按序执行 migrations/*.sql + 版本表，MYSQL_DSN 必填
├── migrations/             # SQL 迁移脚本（可选）
├── scripts/                # 本地脚本（可选，如 smoke.sh 冒烟）
├── internal/
│   ├── config/             # 配置结构体
│   ├── handler/            # goctl 生成：参数绑定、调 logic、写响应（不含业务规则）
│   ├── logic/              # 业务规则层（服务核心，单测重点）
│   ├── svc/                # ServiceContext：显式装配依赖，无 DI 容器
│   ├── types/              # goctl 生成的请求/响应类型
│   ├── repo/               # 存储访问（接口不变，生产换 MySQL 实现）
│   └── clientx/            # 跨服务调用基座样板（复制自 sample 按需裁剪，见 §5）
└── go.mod                  # module nemo/services/<name>（goctl 生成后修正，见 §3 第 3 步）
```

分层规则：

- **依赖方向单向**：handler → logic → repo/client，禁止反向、禁止跨层（架构 §6.3）
- handler 只做协议转换（绑定、调 logic、写响应），业务规则在 logic；repo 的存储模型不外泄（logic 对外用输入/输出结构体）
- `internal/` 以外的包不存在（Go 机制保证服务间不互相 import 业务代码）
- handler / types 由 goctl 生成后**不手改**：改契约改 `.api` 文件重新生成，保证实现与契约一致（CI 校验）

## 3. 新建一个服务（checklist）

每一步在**对照标准 `services/sample`**（模块 `nemo/services/sample`，service 名 `sample-api`）里都有实例。前置：goctl 1.10.2 在 PATH（安装见根 README）。

1. **建目录并写 `<name>.api` 契约**（对照 `sample.api`）：

   ```bash
   mkdir -p services/<name>
   ```

   - 路由前缀 `/v1/`；service 块名 `<name>-api`（goctl 用它命名 etc 配置文件与日志服务名）
   - **JSON 请求体字段必须标 `json:"...,optional"`**：goctl 1.10.2 对未标 optional 的 body 字段生成 form 必填语义，go-zero 对 JSON POST/PUT 也先走 ParseForm，字段缺失直接报错并被全局错误出口转 500（实测坑）；必填语义由 logic 层自行校验
   - 错误码域前缀 = 服务名大写（`<NAME>_`），错误体格式见 §4.2

2. **生成骨架**（在 `services/<name>/` 下执行，目录里**不能预置 go.mod**——goctl 检测到已存在 go.mod 会直接报错中止）：

   ```bash
   goctl api go -api <name>.api -dir . --home ../../kit/template/goctl
   ```

   kit 定制模板自动带：main 三行接线（`kitlog.Init` / `kiterrors.InstallErrorHandler` / `server.Use(kitlog.RequestID)` + `server.Use(kitlog.AccessLog)`，RequestID 必须最外层）、repo/clientx 分层目录、注释规范；并在 workspace 内时自动把本服务登记进 `go.work`。
   **exists-skip 坑**（goctl 1.10.2）：已存在的生成文件不会被覆盖——改契约后必须先删 `internal/handler/`、`internal/types/` 等旧生成文件再重新生成。

3. **修正 go.mod 与 import 前缀**（goctl 自动 `go mod init` 的 module 名取目录 basename，如 `demo`，必须手工改为 `nemo/services/<name>` 并同步替换全部生成文件的 import 前缀，否则 workspace 无法解析）：

   ```bash
   sed -i '' 's|^module <basename>$|module nemo/services/<name>|' go.mod
   grep -rl '<basename>/internal' --include='*.go' . | xargs sed -i '' 's|<basename>/internal|nemo/services/<name>/internal|g'
   gofmt -w .          # 改完 import 必须 gofmt（import 会重排序），否则 contract-check diff 因格式差异失败
   go mod tidy         # 补 go-zero require 与 go.sum；nemo/kit 由 workspace 解析，无需 require/replace
   ```

4. **登记三处**：`go.work` 已由 goctl 自动写入（生成后核验一次）；根 Makefile `gen` 目标登记重生成命令；端口顺延 §7 表并在本文档登记。

5. **装配依赖**：`internal/svc` 的 ServiceContext 显式 new repo/clientx 注入（无 DI 容器，参照 sample）；repo 从接口 + 内存实现起步（`internal/repo`），需要真库时再加 MySQL 实现与 `cmd/migrate` 迁移 runner（runner 需 `MYSQL_DSN` 环境变量，见 §9）；需要调上游时复制 `services/sample/internal/clientx` 整目录到本服务 `internal/` 下按需裁剪。

6. **健康检查**（模板不生成，手工加）：main 里 `server.AddRoute` 注册 `/healthz`（恒 200）与 `/readyz`（就绪判定，依赖未就绪 503），照抄 `services/sample/sample.go` 尾部两段。

7. **编译与契约校验**（仓库根执行）：

   ```bash
   make build && make lint
   make contract-check   # 新服务在 Makefile 照 sample 段加自己的比对块
   ```

8. **本地验证**：`go run .` 后 `curl localhost:<port>/healthz` 与 `/readyz` 返回 ok，CRUD 冒烟参照 `services/sample/scripts/smoke.sh`（可整脚本改名复用）。

## 4. API 契约与错误规范

### 4.1 路由与契约

- 路由在 `.api` 文件中声明，路径前缀 `/v1/`；`.api` 文件**归本服务所有**：消费方以其为准，禁止为调用方加字段
- 成功响应直接返回资源 JSON，**不包信封**；分页统一 `{"items": [...], "total": n}`
- 公开路由（登录回调等）与业务路由在 `.api` 中分组声明，认证在路由组级配置（go-zero rest 的 jwt 配置）；`/healthz` `/readyz` 在认证之外

### 4.2 错误体（全栈统一，前端也依赖此格式）

```json
{"error": {"code": "USER_NOT_FOUND", "message": "用户不存在", "request_id": "..."}}
```

- code 用 `<域>_<原因>` 大写下划线，全局唯一，域前缀 = 服务名（`USER_`、`ORDER_`；sample 示范用 `SAMPLE_`，如 `SAMPLE_NOT_FOUND`、`SAMPLE_NAME_REQUIRED`）
- logic / handler 层用 kit 构造器保证同语义同状态码：`BadRequest` / `Unauthorized` / `Forbidden` / `NotFound` / `Conflict` / `Internal`
- 统一出口写 JSON 响应；`Internal` 不把原始 err 写进响应体（只进日志）

## 5. 跨服务调用规范

- 调用代码放自己 `internal/clientx/`（基座样板复制自 `services/sample/internal/clientx`，按需裁剪出入参与目标地址），**上游响应类型按需自建**（只含本服务用到的字段），不建共享 model 包——上游加字段不影响下游编译
- 统一封装在 client 基座内：自动透传 `X-Request-ID` 与 `Authorization: Bearer`（鉴权一次全链生效）、超时默认 5s、go-zero breaker 熔断、非 2xx 解析为 `*kiterrors.Error` **原样带回上游错误码**（调用方按 `USER_NOT_FOUND` 等错误码 switch 处理）
- 目标地址：demo 用环境变量（如 `SAMPLE_SERVICE_URL`）；生产由注册中心实例缓存提供（etcd 原生 / Consul 官方适配，架构 §3.3），业务代码不变

## 6. 认证（CAS + API key，架构 §4.2）

平台有两类调用方，认证路径不同：

- **人（浏览器）**：登录在 authz 服务完成（2026-09-21 起 auth 链路并入 authz，端口 8084）——前端跳 CAS 登录、回调带 ticket，authz 调 CAS `serviceValidate` 换用户信息后**签发平台 JWT**（HS256，secret 走环境变量 `JWT_SECRET`，全栈同一 secret）。业务服务不感知 CAS，只认平台 JWT
- **程序（agent / 脚本 / 第三方）**：API key，authz 服务签发与管理，网关校验
- 生产流量入口统一过自研网关（认证 + 粗粒度授权，架构 §4.1），服务内 go-zero rest 的 jwt 中间件兜底；**细粒度资源级授权（归属、租户隔离）在服务内做**；`/healthz` `/readyz` 及公开路由在认证之外
- context 取值：handler 不直接读 Header，从 context 取用户身份与 request id

## 7. 配置与端口

- 配置 = `etc/<name>.yaml` + 环境变量覆盖（go-zero config），**所有配置必须有默认值**，生产值由 deploy/compose 注入；不做配置中心
- 命名：`Port`、`JwtSecret`、跨服务目标 `XXX_SERVICE_URL`（生产注册中心接管后废弃）
- 端口分配（连续顺延；新服务取最小空闲号并在本文档登记）：

| 服务 | 端口 | 状态 |
|---|---|---|
| gateway | 8080 | 规划中（服务未建） |
| user | 8081 | 冻结：v2.1 demo 已移出 workspace，端口保留不再维护 |
| order | 8082 | 冻结：v2.1 demo 已移出 workspace，端口保留不再维护 |
| auth | 8083 | 已并入 authz（2026-09-21 服务合并：登录/会话/用户目录链路迁入 authz，端口 8083 停用；同日 services/auth 目录移至 /tmp/nemo-services-auth-backup-20260921，nemo_auth / nemo_auth_biz 库已删除） |
| authz | 8084 | 在用（权限服务，Phase A/B 已交付：管理面+判定面+审计；2026-09-21 起含原 auth 登录链路） |
| kafka | 8085 | 在用（2026-09-23 独立微服务：Topic 管理演示域从 authz 拆出，独立业务库 nemo_kafka，判定/名单经 kit/authz 远程调 authz；与 sample 样板默认端口冲突，本地不同时起） |
| sample | 8086 | 标准样板（Phase 0 交付；默认 8085，与 kafka 冲突后顺延为 8086） |
| 新服务 | 8087、8088…（在本文档登记） | — |

- 环境变量（跨服务目标定址，生产注册中心接管后废弃）：

| 环境变量 | 用途 | 示例 |
|---|---|---|
| ~~AUTHZ_SERVICE_URL~~ | 已废弃：auth 并入 authz（2026-09-21），跨服务角色获取改为进程内引擎判定 | — |

## 8. 可观测与生命周期

- 日志：go-zero logx + kit `logging` 统一字段与轮转初始化；字段约定 `method/path/status/duration_ms/request_id`；生产输出 JSON 文件并轮转（架构 §9.2），本地输出 stdout
- 请求 id：中间件全链注入/透传，响应头回写 `X-Request-ID`，排障以它串联前端日志、网关日志、服务日志
- panic recovery：go-zero rest 内置
- `/metrics` 由 go-zero 自动暴露；`/healthz`（存活）与 `/readyz`（就绪）显式注册：发布摘流量、启动探针都依赖 readyz，依赖未就绪时 ready 函数返回 false
- 优雅停机框架内置：SIGTERM 后停止接新请求，存量请求上限 30s

## 9. 本地开发

以 sample 为例（`go run .` 必须在服务根目录执行——main 包所在目录，`-f` 配置路径相对 cwd 解析）：

```bash
cd services/sample
go run .                       # 内存模式起步（etc/sample-api.yaml 的 Mysql.Dsn 为空，不依赖真库）
curl localhost:8085/healthz    # 存活探针 → ok
curl -X POST localhost:8085/v1/items -H 'Content-Type: application/json' -d '{"name":"n1","desc":"d1"}'
curl localhost:8085/v1/items   # {"items":[...],"total":1}
scripts/smoke.sh               # CRUD 全链冒烟（自动起停服务与清理，PASS/FAIL）

# 迁移（接入 MySQL 时）：MYSQL_DSN 必填，未设置直接报错退出，绝不静默跑空
MYSQL_DSN="user:pass@tcp(127.0.0.1:3306)/nemo?charset=utf8mb4" go run ./cmd/migrate
```

全仓命令在仓库根执行：`make build` / `make test` / `make lint` / `make gen` / `make contract-check`（workspace 模式统一用模块路径通配 `nemo/...`）。

- 单服务调试只需本服务 + 其依赖的上游服务（env 定址 `XXX_SERVICE_URL`）
- 冻结服务：user/order（8081/8082）已随 v2.1 demo 移出 workspace，不再维护（备份 `/tmp/nemo-demo-backup`，git 基线 39eabbc），调试链路不再涉及
- 契约测试对 `.api` 文件：改接口先改 `.api` 再重新生成（注意 §3 exists-skip 坑），评审以契约为准
