# 权限模块 Phase A（M0~M3：authz 服务 + kit/authz）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 authz 服务（内嵌 OpenFGA 引擎、管理 API、判定 API、bootstrap、审计先行+对账）与 kit/authz 判定 client，sample 服务挂 Require 冒烟打通——auth 服务（M4）与前端（M5）另立 Phase B 计划。

**Architecture:** OpenFGA 以 Go module 内嵌（回环 HTTP + 官方 client，不 import 内部包）；管理面（biz 库：规则表/注册/审计/模型版本）与判定面（引擎）同进程不同路由组；管理面鉴权 = JWT claims.roles 含 super_admin（一期固定）；判定 fail-closed。

**Tech Stack:** Go 1.26 + go-zero v1.10.3 + goctl 1.10.2 + OpenFGA（≥ v1.18.x，实际版本任务内落档）+ openfga go-sdk + openfga language（DSL 解析）+ database/sql。

**Spec:** `docs/superpowers/specs/2026-09-06-authz-implementation-design.md`（§3 组件设计、§4 里程碑、§5 测试策略）；授权模型语义以 `docs/permission-design.md` v1.1 §4 为准。

## Global Constraints

- 任务边界 = commit，审查基于 git diff；"打标" = 计划勾选 + SDD ledger + TodoWrite 同步
- Phase 0 基座（直接消费）：kit/errors（构造器 `(code,message)`、`InstallErrorHandler`、`Decode`、`NotFound/Forbidden/BadRequest/Internal`）、kit/logging（`Init`、`RequestID` 最外层 + `AccessLog`）、kit/template/goctl（main.tpl 自带三行接线）、Makefile `nemo/...` 通配、contract-check
- 错误码前缀 `AUTHZ_`；判定明确无权 403 `AUTHZ_DENIED`、引擎不可达/内部错 503 `AUTHZ_SERVICE_UNAVAILABLE`（fail-closed 绝不放行）
- 引擎访问只经官方 go-sdk HTTP client（回环 127.0.0.1:18084 配置化）；**禁止 import openfga 引擎内部包**（护栏：只许 import pkg/server、pkg/storage/*、go-sdk、language）
- 管理面鉴权一期固定校验 JWT claims.roles 含 `super_admin`；user 一律取 claims.sub（.api 用 go-zero jwt 中间件，secret 走配置默认 dev 值）
- 引擎 datastore：dev 默认 memory；MySQL datastore（nemo_authz 库）做成配置路径并在 Task 8 验证可切换（生产形态）
- MySQL 凭据在 `.local-dev.md`（不进 git）；etc yaml 默认 DSN 留空 = 内存/跳过；仓库内任何文件不得出现 root 密码
- bootstrap 幂等（read-before-write：OpenFGA 对已存在 tuple 的重复写入会报错）；初始 API key 推迟 M4（spec 偏差注记）
- AGENTS.md：中文注释；logx 日志禁 fmt.Println；repo 参数化 SQL
- 用户并行改动（console-frontend/、docs/frontend-design.md、README.md）绝不卷入 commit

---

### Task 1: M0 引擎 PoC——内嵌 OpenFGA + 模型加载 + Check/ListObjects 冒烟

**Files:**
- Create: `services/authz/model.fga`（首版授权模型，permission-design v1.1 §4.2）
- Create: `services/authz/internal/engine/engine.go`、`services/authz/internal/engine/bootstrap.go`
- Create: `services/authz/go.mod`（module nemo/services/authz）
- Test: `services/authz/internal/engine/engine_test.go`

**Interfaces:**
- Produces（Task 2-8 消费）:
  - `engine.Options{HTTPAddr string; StoreName string; Datastore string /* memory|mysql */; MySQLDSN string}`
  - `engine.Start(ctx context.Context, opt Options) (*Handle, error)`——进程内启动引擎 server（回环），返回 Handle
  - `type Handle struct`；`func (h *Handle) APIURL() string`；`func (h *Handle) EnsureModel(ctx context.Context, modelDSL string) (storeID, modelID string, err error)`——幂等：按名找 Store（无则建）、DSL 经 language transformer 转 JSON 后写入模型（与 biz 模型版本比对逻辑 Task 6 接管，本任务总是新建）
  - `func (h *Handle) WriteTuples(ctx, tuples []Tuple) error`（幂等 read-before-write）；`type Tuple{User, Relation, Object string}`
  - `func (h *Handle) Check(ctx, user, relation, object string) (bool, error)`；`func (h *Handle) ListObjects(ctx, user, relation, objType string) ([]string, error)`
- 模型 DSL（model.fga 全文，Task 6 注册机制在此之上扩展）:

```
model
  schema 1.1

