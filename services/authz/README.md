# services/authz（授权服务）

基于 OpenFGA 的集中式授权模块：go-zero 服务（.api 契约 + goctl 生成）——三组路由（管理面 /v1/admin/*、判定面 /v1/authz/*、当前用户 /v1/me/*：permissions/menus/orgs）、JWT 鉴权、AdminRole 管理面中间件、健康检查；引擎以进程内嵌方式启动（loopback HTTP），业务逻辑（管理面/判定面/审计/对账/模型注册）已全部落地。

> 2026-09-23：kafka Topic 演示域已拆出为独立服务 `services/kafka`（:8085），判定面/relations 通用接口保留在 authz；业务服务（如 kafka）经 `kit/authz` Remote 接入，不再内嵌引擎。
> 同日清理：authz 内 kafka 路由/handler/logic/repo/types 全部删除（/v1/kafka/* 在 8084 已 404），
> 业务库 nemo_authz_biz 的 topics 表随数据迁入 nemo_kafka 后删除；通用名单逻辑依赖的
> kafka_topic 判定辅助收敛为 internal/logic/resourcegrantcommon.go。

## 目录

```
services/authz/
├── model.fga               # 首版授权模型 DSL（permission-design v1.1 §4.2，模型版本 v1）
├── authz.api               # API 契约（三组路由 + 类型；make gen / make contract-check 消费）
├── authz.go                # 服务入口（手写层：kit 接线 + 身份中间件 + healthz/readyz）
├── etc/authz-api.yaml      # 默认配置（Port 8084、引擎 127.0.0.1:18084、memory、dev secret）
├── migrations/             # SQL 迁移（000 建库 / 001 业务四表），由 cmd/provision 执行
├── cmd/
│   ├── bootstrap/          # 首次启动初始化（写库 bootstrap）
│   └── provision/          # 建库 + 迁移 runner（server 级 DSN）
├── go.mod                  # module nemo/services/authz
└── internal/
    ├── engine/             # 引擎包：Start/Handle/Check/ListObjects/EnsureModel/WriteTuples/Ping
    ├── authn/              # JWT 身份工具：Identity 中间件 / IdentityFrom / TestToken（测试签发）
    ├── middleware/         # AdminRole 管理面鉴权（claims.roles 含 AdminRoles 任一即过）
    ├── config/             # Config：RestConf + Auth + Engine + Mysql + AdminRoles
    ├── svc/                # ServiceContext：引擎装配 + 四个 biz repo（Dsn 空=内存，非空=MySQL）
    ├── repo/               # 业务库四 repo：接口契约 + 内存实现 + MySQL 实现
    ├── handler/            # goctl 生成（DO NOT EDIT）
    ├── types/              # goctl 生成（DO NOT EDIT）
    └── logic/              # 业务规则层（管理面/判定面/审计对账）
```

## 使用方式

```go
h, err := engine.Start(ctx, engine.Options{
    HTTPAddr:  "127.0.0.1:0", // 随机端口
    StoreName: "nemo-authz",
    Datastore: engine.DatastoreMemory, // 或 engine.DatastoreMySQL（配 MySQLDSN，见下）
})
defer h.Stop()

storeID, modelID, err := h.EnsureModel(ctx, string(modelDSLBytes))
_ = h.WriteTuples(ctx, []engine.Tuple{{User: "user:u1", Relation: "owner", Object: "order:o1"}})
allowed, _ := h.Check(ctx, "user:u1", "can_edit", "order:o1")
objects, _ := h.ListObjects(ctx, "user:u1", "can_view", "order")
```

## 命令

```bash
# 从仓库根
make build                      # 或：cd services/authz && go build ./...
make gen                        # 重新生成 goctl 骨架（handler/types）
make contract-check             # .api 契约与生成代码一致性校验
go test ./services/authz/...    # 单测（引擎 + authn + AdminRole + svc + repo 内存实现）
# 本地起服务（etc 默认值：8084，引擎 127.0.0.1:18084 memory）
cd services/authz && go run . -f etc/authz-api.yaml
# 建库 + 迁移（两轮幂等；凭据见 .local-dev.md，不进 git）
cd services/authz && MYSQL_DSN="root:***@tcp(127.0.0.1:3306)/" go run ./cmd/provision
# MySQL 真库集成测试（未设置则自动跳过）
cd services/authz && MYSQL_DSN="root:***@tcp(127.0.0.1:3306)/nemo_authz_biz?charset=utf8mb4" go test ./internal/repo/ -v
```

## 菜单与资源类型初始化

平台菜单**不在代码里硬编码默认清单（基线 seed 除外）**。现状（2026-10-11 菜单模型简化 + seed 化后）：

- 菜单经**管理面 CRUD 维护**（菜单管理页 / `POST /v1/admin/menus`），持久化在 `nemo_authz_biz` 重启不丢；全新环境由 `scripts/init-env.sh` 初始化（provision 执行 `migrations/008_menu_seed.sql` 基线 seed：菜单/按钮 + 资源类型注册表，INSERT IGNORE 幂等）；日常恢复只跑 `scripts/restore-dev.sh` 拉起服务；
- **资源类型由菜单/按钮声明驱动派生**（菜单行 `resourceType` ⇒ 隐含 can_view，资源级按钮 `relation` ⇒ 贡献出口；写路径调 `logic/resourcetypesync.go` 同步注册表与引擎模型），无独立注册文件；
- 历史路径（2026-10-11 已下线）：`deploy/menus.yaml` + `scripts/apply-config.py` 曾为菜单初始化下发源——删除，菜单事实源统一为库（管理面 CRUD + 008 seed 基线）。

新增资源类型：在菜单管理页为页面/按钮声明 `resourceType`/`relation`（类型自动派生注册）→ 管理面 CRUD 维护。设计背景见 docs/permission-design.md §6 与 docs/menu-simplify-design.md。

**按钮实体化（2026-09-28）**：`menus` 表行分两类——`type=menu`（导航，默认）与 `type=button`（按钮级授权实体，`parent_id` 挂父菜单下，不入侧边栏）。两类按钮语义：

- **页面级按钮**（`relation` 空，如 新建/导出）：无资源实例的操作，授权 = 功能权限勾选，`permission_code` 必填（`服务:动作` 形态，如 `kafka-topics:create`）；
- **资源级按钮**（`relation` 非空 + `resourceType` 必填，如 冻结/共享某行）：有资源实例的操作，无权限码，授权跟随该资源「数据权限」三档（`can_view/can_edit/can_delete`），授权界面只读展示、不单独勾选——**复用优先**：常见操作直接映射既有 `can_*` 关系，不新增语义。

- 授权界面（Grants）按菜单聚合展示按钮：页面级 = 功能权限勾选；资源级 = 只读标签（跟随三档）。

**三级菜单（2026-09-28，2026-10-08 改 id 引用）**：菜单行挂 `parent_id` 形成层级——`parent_id` 空 = 顶级（服务首页/一级），非空 = 挂父菜单行下，沿父链校验层级最多三级（顶级/一级/二级，二级不能再作父级）；按钮行父级可为任意菜单行，禁止按钮再挂按钮。删除菜单递归级联删除后代（子菜单行 + 按钮行）并回收孤儿授权码。`/v1/me/menus` 下发 `parentId` 供基座组装嵌套侧边栏（服务分组按 path 首段）；菜单管理父级选择树与授权矩阵均为同构三级结构。

## 配置说明

- `Port`：服务监听端口，默认 8084。
- `Auth.AccessSecret/AccessExpire`：JWT 鉴权；etc 默认为 dev 值，**真实 secret 生产由环境变量注入**。
- `Engine.HTTPAddr/StoreName/Datastore/MySQLDSN`：内嵌引擎（回环 HTTP + go-sdk client，禁止 import 引擎内部包）；`Datastore` dev 默认 `memory`，生产切 `mysql`（启用方式见下"引擎 MySQL datastore"）。
- `Mysql.Dsn`：业务库（规则/资源类型/模型版本/审计）连接串，空 = 内存模式（默认）；非空 = MySQL 模式（先 `cmd/provision` 建库迁移，连接失败启动即 panic 快速失败）。
- `AdminRoles`：管理面放行的平台角色，默认 `["super_admin"]`（claims.roles 含任一即过；user 一律取 claims.sub）。

## 鉴权说明

- 三组路由均需 JWT（`Authorization: Bearer <token>`，无 token 401）。
- 管理面 `/v1/admin/*` 额外经 AdminRole 中间件：roles 不含 `AdminRoles` 任一 → 403 `AUTHZ_FORBIDDEN`。
- go-zero v1.10.3 的 jwt 中间件只把**非标准** claims 注入 ctx（标准 `sub` 被丢弃），故 `internal/authn` 的 Identity 中间件在同一 secret 下复解析 token，把 `sub`/`roles` 类型化注入 ctx——业务层统一经 `authn.IdentityFrom(ctx)` 取当前用户。
- `/healthz` 恒 200；`/readyz` 由引擎端口拨号（`engine.Handle.Ping`）判定，失败 503。
- 测试 token：`authn.TestToken(secret, sub, roles)`（HS256，1 小时）。

## 引擎 MySQL datastore（Options.Datastore = mysql）

- 配置：`Datastore: mysql` + `MySQLDSN`（go-sql-driver DSN 形态，指向**引擎库 nemo_authz**，与业务库 Mysql.Dsn 指向的 nemo_authz_biz 是两个库）。
- 迁移：**启动时自动执行** OpenFGA 自带 schema 迁移（goose 管理 store/authorization_model/tuple/assertion 等表，幂等，重装/重启只跳过），无需单独运行 `openfga migrate`；表未就绪引擎不会对外服务。DSN 会强制 `parseTime=true`（时间列读取依赖）。
- 业务库（Mysql.Dsn → nemo_authz_biz）的建库/迁移仍走 `cmd/provision`，与引擎库迁移相互独立。

## 运维注意

- **多副本首启**：引擎 MySQL 迁移虽幂等，但多副本同时首启存在迁移竞态窗口——首次部署须串行启动（或先跑一次单实例/前置脚本完成迁移）后再扩副本。

## CAS 组织架构同步（orgsync）

- **事实源分工**：组织树与人员归属由 CAS 统一维护并对外提供目录接口（`GET /orgs`、`GET /users`，本地 mock 为 cas-test 容器 8443），本系统不做组织事实源，只做同步副本落授权系统；授权关系（组织 × 权限点 can 元组）为本系统独有职责，同步器**绝不触碰**。
- **同步语义**（`internal/orgsync`，全量对账）：拉 CAS 全量 → 与本系统现状（orgs 注册表 + 引擎 parent/member 元组）diff → 增/删/改收敛。只写删 CAS 拥有的两类关系（`org:{parent}#parent@org:{child}`、`org:{id}#member@user:{uid}`）；幂等，无差异零动作；撤权时效 = 同步周期。
- **触发方式**：定时（`CasSync.Enabled` + `CasSync.Interval`，启动即先跑一轮）与管理面手动触发 `POST /v1/admin/orgs/sync` 同一入口。`CasSync.BaseURL` 与 auth 服务 `Cas.BaseURL` 同源。
- **删除安全**：CAS 已撤销的组织若仍被授权引用（权限授予/角色承担，经 ListObjects 反查——引擎 v1.19 Read 不支持 user 维度过滤）或被子组织 parent 引用，跳过删除并告警，人工处理授权后下轮自动删除。
- **orgs 注册表持久化**：`Mysql.Dsn` 非空时组织元数据（名称/父子关系）落 MySQL（`orgs` 表，migrations/004_orgs.sql），随 CAS 同步自动收敛，不再依赖手工重建。

## 引擎依赖

OpenFGA 三模块：openfga v1.19.0、go-sdk v0.8.2、language/pkg/go v0.3.2-pre（另含其必需传递依赖 api/proto，用于 grpc-gateway HTTP 注册）；MySQL 后端另复用 openfga 模块内的 pkg/storage/migrate（goose）与 pkg/storage/mysql datastore。
