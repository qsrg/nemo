# nemo/kit

公共约定模块：把"20 个服务必须一致"的行为（错误体、日志字段、服务骨架）收敛为编译期依赖的代码。**编译器依赖，不是运行时组件**——不部署、无常驻进程，被编译进每个业务服务（architecture.md §5）。

## 包清单（现状）

| 包 | 职责 | 状态 |
|----|------|------|
| `errors` | 跨服务统一 JSON 错误体、错误码约定 | 已交付 |
| `logging` | 日志初始化（字段约定、JSON 文件输出与轮转默认值）+ 两个 HTTP 中间件 | 已交付 |
| `template` | goctl 定制模板，生成服务骨架自带 kit 接线 | 已交付 |
| `authz` | 权限判定接入件（Checker/Remote/Require/RequireWith + 权限码判定 CodeGranted）+ 统一身份中间件（Identity/Session/IdentityFrom），见 permission-design.md §5.3 与下文「统一鉴权」 | 已交付 |
| `audit` | 审计先行写路径模板（WriteWithAudit：pending → write → committed/failed 三态 + fail-closed），服务 repo 经 Sink 适配接入 | 已交付 |

> 历史：v2.1 的 `auth`/`config`/`httpx`/`middleware`/`reqid` 五个冻结包已于 2026-09-06 随 user/order demo 退役删除（git 基线 39eabbc 可考），kit 现为上表五包。

## kit/errors

统一错误体 `{"error":{"code","message","request_id"}}`。业务侧用语义构造器返回错误，handler 的 `httpx.ErrorCtx` 经全局出口统一转换。

- 构造器（均为 `(code, message string)`）：`BadRequest` / `Unauthorized` / `Forbidden` / `NotFound` / `Conflict` / `Internal`；任意状态码用 `New(status, code, message)`
- `From(err)`：error 转断言为 `*Error`（跨服务响应体解码场景配合 `Decode` 使用）
- `InstallErrorHandler()`：注册 go-zero 全局错误出口（`httpx.SetErrorHandlerCtx`），**main 调用一次**；非 kit 错误兜底为 INTERNAL
- `WithRequestID(ctx, id)` / `RequestIDFrom(ctx)` / `NewRequestID()`：request id 的 context 存取与生成

```go
return "", kiterrors.NotFound("KIT_TEST_NOT_FOUND", "资源不存在")
```

## kit/logging

logx 能力框架已给，kit 只统一初始化与字段。

- `Init(serviceName string, prod bool)`：main 最先调用；prod=true 时 JSON 文件输出与轮转
- `RequestID(next http.HandlerFunc)`：读取/生成 `X-Request-ID`，写 context 并回写响应头；**必须挂中间件链最外层**
- `AccessLog(next http.HandlerFunc)`：统一字段访问日志（method/path/status/duration_ms/request_id）；挂 `RequestID` 之后

## kit/authz

权限判定 + 统一身份接入件（标准库 `http.HandlerFunc` 形态，**不依赖 go-zero**；熔断等运行时治理不在此做——kit 纪律 3）。