type user

type org
  relations
    define parent: [org]
    define admin: [user] or admin from parent
    define member: [user] or admin or member from parent

type platform_role
  relations
    define assignee: [user]

type order
  relations
    define org: [org]
    define owner: [user]
    define sharer: [user] or owner or admin from org
    define editor: [user] or owner
    define viewer: [user] or editor
    define member_view: viewer or member from org
    define can_view: member_view
    define can_edit: editor
    define can_delete: owner
    define can_share: sharer
```

- [x] **Step 1: 引入依赖并核对版本**

```bash
mkdir -p services/authz && cd services/authz
go mod init nemo/services/authz && go mod edit -go 1.26
go get github.com/openfga/openfga@latest github.com/openfga/go-sdk@latest github.com/openfga/language/pkg/go/transformer@latest
go list -m github.com/openfga/openfga   # 记录版本，须 >= v1.18（permission-design §8）
go mod edit -require nemo/kit@v0.0.0 -replace nemo/kit=../../kit 2>/dev/null || true
go mod tidy && go build ./...
```

Expected: 依赖解析成功且 openfga ≥ v1.18.x（若解析版本 < v1.18 报 BLOCKED 并附版本证据）。模块根加 go.work 登记（`use ./services/authz`）。

- [x] **Step 2: 核对引擎装配与 SDK API 表面**

```bash
go doc github.com/openfga/openfga/pkg/server | head -40
go doc github.com/openfga/openfga/pkg/server New
go doc github.com/openfga/openfga/pkg/storage/memory New
go doc github.com/openfga/go-sdk/client NewClient
go doc github.com/openfga/language/pkg/go/transformer TransformerDSLToJSON
```

Expected: 确认 server 装配选项（WithDatastore/WithHTTPServerAddr 或等价）、memory datastore 构造、client 配置结构、DSL→JSON 转换函数签名。**以实际 API 为准实现 Step 4，出入记录进报告**（Phase 0 已验证此模式可行）。

- [x] **Step 3: 写 model.fga**

按上方 Interfaces 中 DSL 全文落盘 `services/authz/model.fga`（含中文头注释：来源 permission-design v1.1 §4.2，模型版本 v1）。

- [x] **Step 4: 实现 engine 包**

`engine.go` 要点（全部中文注释）：`Start` 装配 server（memory datastore；mysql 分支本任务留 `NotImplemented` 占位返回 error，Task 8 填实）→ goroutine `Run(ctx)` → 等待 HTTP 端口就绪（dial 重试 ≤5s）→ 构造 go-sdk client。`bootstrap.go`：EnsureModel（transformer 解析 model.fga → CreateStore 幂等按名 → WriteAuthorizationModel）与 WriteTuples（写前 Read 校验存在即跳过，实现幂等）。**不 import 除四个许可包外的任何 openfga 包。**

- [x] **Step 5: 写失败测试（PoC 断言组）**

`engine_test.go`：`Start`（memory）→ EnsureModel（读 model.fga）→ 写 tuples 构造四类场景：

```
org:root(parent 无)、org:child(parent org:root)
user:u1 member org:root；user:u2 无任何关系；user:u3 viewer order:o2
order:o1 org org:child；order:o2 org org:child
```

断言（全部经 Handle API）：①租户隔离：u2 对 o1/o2 的 can_view 均 false ②层级继承：u1 对 o1 can_view=true（member from parent 链）③共享：u3 对 o2 can_view=true、can_share=false ④owner：owner(order:o1)=u1 时 can_edit/can_delete/can_share=true ⑤ListObjects(u1, can_view, order) 含 o1 不含 u3 专属 o2（o2 也在 u1 可见则断言含 o1 即可，按实际写）。测试后 t.Cleanup 停 server。

- [x] **Step 6: 跑通并落档**

Run: `cd services/authz && go test ./internal/engine/ -v && go build ./...`
Expected: PASS。**M0 结论落档**：解析的 openfga/go-sdk/language 三版本、装配 API 实际形态、与 plan 假设的差异——写进报告；若引擎嵌入有阻断性缺陷（如 server 无法进程内启动），立即 BLOCKED 上报（退路：引擎独立容器方案重新评审）。

- [x] **Step 7: 打标**

commit（仅 Task 1 文件，subject "Task1(M0): OpenFGA 内嵌 PoC——引擎包 + 模型加载 + 四场景断言"），勾选，ledger，TodoWrite。

---

### Task 2: M1a authz 服务骨架（.api 契约 + goctl 生成 + 健康检查）

**Files:**
- Create: `services/authz/authz.api`、etc/authz-api.yaml、internal/{config,handler,logic,svc,types}（生成物）
- Modify: `go.work`（已有 authz 则不动）
- Modify: `services/authz/sample` 无关——根 `Makefile` 的 `gen`/`contract-check` 追加 authz 段

**Interfaces:**
- Consumes: Task 1 engine 包；kit/template/goctl；kit/errors、kit/logging
- Produces（Task 3-7 消费）:
  - 路由三组（.api 契约固化）：`@server(jwt: Auth, middleware: AdminRole)` 组 `/v1/admin/*`；`@server(jwt: Auth)` 组 `/v1/authz/*` 与 `/v1/me/permissions`
  - config.Config：`rest.RestConf` + `Auth{AccessSecret, AccessExpire}` + `Engine{HTTPAddr, StoreName, Datastore, MySQLDSN}` + `MysqlConf{Dsn}`（业务库）+ `AdminRoles []string`（默认 ["super_admin"]）
  - svc.ServiceContext：Engine *engine.Handle + 四个 biz repo 接口字段（Task 4 填实，先 nil + 注释）

- [x] **Step 1: 写 authz.api 契约**

三组路由 + 类型（全部中文注释；请求/响应类型按 Task 4-7 需要一次定齐）：

```
@server (
	jwt: Auth
	middleware: AdminRole
	prefix: /v1/admin
)
service authz-api {
	@handler CreateOrg        post /orgs (CreateOrgReq) returns (OrgResp)
	@handler ListOrgs         get /orgs returns (OrgListResp)
	@handler UpdateOrg        put /orgs/:id (UpdateOrgReq) returns (OrgResp)
	@handler DeleteOrg        delete /orgs/:id (OrgIdReq)
	@handler AddMember        post /orgs/:id/members (MemberReq) returns (MemberResp)
	@handler RemoveMember     delete /orgs/:id/members/:uid (MemberIdReq)
	@handler AssignRole       post /roles/assign (AssignRoleReq) returns (RoleResp)
	@handler RevokeRole       post /roles/revoke (AssignRoleReq) returns (RoleResp)
	@handler ListRoles        get /roles returns (RoleListResp)
	@handler UpsertRule       post /rules (UpsertRuleReq) returns (RuleResp)
	@handler ListRules        get /rules returns (RuleListResp)
	@handler DeleteRule       delete /rules/:id (RuleIdReq)
	@handler RegisterResourceType post /resource-types (RegisterResourceTypeReq) returns (ResourceTypeResp)
	@handler ListResourceTypes get /resource-types returns (ResourceTypeListResp)
	@handler ListAudit         get /audit (ListAuditReq) returns (AuditListResp)
}

@server (
	jwt: Auth
	prefix: /v1/authz
)
service authz-api {
	@handler Check       post /check (CheckReq) returns (CheckResp)
	@handler ListObjects  post /list-objects (ListObjectsReq) returns (ListObjectsResp)
	@handler WriteRelations post /relations (WriteRelationsReq) returns (WriteRelationsResp)
}

@server (
	jwt: Auth
	prefix: /v1/me
)
service authz-api {
	@handler MePermissions get /permissions returns (MePermissionsResp)
}
```

类型字段（要点）：OrgResp{Id,Name,ParentId}；MemberReq{UserId}；AssignRoleReq{RoleKey,UserId}；UpsertRuleReq{Id optional,RoleKey,Prefix,Action optional}；CheckReq{Object,Relation}（**不带 user**——从 JWT sub 取）；CheckResp{Allowed bool}；ListObjectsReq{Type,Relation}；WriteRelationsReq{Tuples []TupleReq{User,Relation,Object}}（幂等，X-Service-Name 头记录来源）；MePermissionsResp{Perms []string}（`服务:动作` 码）；AuditListResp 分页 {items,total}。

- [x] **Step 2: 生成骨架**

```bash
cd services/authz && goctl api go -api authz.api -dir . --home ../../kit/template/goctl
```

生成物不手改（handler/types）；etc 配置全默认值（Port 8084、Engine.HTTPAddr "127.0.0.1:18084"、Datastore "memory"、两 Dsn 空、AccessSecret 默认 dev 值——**真实 secret 生产由环境变量注入**）。

- [x] **Step 3: svc 装配与 AdminRole 中间件**

svc：ServiceContext{Config, Engine *engine.Handle,Logger}——Start 时 `engine.Start`（Datastore=memory）；AdminRole 中间件（internal/middleware/adminrole.go）：读 jwt claims roles，不含 Config.AdminRoles 任一 → `Forbidden("AUTHZ_FORBIDDEN","需要平台管理员角色")`。claims 结构：go-zero jwt 解析后 `sub/roles` 在 ctx（自定义 key 或 `github.com/zeromicro/go-zero/core/stores/ctxmon`？以生成物 types/claims 取用方式为准——标准做法：jwt claims 注入 ctx 由 go-zero rest 自动完成，key 为 "sub" 等字符串；实现 `IdentityFrom(ctx) (userID string, roles []string, err error)` 工具）。

- [x] **Step 4: healthz/readyz + 路由验证**

main AddRoute：/healthz 恒 200；/readyz 引擎 HTTP 端口 dial 成功才 200（engine.Handle 暴露 Ping()）。`make gen && make contract-check`（Makefile 追加 authz 段后）通过；curl 五路由 401（无 token）/200（带测试 token，svc 提供 `TestToken(secret, sub, roles)` 测试助手或单测内签）。

- [x] **Step 5: 打标**

验证 + commit "Task2(M1a): authz 骨架——契约/生成/管理面鉴权/健康检查"，勾选、ledger、TodoWrite。

---

### Task 3: M1b bootstrap——幂等建 Store/模型/根组织/超管

**Files:**
- Create: `services/authz/cmd/bootstrap/main.go`、`services/authz/internal/bootstrap/bootstrap.go`（可测核心逻辑）
- Modify: `services/authz/sample` 的 main（`BOOTSTRAP=1` 时服务启动内联执行同一逻辑）
- Test: `services/authz/internal/bootstrap/bootstrap_test.go`

**Interfaces:**
- Consumes: engine.Handle（Task 1）
- Produces: `bootstrap.Run(ctx, h *engine.Handle) error`——幂等执行：①EnsureModel ②写根组织 tuples（`org:root` 存在检查）③超管 `platform_role:super_admin#assignee@user:admin` ④输出 store/model id（后续任务状态承接）

- [x] **Step 1: 写失败测试**：连续跑 `bootstrap.Run` 两次，断言无 error 且关键 tuples 第二次不重复（WriteTuples 幂等路径生效）、根组织成员/超管 Check 语义正确（admin 用户 is super_admin）。
- [x] **Step 2: 确认 FAIL → 实现 → GREEN**：main.go 读配置（env `AUTHZ_API_CONFIG` 或默认 etc 路径）+ 独立 flag；main.tpl 生成的 sample 主入口加 `BOOTSTRAP=1` 内联钩子（该文件为手写层，允许改）。
- [x] **Step 3: 验证**：`go run ./cmd/bootstrap` 连跑两次日志一致幂等；`make build` 过。
- [x] **Step 4: 打标** commit "Task3(M1b): bootstrap 幂等——Store/模型/根组织/超管"。

---

### Task 4: M2a 业务库——建库 + biz repo（接口/内存/MySQL）

**Files:**
- Create: `services/authz/internal/repo/{rule,resource_type,model_version,audit}.go`（接口 + 模型）、`memory_*.go`、`mysql_*.go`、`services/authz/migrations/{000_provision.sql,001_biz.sql}`、`services/authz/cmd/provision/main.go`
- Modify: `services/authz/internal/svc/servicecontext.go`（装配四 repo）
- Test: `services/authz/internal/repo/*_test.go`

**Interfaces:**
- Produces:
  - `RuleRepo{Upsert(ctx, Rule) (Rule, error); List(ctx) ([]Rule, error); Delete(ctx, id string) error}`；`Rule{ID, RoleKey, Prefix, Action string}`（Action 空 = 服务级）
  - `ResourceTypeRepo{Create(ctx, ResourceType) (ResourceType, error); Get(ctx, name string) (ResourceType, error); List(ctx) ([]ResourceType, error)}`；`ResourceType{Name, DslBlock string, CreatedAt time.Time}`
  - `ModelVersionRepo{Latest(ctx) (ModelVersion, error); Save(ctx, ModelVersion) error}`；`ModelVersion{ID, Dsl string, CreatedAt}`
  - `AuditRepo{CreatePending(ctx, AuditEntry) (string, error); MarkCommitted(ctx, id string) error; MarkFailed(ctx, id, reason string) error; ListPending(ctx, olderThan time.Duration) ([]AuditEntry, error); Query(ctx, filter AuditFilter) ([]AuditEntry, int, error)}`；`AuditEntry{ID, Operator, Action, Target, Snapshot string, Status /*pending|committed|failed*/, CreatedAt}`
  - `provision` 工具：连接 server 级 DSN 执行 `CREATE DATABASE IF NOT EXISTS nemo_authz / nemo_authz_biz`，然后跑 migrations（复用 sample 的 runner 模式）

- [x] **Step 1: TDD**——共享断言组模式（参照 `services/sample/internal/repo/repo_test.go` 既有模式）：每个 repo 定义 `testRuleRepo(t, RuleRepo)` 等断言组，内存必跑；MySQL 走 `MYSQL_DSN` 开关 skip。先写测试确认 FAIL。
- [x] **Step 2: 实现**——mysql.go 用 `database/sql` 参数化（审计 Status 枚举、Snapshot 存 JSON 文本）；provision 工具 + 迁移 SQL（表结构含 created_at/updated_at、audit 的 status 索引）。
- [x] **Step 3: 验证**——无 DSN 全绿；**本机 MySQL 实测**（凭据见 .local-dev.md：先 `go run ./cmd/provision` 建两库 + 迁移，再 `MYSQL_DSN='root:***@tcp(127.0.0.1:3306)/nemo_authz_biz?...' go test ./internal/repo/ -v` 全绿；两轮幂等）；`make build/test/lint` 过。
- [x] **Step 4: 打标** commit "Task4(M2a): 业务库——四 repo + 建库 provision + 迁移"。

---

### Task 5: M2b 管理 API——组织/成员 + 平台角色（FGA tuple 写路径 + 审计）

**Files:**
- Modify: `services/authz/internal/logic/{createorg,addmember,assignrole,...}logic.go`（填充生成骨架）、`internal/svc/servicecontext.go`
- Test: `services/authz/internal/logic/*_test.go`

**Interfaces:**
- Consumes: engine.Handle（WriteTuples/Check）、RuleRepo/AuditRepo
- Produces（语义，Phase B 前端直接调）:
  - CreateOrg：写 `org:{id}#parent@{parent}`（根组织 parent 留空）+ 审计；**防环**：沿 parent tuple 向上遍历，见自身即 `Conflict("AUTHZ_ORG_CYCLE","组织环")`
  - AddMember/RemoveMember：`org:{id}#member@user:{uid}` grant/revoke + 审计（operator=JWT sub）
  - AssignRole/RevokeRole：`platform_role:{key}#assignee@user:{uid}`；RoleKey 白名单校验（内置 super_admin/operator/readonly，未注册 → `BadRequest("AUTHZ_ROLE_UNKNOWN","角色未定义")`）
  - 所有写路径：先 `AuditRepo.CreatePending` → 写引擎 → `MarkCommitted`；引擎错误 → `MarkFailed` + 503

- [x] **Step 1: TDD（logic 层，engine 用 fake——在 internal/engine 定义 `type Checker interface` 最小面或直接用 memory 引擎真实例，选后者：Task 1 已证 memory 启动 <1s，单测直启真引擎语义最真）**：创建组织/加成员/赋角色成功路径 + 环检测 + 未注册角色 400 + 审计 pending→committed 断言（fake AuditRepo 或查内存 repo）。
- [x] **Step 2: 实现 → GREEN**；越权用例：非 super_admin token 调 admin 路由 → 403 `AUTHZ_FORBIDDEN`（middleware 层测试）。
- [x] **Step 3: 验证**：`make test/lint/build` 过；curl 带 token 实测建组织→加成员→Check 判定联动（smoke 扩展）。
- [x] **Step 4: 打标** commit "Task5(M2b): 管理 API——组织/成员/平台角色（tuple 写路径+审计）"。

---

### Task 6: M2c 管理 API——规则表 + 资源类型注册 + 审计对账定时任务

**Files:**
- Modify: `services/authz/internal/logic/{upsertrule,registerresourcetype,...}logic.go`、`services/authz/internal/engine/modelgen.go`（注册配置→DSL 块生成）
- Test: `services/authz/internal/logic/*_test.go`、`internal/engine/modelgen_test.go`

**Interfaces:**
- Produces:
  - `modelgen.Render(r ResourceType) string`——按固定模板从配置生成 DSL 块（**同参数同输出**；白名单开关：member_view 选配、sharer 选配、can_* 出口固定集），非法配置不可能产出越权结构（模板决定一切，不做自由 DSL）
  - RegisterResourceType：校验名唯一（ResourceTypeRepo）→ Render → 与当前模型合并出新 DSL → 写引擎新版本 → ModelVersionRepo.Save → 审计；未注册服务判定 fail-closed（引擎报 type 不存在 → 503/403 由 Task 7 定）
  - UpsertRule/DeleteRule：biz 表 + 审计；Action 分组标签按 permission-design §5.2（前缀 × 动作标签）
  - **对账定时任务**：main 内 Ticker（间隔配置默认 1m）扫 `ListPending(5m)` → 回查引擎实际状态（tuple Read）→ committed/failed 修正 + 告警日志

- [x] **Step 1: TDD**——modelgen 同参数同输出（两次 Render 深比较）+ 白名单矩阵（选配开/关组合产出合法 DSL，用 engine.EnsureModel 真解析验证合法）；注册全流程（注册→新模型版本→Check 新 type 生效）；对账任务：pending 超时 + 引擎实态不一致 → 修正断言（注入受控 engine handle）。
- [x] **Step 2: 实现 → GREEN**；`make test/lint/build` 过。
- [x] **Step 3: 打标** commit "Task6(M2c): 规则表/资源类型注册/审计对账任务"。

---

### Task 7: M3a 判定面——Check / ListObjects / me/permissions / 关系写接口

**Files:**
- Modify: `services/authz/internal/logic/{check,listobjects,mepermissions,writerelations}logic.go`
- Test: `services/authz/internal/logic/*_test.go`

**Interfaces:**
- Produces:
  - Check：user=claims.sub → engine.Check → {Allowed}；明确 false = 200 {allowed:false}（**业务层 403 由 kit client 或前端转译**，服务端不把"无权"当错误）；引擎 error → `Internal("AUTHZ_ENGINE_ERROR","权限判定服务异常")`（fail-closed，HTTP 503 经 kiterrors）
  - ListObjects：同上返回 id 数组
  - MePermissions：claims.roles × RuleRepo.List() 归并 → `服务:动作` 码集合（规则表派生，permission-design §5.2；服务级规则产出 `<服务>:read` 缺省码，动作级产出 `<服务>:<动作>`）
  - WriteRelations：幂等写（WriteTuples 已幂等）+ 审计（operator=sub，service=X-Service-Name 头缺省 "unknown"）
- [x] **Step 1: TDD**——判定 200{false} 语义、引擎故障 503（关掉 engine 端口模拟）、me/permissions 归并矩阵（多角色×多规则去重）、WriteRelations 幂等（重复写不报错）+ 审计记录。
- [x] **Step 2: 实现 → GREEN**；`make test/lint/build` 过。
- [x] **Step 3: 打标** commit "Task7(M3a): 判定面——Check/ListObjects/me-permissions/关系写"。

---

### Task 8: M3b kit/authz + 引擎 MySQL datastore + 模型测试全矩阵 + 集成验收

**Files:**
- Create: `kit/authz/authz.go`、`kit/authz/remote.go`、`kit/authz/middleware.go`
- Modify: `services/authz/internal/engine/engine.go`（MySQL datastore 分支填实）、`services/sample/internal/…`（挂 Require 冒烟一行）、根 `Makefile`（kit/authz 并入 test/lint）
- Test: `kit/authz/authz_test.go`、`services/authz/internal/engine/engine_mysql_test.go`

**Interfaces:**
- Produces:
  - `authz.Checker interface { Check(ctx context.Context, object, relation string) (bool, error); ListObjects(ctx context.Context, objType, relation string) ([]string, error) }`（ctx 内含身份——Remote 从 ctx 取 JWT 透传）
  - `authz.Remote New(Config{BaseURL string, Timeout time.Duration})`；`authz.Require(c Checker, perm string)` 中间件（http.HandlerFunc，kit/authz 不依赖 go-zero）；`authz.CheckObject(ctx, c, "order", id, "can_view")`
  - 语义：无权 403 `AUTHZ_DENIED`；网络/5xx/超时 503 `AUTHZ_SERVICE_UNAVAILABLE`（fail-closed）；错误解析用 kit/errors.Decode
  - engine MySQL datastore：配置 Datastore=mysql + MySQLDSN（nemo_authz 库）可启动且 Check 语义与 memory 一致（同一断言组跑两种 datastore——**注意 openfga mysql datastore 需要迁移：核对 `go doc` 其 Migrate/`openfga migrate` 用法并在 EnsureModel 前执行**）

- [x] **Step 1: TDD**——kit/authz 单测（fake Checker 注入 Require；Remote 对 httptest 对端：403/503/200 三语义 + 超时；**熔断镜像测试参照 sample clientx 的 nil 分支教训**）；引擎双 datastore 同断言组（memory 全绿 + MYSQL_DSN 真 nemo_authz 库全绿，无 DSN skip）。
- [x] **Step 2: 实现 → GREEN**；sample 挂 Require("order:can_edit") 冒烟（sample 接 authz 判定——一行中间件 + 一个受保护路由，冲突时以 sample 现有契约为准最小侵入）；四场景模型测试矩阵（Task 1 断言组扩全：租户隔离/层级继承/共享与越权转授/平台角色×规则组合）。
- [x] **Step 3: 集成验收**——bootstrap → 管理 API 建组织/角色/规则 → 签发含 roles 的测试 JWT → Check 判定 → me/permissions → sample Require 联动 → 403/503 全语义；`make test/lint/build/contract-check` 全过。
- [x] **Step 4: 打标** commit "Task8(M3b): kit/authz + 引擎 MySQL 路径 + 模型测试矩阵 + 集成验收"。

---

---

## Phase B 遗留清单(全分支终审 2026-09-07 分诊固化)

**前置项(Phase B 第一批)**:

1. 服务身份凭证(M4 API key)落地后,回迁/强化 WriteRelations 出 AdminRole 通道(Phase A 以写白名单 AUTHZ_RELATION_FORBIDDEN 护栏过渡)
2. 引擎元组 O(n) 扫描的性能替代方案;Orgs 组织注册表 MySQL 落库(当前内存实现,重启丢组织名,README 已注明)
3. DeleteOrg"有子组织拒绝删除"预检
4. kit 新增 ServiceUnavailable 构造器收口 + authz 503 码统一(AUTHZ_ENGINE_ERROR → AUTHZ_SERVICE_UNAVAILABLE;kit 双 owner 治理)

**已接受取舍(记录备查)**:

- authn 无密钥轮换(prevSecret 空);ModelFile cwd 相对路径三入口同约定
- ListOrgs/DeleteOrg/orgExists O(全仓元组);memory ListPending ID 字典序;对账 Ticker 无重入保护;注册重名 TOCTOU 503;DeleteRule 全量预检
- MySQL 多副本首启迁移竞态窗口(README 运维注意已写);migrate 传递引入 goose+modernc sqlite;未勾选 can_share 的类型写 sharer 引擎层 relation-not-found 503(语义错位,fail-closed)
- UpdateOrg 组织降根不可表达(需产品输入);测试断言重复、gofmt 对齐等品味项

## Self-Review 记录

- **Spec 覆盖**：spec §4 M0→Task 1；M1→Task 2/3；M2→Task 4/5/6；M3→Task 7/8（spec M3 的"模型测试四场景进 CI"由 Task 1+8 的 engine 测试矩阵承担，`make test` 即 CI 入口）。M4/M5/M6 不在本计划（Phase B 另立计划，M6 联调纳入 Phase B）。
- **类型一致性**：engine.Handle API 在 Task 1 定义、2/3/5/6/7 消费一致；biz 四 repo 在 Task 4 定义、5/6/7 消费一致；Checker 接口 Task 8 定义即终态（M5 前端不消费）。claims 语义（sub/roles）Task 2 定、5/7 消费一致。
- **占位符**：无 TBD；go-openfga/SDK API 表面各 Task 均带 go doc 核对步骤与"以实 API 为准"决策规则（Phase 0 验证过的模式）；MySQL datastore 迁移用法为 Task 8 的显式核对项。