- `Checker` 接口：`Check(ctx, object, relation) (bool, error)` / `ListObjects(ctx, objType, relation) ([]string, error)`；ctx 内含身份——实现方从 ctx 取 JWT 透传
- `Remote`（`New(Config{BaseURL, Timeout})`）：远端实现，调集中式 authz 服务的 `/v1/authz/check`、`/v1/authz/list-objects`；`WithToken(ctx, jwt)` 注入身份，`X-Request-ID` 随行
- `Require(c Checker, perm string)`：路由保护中间件；perm 形如 `"<对象类型>:<关系>"`，对象实例 ID 取自路径末段（只应挂在带 `:id` 的路由上）
- `RequireWith(c Checker, opts Options)`：声明式进阶守卫——`Options{Perm, ObjectID, FallbackCodes}`：自定义对象 ID 提取器（多级路径/格式校验，非法 400 `AUTHZ_OBJECT_INVALID`）与权限码回退链（主判定明确无权时逐码 `CodeGranted`，任一持有即放行——页面级管理码"管理员全权"语义）
- `RequireCode(c Checker, codes ...string)`：纯权限码守卫（资源创建等**无实例 ID 的集合路由**入口）：逐码 `CodeGranted`，任一持有即放行；错误语义与 RequireWith 一致
- `CodeGranted(ctx, c, code)` / `EffectiveCodes(ctx, c)` / `PermGranted(code, perms)`：平台权限码判定（permission 对象），三级匹配语义的唯一权威实现（服务侧不得再复制）——精确码 / 段内通配（`*:动作`、`服务:*`、`*`）/ 纯服务码；**跨服务通配不授予**（持有 `redis:*` 不通过 `kafka-topics:manage`）
- `CheckObject(ctx, c, objType, id, relation)`：类型 + ID 拼装对象标识后判定
- `Identity(secret)`：统一身份中间件（平台唯一一份，authz/kafka/sample 共用）；解析 Bearer token 把 claims 注入 ctx（详见下文「统一鉴权」）
- `Session` / `IdentityFrom(ctx)` / `WithIdentity(ctx, userID, roles)` / `TestToken(secret, sub, roles)`：会话类型、取用与测试辅助
- 四语义（逐字）：无权 403 `AUTHZ_DENIED`；网络/5xx/超时 503 `AUTHZ_SERVICE_UNAVAILABLE`（fail-closed）；上游 401 原样透传（身份错误可换 token 自救）；守卫提取器判非法对象 ID 400 `AUTHZ_OBJECT_INVALID`。上游错误体经 `kit/errors.Decode` 解析

```go
checker := authzkit.New(authzkit.Config{BaseURL: c.Authz.BaseURL, Timeout: c.Authz.Timeout})
server.AddRoute(rest.Route{
	Method:  http.MethodGet,
	Path:    "/v1/managed-items/:id",
	Handler: authzkit.Require(checker, "order:manage")(handler.GetItemHandler(ctx)),
})
```

RequireWith 声明式守卫（多级路径对象 ID + 权限码回退，kafka 的行级接口即此形态，见 services/kafka/internal/handler/guard.go）：

```go
Handler: authzkit.RequireWith(checker, authzkit.Options{
	Perm:          "report:can_edit",
	ObjectID:      reportIDFromPath,          // 提取器：从 :id 路径参数取并校验，非法 400
	FallbackCodes: []string{"reports:manage"}, // 主判定无权时的管理码回退
})(handler.UpdateReportHandler(ctx))
```

## kit/audit

审计先行写路径模板——authz 与 kafka（及后续业务服务）共用的唯一权威实现，服务侧 repo 经 `Sink` 三方法适配接入，不得再各拷贝一份。

- `Sink` 接口：`CreatePending(ctx, Entry) (int64, error)` / `MarkCommitted(ctx, id)` / `MarkFailed(ctx, id, reason)`
- `WriteWithAudit(ctx, sink, operator, action, target, snapshot, write)`：pending 先落 → 执行 write → 成功 committed / 失败 failed；错误分类：写闭包失败 503 `AUTHZ_SERVICE_UNAVAILABLE`（fail-closed，已记录待对账），引擎校验拒绝（Invalid tuple）400 `AUTHZ_ENGINE_REJECTED`，审计落库失败 500 `AUTHZ_INTERNAL`（不执行业务写）
- 调用契约：参数校验类错误（400/404/409）在进入前完成——之后的失败只可能是授权服务/存储故障；跨服务无本地事务，闭包内"先业务写后元组写"等补偿语义由调用方注释钉死（对账兜底）

## 统一鉴权（新服务接入）

鉴权分两层，均为平台统一实现，**业务服务不写鉴权逻辑**：

1. **身份注入（你是谁）**：`server.Use(authzkit.Identity(c.Auth.AccessSecret))` 挂一次——解析 Bearer token，把 `claims.sub/name/tenant/roles` 注入 ctx（`authzkit.IdentityFrom(ctx)` 取用），并把原始 token 写入 authz ctx 键供 Remote 透传。全平台共用这一份（kit/authz/authn.go），不再允许各服务拷贝 `internal/authn`。
2. **JWT 校验 + 权限判定（你能不能）**：需要鉴权的路由组挂 `rest.WithJwt(c.Auth.AccessSecret)`（401 fail-closed）；资源级判定再叠加 `authzkit.Require(checker, "<对象类型>:<关系>")`（403/503 fail-closed）。公开路由（healthz/readyz/登录通道）不挂 WithJwt 即可匿名访问。

约定：

- `Auth.AccessSecret` 须与 authz 服务一致（平台 JWT 共享签名密钥，见各服务 etc/*.yaml 的 `Auth` 块）
- 认证与判定是两类事实：401 未认证 ≠ 403 无权；`typ=temp` 的临时 token（CAS 回调链路专用）被 Identity 直接 401 拒绝，不访问任何业务接口
- 新服务用 goctl 模板生成时**默认自带** `server.Use(authzkit.Identity(...))`（.api 需声明 jwt 以生成 config 的 Auth 块）；logic 单测用 `authzkit.WithIdentity` 构造带身份的 ctx，`authzkit.TestToken` 签发测试 token

```go
// main：身份注入一行
server.Use(authzkit.Identity(c.Auth.AccessSecret))
// routes：受保护路由组
server.AddRoutes(routes, rest.WithJwt(serverCtx.Config.Auth.AccessSecret), rest.WithPrefix("/v1/xxx"))
// logic：取操作人
sess, err := authzkit.IdentityFrom(l.ctx)
```

## kit/template

goctl 定制模板（`goctl template init` 产物 + 团队约定修改），保证所有服务骨架一致。

生成服务（`.api` 在服务根目录；以 sample 为例）：

```bash
# 仓库根执行
goctl api go -api services/<name>/<name>.api -dir services/<name> --home kit/template/goctl
# 或在 services/<name>/ 内执行（checklist 标准姿势，见 backend-design §3）
goctl api go -api <name>.api -dir . --home ../../kit/template/goctl
```

生成物自带（无需手工接线）：

1. **main 四要素**：`kitlog.Init(服务名, false)`、`kiterrors.InstallErrorHandler()`、`server.Use(kitlog.RequestID)` + `server.Use(kitlog.AccessLog)`（RequestID 在外层，顺序不可颠倒）、`server.Use(authzkit.Identity(c.Auth.AccessSecret))`（统一身份注入，见上文「统一鉴权」）
2. **注释规范**：handler 包注释"只做协议转换，业务规则在 logic（backend-design §2）"；logic 包注释"业务规则层、单测重点，错误用 kit/errors 语义构造器返回"；ServiceContext 注释"显式装配依赖，无 DI 容器，repo/client 接口在此注入"

模板能力边界（生成后注意，无需改模板）：

- `Init` 的服务名取 `.api` 的 service 块名；与部署名不符时生成后手工修正
- `Init` 的 prod 参数固定 `false`，生产部署时改 `true`（或改为从 config 读取）
- 统一身份中间件依赖 `.api` 声明 jwt：未声明时 config 无 `Auth` 块，生成物编译失败——新服务必须声明 jwt（与 authz/kafka/sample 一致），这是强制统一鉴权的有意行为
- goctl 不会写入 `nemo/kit` 依赖：仓库内服务的 go.mod **不写 kit 的 require/replace**（由 workspace 直接解析，monorepo 约束，kit 无版本发布；见 `services/sample/go.mod` 注释）；workspace 外临时实验需自行 `require nemo/kit v0.0.0` + `replace nemo/kit => <本仓绝对路径>/kit`
- goctl 1.10.2 exists-skip：已存在的生成文件不会被覆盖，改契约后先删旧生成文件再重新生成

## 纪律与治理

三个纪律（PR 评审硬性检查，architecture.md §5）：

1. 不含业务逻辑
2. 不出现业务名词（`user`/`order` 等出现在 kit 的 PR 中直接拒绝）
3. 不做运行时治理；不是强制依赖（服务偏离约定需在评审中说明理由）

变更评审：kit 为团队共有代码，**双人审批（架构 §5，双 owner + RFC）**，业务团队不改。
