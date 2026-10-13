# 权限模块 Phase B(M4 auth 服务 + M5 管理界面 + M6 联调验收)实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 auth 服务(CAS 登录→角色→签发 JWT)、authz-admin 管理界面微应用(六页+基座接入+权限过滤)、全链路联调验收——权限管理系统闭环。

**Architecture:** `services/auth`(:8083,go-zero)做认证协议适配层:CAS 3.0 校验 ticket → 映射/自动建用户 → **调 authz `/v1/authz/list-objects`(platform_role × assignee)取角色,取不到 fail-closed 拒发 token** → 签发 HS256 JWT(claims `{sub,name,tenant,roles}`,TTL 1h,与 authz 共享 secret)。前端 `apps/authz-admin`(:5276)加入 monorepo,六页经 `@nemo/http` 调 authz;基座新增权限数据链(/me/permissions → pinia → props 下发 → 菜单过滤)。

**Tech Stack:** Go 1.26 + go-zero v1.10.3 + goctl 1.10.2 + kit(template/errors/logging);前端 Vue 3 + TS + Vite + Element Plus + pinia + wujie + @nemo/http + @nemo/shared;本地 CAS(docker,https://localhost:8443,CAS 3.0 `serviceValidate`)。

**Spec:** `docs/superpowers/specs/2026-09-06-authz-implementation-design.md`(§3.2 auth、§3.4 前端、§4 M4-M6、§5 测试策略);授权语义以 `docs/permission-design.md` 为准。Phase A 产物(`services/authz` :8084、`kit/authz`)是直接依赖。

## Global Constraints

- **提交卫生:用户并行改动绝不卷入 commit**——工作区常驻未提交文件:console-frontend/apps/{console,user,order}/**、docs/frontend-design.md、AGENTS.md、.idea/**。每任务 `git add` 只加自己创建/修改的文件,禁止 `git add -A`/`git add console-frontend`(整目录会卷入用户改动)
- JWT:HS256,**全栈同一 secret**(dev 默认 `dev-authz-access-secret`,与 authz etc 一致);claims `{sub, name, tenant, roles}`;TTL 1h;**tenant 一期固定 `"platform"`**(多租户切换在排除项);roles 为平台角色名数组(如 `["super_admin"]`)
- auth 错误码前缀 `AUTH_`(如 `AUTH_CAS_TICKET_INVALID`、`AUTH_ROLE_FETCH_FAILED`);语义:ticket 无效 401、上游 authz 故障 503 fail-closed 拒发 token
- auth 依赖 authz 地址走环境变量 `AUTHZ_SERVICE_URL`(dev 默认 `http://127.0.0.1:8084`);服务内 jwt 校验用 go-zero rest jwt 配置;调用 authz 透传用户 JWT(经 kit/httpx 或标准库 client,带超时 5s)
- CAS:地址 `https://localhost:8443`(见 .local-dev.md,不进 git);CAS 3.0 协议,**`serviceValidate` 具体路径 M4 联调时探测**(候选 `/cas/p3/serviceValidate`、`/serviceValidate`),探测结论落报告;标准库 HTTP 实现,无重依赖
- auth 用户 repo 一期**内存实现**(spec §3.2;CAS 用户首次登录自动建,重启丢失可接受);API key 只存 SHA-256,原始 key 一次性返回
- 端口:auth 8083(登记 backend-design §7);authz-admin 5276;`go.work`、根 Makefile `gen`/`contract-check` 登记 auth
- AGENTS.md:所有函数中文注释;logx 禁 fmt.Println;repo 参数化 SQL(本计划无 SQL)
- 前端:Element Plus、`@nemo/http`(禁裸 fetch)、菜单注册表加一条(key `authz`,域"安全合规");**authz-admin 接入方式以 `packages/shared` 现行代码为准**(基座容器正在被用户重构:WujieHost.vue→MicroAppFrame.vue 未提交,冲突时读现行 shared/micro.ts 适配,不改用户未提交的实现方向)
- authz Phase A 接口事实(消费契约,勿改 authz):`POST /v1/authz/list-objects {type,relation}` → `{objects []string}`;`GET /v1/me/permissions` → `{perms []string}`(码形 `服务:动作`);管理 API 在 `/v1/admin/*`(JWT+AdminRole);`POST /v1/authz/relations` 有写白名单(仅注册资源类型 owner/editor/viewer/sharer,越权 403 `AUTHZ_RELATION_FORBIDDEN`)
- OpenFGA ListObjects 返回的 object id 是否带 `platform_role:` 前缀以实测为准,auth 侧取角色统一 `strings.TrimPrefix(id, "platform_role:")` 兼容两种形态

---

### Task 1: M4a auth 服务骨架(.api 契约 + goctl 生成 + 健康检查)

**Files:**
- Create: `services/auth/auth.api`、`services/auth/etc/auth-api.yaml`、`services/auth/internal/{config,handler,logic,svc,types,authn}`(生成物+手写层)、`services/auth/README.md`
- Modify: `go.work`、根 `Makefile`(gen/contract-check 追加 auth 段,照 authz 写法)
- Modify: `docs/backend-design.md`(端口表登记 auth 8083、环境变量表登记 `AUTHZ_SERVICE_URL`)——只加行,不碰用户可能改的其他段落

**Interfaces:**
- Consumes: Phase 0 kit 接线三件套(kitlog.Init、InstallErrorHandler、server.Use 顺序)、kit/template/goctl
- Produces(Task 2-4 消费):
  - 路由两组:`@server(prefix: /v1)` 公开组:`GET /auth/cas/login`、`POST /auth/cas/callback (CasCallbackReq) returns (TokenResp)`;`@server(jwt: Auth, prefix: /v1)` 组:`GET /me returns (MeResp)`、`POST /auth/refresh returns (TokenResp)`、`POST /api-keys (CreateApiKeyReq) returns (ApiKeyResp)`
  - 类型:`CasCallbackReq{Ticket, Service}`;`TokenResp{Token, ExpiresAt int64}`;`MeResp{Sub, Name, Tenant, Roles []string}`;`CreateApiKeyReq{Name, Prefixes []string}`;`ApiKeyResp{Key, Id}`(Key 一次性)
  - config.Config:`rest.RestConf + Auth{AccessSecret, AccessExpire} + AuthzConf{BaseURL}` + `CasConf{BaseURL, ValidatePath}`
  - etc 默认:Port 8083、AccessSecret `dev-authz-access-secret`、AuthzConf.BaseURL `http://127.0.0.1:8084`、CasConf.BaseURL `https://localhost:8443`、ValidatePath `/cas/p3/serviceValidate`(联调时探测修正)

- [x] **Step 1: 写 auth.api 契约**(全部中文注释;字段全标 `optional` 并注释"logic 层自校验必填——Task 5-7 教训"照 authz.api 惯例)
- [x] **Step 2: goctl 生成**:`cd services/auth && go mod init nemo/services/auth && go mod edit -go 1.26 && go mod edit -require nemo/kit@v0.0.0 -replace nemo/kit=../../kit && goctl api go -api auth.api -dir . --home ../../kit/template/goctl && go mod tidy`
- [x] **Step 3: svc 装配**:ServiceContext{Config, Users UserRepo(Task 2 填实,先 nil+注释), CasClient(Task 2), AuthzClient(Task 2)};main 五步接线照 authz main(kitlog.Init→config→ServiceContext→handler→Use)
- [x] **Step 4: healthz/readyz**:/healthz 恒 200;/readyz 拨测 authz `/healthz`(HTTP 200 即就绪;CAS 不做依赖探针)
- [x] **Step 5: 验证**:`make gen && make contract-check` 过;`go build ./...` 过;curl /healthz 200、/v1/me 无 token 401
- [x] **Step 6: Commit**(仅本任务文件)"Task9(B1): auth 骨架——契约/生成/健康检查/Makefile 登记"

---

### Task 2: M4b CAS 登录链路(CAS client + 用户映射 + 角色获取 + 签发)

**Files:**
- Create: `services/auth/internal/cas/cas.go`(CAS 3.0 client)、`services/auth/internal/authzclient/authz.go`(取角色)、`services/auth/internal/repo/users.go`(接口+内存实现)、`services/auth/internal/logic/caslogin.go`+`cascallback.go`(login 返回 CAS 跳转 URL 的拼装与 callback 编排)
- Test: `services/auth/internal/cas/cas_test.go`、`internal/repo/users_test.go`、`internal/logic/cascallback_test.go`

**Interfaces:**
- Consumes: Task 1 骨架;authz `POST /v1/authz/list-objects`(透传 JWT);kit/errors
- Produces(Task 3/5 消费):
  - `cas.Client.ValidateTicket(ctx, ticket, service string) (CasUser, error)`;`CasUser{Username string, Name string, Email string}`(attributes 缺失时 Name/Email 回落 Username);实现:GET `{BaseURL}{ValidatePath}?ticket=&service=`,解析 CAS 3.0 XML(`cas:authenticationSuccess/cas:user`),非 success → `errors.New(401, "AUTH_CAS_TICKET_INVALID", "CAS ticket 校验失败")`;TLS 自签证书跳过校验(dev CAS https 自签,`InsecureSkipVerify` 配置开关,注释注明仅 dev)
  - `authzclient.Client.ListPlatformRoles(ctx, userJWT string) ([]string, error)`:POST `{BaseURL}/v1/authz/list-objects` body `{"type":"platform_role","relation":"assignee"}`,透传 `Authorization: Bearer <userJWT>`;非 200 → `errors.New(503, "AUTH_ROLE_FETCH_FAILED", "角色获取失败")`(fail-closed);返回值逐个 `strings.TrimPrefix(id, "platform_role:")`
  - `repo.UserRepo{GetByIdp(ctx, idpType, idpSubject) (*User, error) /*ErrNotFound*/; Create(ctx, User) (*User, error)}`;`User{ID, Name, Email, IdpType, IdpSubject}`(ID = `user:` + cas username)
  - `logic.IssueToken(ctx, u *repo.User, roles []string) (TokenResp, error)`:签 HS256 claims `{sub: strings.TrimPrefix(u.ID, "user:"), name, tenant: "platform", roles, exp: now+AccessExpire}`(sub 裸用户名,与临时 token 同口径)(标准库或 golang-jwt/v4 与 go-zero rest jwt 兼容——HS256 map claims;secret 取 Config.Auth.AccessSecret)

- [x] **Step 1: 写失败测试**——cas_test:httptest 假 CAS(success XML / failure XML / 非 200 → 503 语义区分:非 200 是 503 AUTH_CAS_UNAVAILABLE,success=false 是 401 AUTH_CAS_TICKET_INVALID);users_test:GetByIdp 未命中→ErrNotFound→Create→再取命中;cascallback_test:logic 编排(假 cas.Client + 假 authzclient + 真 repo)——首次登录自动建用户、roles 映射进 claims、**authzclient 返回错误 → logic 返回 503 且不发 token(fail-closed)**、ticket 无效 → 401
- [x] **Step 2: 确认 FAIL → 实现 → GREEN**(实现要点全中文注释;XML 解析用 encoding/xml 定义 `casServiceResponse` 最小结构)
- [x] **Step 3: login logic**:拼 `{CasConf.BaseURL}/login?service={service}` 返回给 handler(302 由 handler 层 `http.Redirect` 做——goctl 生成 handler 允许手写层重写该一个 handler,其余不动;契约上 login 无返回体)
- [x] **Step 4: 验证**:`go test ./internal/... -v` 全绿;`make build/lint` 过
- [x] **Step 5: Commit**"Task10(B2): CAS 登录链路——ticket 校验/用户映射/角色 fail-closed/签发"

---

### Task 3: M4c 会话接口(/v1/me + /v1/auth/refresh + 401 语义)

**Files:**
- Modify: `services/auth/internal/logic/melogic.go`、`refreshlogic.go`(填实生成桩);`internal/authn/`(若无生成则新建:go-zero rest jwt 已校验签名,logic 从 ctx 取 claims——claims 取用方式以 `go doc github.com/zeromicro/go-zero/rest` 与 authz 的 authn 经验为准:go-zero 丢弃标准 claims,需自行解析或用非标准 claim 键;**复用 authz 的 authn.Identity 方案**,把 sub/name/tenant/roles 注入 ctx)
- Test: `services/auth/internal/logic/session_test.go`

**Interfaces:**
- Consumes: Task 2 IssueToken;Config.Auth(rest jwt 自动校验)
- Produces: `/v1/me` → `{sub,name,tenant,roles}`(从 ctx claims 直取);`/v1/auth/refresh` → 旧 token 未过期即换发:claims.roles **实时重取**(调 Task 2 authzclient,失败 503 拒续签)→ 新 TokenResp;无 token/过期 → 401(go-zero jwt 层自然行为)

- [x] **Step 1: 写失败测试**:签真 token → me 返回 claims;refresh → roles 重取被调用、新 token 可过 /v1/me;authzclient 故障 → 503;无 token → 401
- [x] **Step 2: 实现 → GREEN**(authn.Identity 照 authz 的实现复制改造:同一 secret 二次解析注入;中文注释注明来源)
- [x] **Step 3: 验证**:`go test ./internal/... -v` 全绿;curl 带真 token /v1/me 200、无 token 401、假 secret token 401;make build/lint
- [x] **Step 4: Commit**"Task11(B3): 会话——me/refresh/401 语义(roles 续签实时重取)"

---

### Task 4: M4d API key 签发最小集

**Files:**
- Create: `services/auth/internal/repo/apikeys.go`(接口+内存:只存 SHA-256)、`services/auth/internal/logic/createapikeylogic.go`(填实)
- Test: `services/auth/internal/repo/apikeys_test.go`、`internal/logic/createapikey_test.go`

**Interfaces:**
- Consumes: Task 1 路由(POST /v1/api-keys 在 jwt 组);需 super_admin 校验——新建 `internal/authn/adminrole.go`(照 authz AdminRole 简版:claims.roles 含 `super_admin`,否则 403 `AUTH_FORBIDDEN`);`.api` 该路由组加 `middleware: AdminRole` 后重新 goctl 生成(生成物不手改)
- Produces: `CreateApiKeyReq{Name, Prefixes []string}`(Name 必填;Prefixes 即 scope 路由前缀白名单,permission-design §5.4,一期只存不消费);`ApiKeyResp{Key, Id}`——Key 形如 `nemo_<32hex>` 一次性返回;repo 存 `SHA-256(key)`;**不做管理界面、不做列表/撤销**(spec §1 排除项)

- [x] **Step 1: 写失败测试**:repo 存的是哈希(原始 key 不可反查)、Create 返回的 Key 二次校验哈希一致;logic:非 super_admin → 403;Name 空 → 400;成功 → Key 一次性
- [x] **Step 2: 实现 → GREEN**
- [x] **Step 3: 验证**:`go test ./internal/... -v` 全绿;curl super_admin token 签 key 200、普通 token 403;make build/lint
- [x] **Step 4: Commit**"Task12(B4): API key 签发最小集(SHA-256 落存/一次性展示/super_admin 鉴权)"

---

### Task 5: M5a 基座权限数据链(me/permissions → pinia → props → hasPermission)

**Files:**
- Create: `console-frontend/packages/shared/src/permission.ts`(`hasPermission(code: string, perms: string[]): boolean`——匹配规则:①perms 含精确 code → true;②perms 含纯 `<服务>`(code 冒号前段)→ true(服务级全量授权,当前派生口径不产生该形态,留作演进);**禁止** `<服务>:read` 兜底授予其他动作——菜单检查的 code 本身就是服务级码(如 `authz:read`),精确匹配即可;`order:read` 绝不能授予 `order:manage`。全中文注释,permission-design §5.2 口径)、`console-frontend/apps/console/src/stores/permission.ts`(pinia:perms 集合 + fetchPermissions(http GET /api/v1/me/permissions)+ refresh-permission 事件消费)
- Modify: `console-frontend/apps/console/src/stores/auth.ts`(登录/恢复成功后触发 fetchPermissions——**该文件用户可能已改,改动最小化,只在登录成功点后追加一行**)、`console-frontend/apps/console/src/views/Layout.vue`(向微应用 props 增加 `permissions` 数组——同样最小化)
- Modify: `console-frontend/packages/shared/src/index.ts`(export permission.ts)

**Interfaces:**
- Consumes: authz `GET /v1/me/permissions` → `{perms: string[]}`;基座现行 token 机制(auth store)
- Produces(Task 6-10 消费):`hasPermission(code, perms)`;`usePermissionStore()`(state: perms, loaded;action: fetch, reset);props 契约新增字段 `permissions: string[]`

- [x] **Step 1: 实现 permission.ts + 单测**(若 monorepo 无测试设施则用 vitest 最小配置加在 packages/shared——`pnpm --filter @nemo/shared add -D vitest && vitest run`;测试矩阵:精确码、服务级兜底、空 perms、`order:manage` vs perms 含 `order:read`)
- [x] **Step 2: 基座接线**:auth store 登录成功回调后 `permissionStore.fetch()`;Layout 微应用 props 加 permissions;wujie bus 监听 `refresh-permission` → fetch()(frontend-design §6 预留事件)
- [x] **Step 3: 验证**:`pnpm --filter @nemo/shared test` 过;`pnpm --filter console build` 过;dev 手测:登录后 pinia devtools 可见 perms(或临时 console.log 落报告)
- [x] **Step 4: Commit**(**git add 只加本任务 5 个文件**)"Task13(B5): 基座权限数据链——hasPermission/permissions store/props 下发/refresh-permission"

---

### Task 6: M5b 菜单权限过滤 + 代理接线

**Files:**
- Modify: `console-frontend/apps/console/src/menus.ts`(ServiceDef 增可选 `permission?: string` 字段——服务级码,如 authz 条目 `authz:read`;注释注明"叠加后端权限过滤,permission-design §5.2")
- Modify: `console-frontend/apps/console/src/views/Layout.vue` + `Workbench.vue`(首页目录卡片与左侧我的菜单渲染前过 `hasPermission(service.permission ?? service.key + ':read', perms)`——不可见即隐藏;**这两个文件用户在改,改动最小化:只插过滤调用一行级**)
- Modify: `console-frontend/apps/console/vite.config.ts`(代理:`'/api/v1/authz'`、`'/api/v1/admin'`、`'/api/v1/me/permissions'` → `http://localhost:8084`(permissions 是 authz 端点)、`'/api/v1/auth'`、`'/api/v1/api-keys'`、`'/api/v1/me'` → `http://localhost:8083`(auth 接管会话恢复,注释写明切换原因与回退);**具体路径规则必须声明在更宽前缀之前**,否则被截走)

**Interfaces:**
- Consumes: Task 5 permission store + hasPermission
- Produces: 过滤后的菜单渲染;Task 7+ 的 authz-admin 请求通路

- [x] **Step 1: 实现(改动逐文件 ≤10 行,尊重用户未提交的重构方向)**
- [x] **Step 2: 验证**:`pnpm --filter console build` 过;dev 手测:perms 含 `authz:read` 时菜单可见、清空后隐藏(临时改 store 或用 authz 实数据),截图/快照落报告
- [x] **Step 3: Commit**(git add 逐文件列出)**"Task14(B6): 菜单权限过滤 + dev 代理接线(auth/me 切 8083、authz/admin 切 8084)"**

---

### Task 7: M5c authz-admin 应用骨架

**Files:**
- Create: `console-frontend/apps/authz-admin/`(package.json、vite.config.ts、index.html、src/{main.ts,App.vue,router.ts,api/client.ts,views/Layout.vue})——结构照 apps/user;`api/client.ts` 封装 @nemo/http(token 从基座 props 注入,401 上报 `unauthorized` 事件,503 提示"服务暂不可用",错误码 `AUTHZ_RELATION_FORBIDDEN`/`AUTHZ_DENIED` 提示"无权限")
- Modify: `console-frontend/pnpm-workspace.yaml`(若无通配则补 `apps/*`)、根 `package.json` dev 脚本(若存在并行启动脚本则追加)、`console-frontend/apps/console/src/menus.ts`(注册:`key: 'authz'`、`title: '权限管理'`、`domain: '安全合规'`、`permission: 'authz:read'`、url `http://localhost:5276`、menus 六项:组织树 `/authz/orgs`、平台角色 `/authz/roles`、规则表 `/authz/rules`、资源共享 `/authz/shares`、资源类型 `/authz/types`、审计 `/authz/audit`)

**Interfaces:**
- Consumes: Task 5 props.permissions;Task 6 代理;现行 shared 的 defineMicroApp/micro 约定(**读现行代码适配,勿发明第二套**)
- Produces: 六个空路由页(Task 8-11 逐页填实);`api/client.ts` 的 `http` 实例与错误提示约定

- [x] **Step 1: 脚手架照 apps/user 最小改造(删除 demo 页面,留布局与路由骨架)**
- [x] **Step 2: 菜单注册 + 基座集成验证**:dev 起基座+authz-admin,页签打开 `/authz/orgs` 空页成功、刷新恢复、props.token/permissions 注入生效(临时展示在页面上落报告)
- [x] **Step 3: 验证**:`pnpm --filter authz-admin build` 过;基座集成手测清单落报告
- [x] **Step 4: Commit**"Task15(B7): authz-admin 应用骨架——monorepo 接入/菜单注册/API 封装"

---

### Task 8: M5d 组织树页 + 成员管理

**Files:**
- Create: `console-frontend/apps/authz-admin/src/views/Orgs.vue`(el-tree 组织树 + 新建/改名/删除对话框 + 成员管理抽屉:成员列表、添加成员 UserSelect、移除)、`src/api/orgs.ts`(createOrg/listOrgs/updateOrg/deleteOrg/addMember/removeMember 包装)
**Interfaces:**
- Consumes: authz `/v1/admin/orgs*`(OrgResp{Id,Name,ParentId})、`/v1/admin/orgs/:id/members*`;api/client.ts;hasPermission(`authz:manage` 控制写按钮显隐)
- Produces: 组织树 CRUD 真实生效落审计(联调用)

- [x] **Step 1: api 包装 + 页面实现**(el-tree 懒加载或全量渲染均可,量级小;环错误 409 `AUTHZ_ORG_CYCLE` 提示"组织环")
- [x] **Step 2: 验证**:dev 手测——建根组织→建子组织→加成员→authz 侧 Check 判定联动(用 Task 12 验收脚本或 curl);`pnpm --filter authz-admin build` 过
- [x] **Step 3: Commit**"Task16(B8): 组织树页+成员管理(树 CRUD/成员抽屉/权限按钮级)"

---

### Task 9: M5e 平台角色页 + 规则表页

**Files:**
- Create: `src/views/Roles.vue`(内置角色列表 super_admin/operator/readonly + AssignRole/RevokeRole 到用户的表单)、`src/views/Rules.vue`(规则表表格 + UpsertRule 对话框:RoleKey、Prefix、Action 下拉 read/manage/空=服务级)、`src/api/{roles,rules}.ts`

**Interfaces:**
- Consumes: authz `/v1/admin/roles*`、`/v1/admin/rules*`(RuleResp{Id,RoleKey,Prefix,Action});角色白名单错误 400 `AUTHZ_ROLE_UNKNOWN` 提示
- Produces: 规则表变更 → /me/permissions 派生变化(Task 12 验收项"改规则→菜单实时反映")

- [x] **Step 1: 实现(两个 CRUD 表格页,照 Element Plus 表格+对话框惯例;规则表保存成功后 emit refresh-permission 事件——bus 可用则发,不可用则提示用户刷新,以现行 bus 约定为准)**
- [x] **Step 2: 验证**:dev 手测——给 admin 用户授 readonly→收回;新增规则 `readonly × order × read` 后 /me/permissions 出现 `order:read`(curl 验证)落报告
- [x] **Step 3: Commit**"Task17(B9): 平台角色页+规则表页(分配/撤销/规则 CRUD/权限变更联动)"

---

### Task 10: M5f 资源类型注册页 + 资源共享页

**Files:**
- Create: `src/views/ResourceTypes.vue`(已注册类型列表 + 注册对话框:Name、Actions 多选 [view,edit,delete,share])、`src/views/Shares.vue`(资源类型下拉 + 资源 ID + 目标用户 + 关系 viewer/editor 撤销列表;经 `POST /api/v1/authz/relations` 幂等写)、`src/api/{resourcetypes,shares}.ts`

**Interfaces:**
- Consumes: authz `/v1/admin/resource-types*`、`/v1/authz/relations`(**写白名单:仅 owner/editor/viewer/sharer 且类型已注册,403 AUTHZ_RELATION_FORBIDDEN 提示"不允许写该关系"**——页面只提供合法组合,漏网错误也要有提示文案)
- Produces: 共享真实生效(Task 12 验收"共享后对方立即可访问")

- [x] **Step 1: 实现(注册页注意:Actions 勾选驱动模型派生,can_view⇒member_view 等隐含契约在表单帮助文案说明)**
- [x] **Step 2: 验证**:dev 手测——注册 report 类型→共享 report:1 给 u2→authz Check u2 can_view=true(curl)落报告
- [x] **Step 3: Commit**"Task18(B10): 资源类型注册页+资源共享页(注册/共享/撤销/白名单提示)"

---

### Task 11: M5g 审计查询页

**Files:**
- Create: `src/views/Audit.vue`(表格:时间/操作人/动作/目标/状态;筛选:时间范围+操作类型+分页 items/total)、`src/api/audit.ts`(GET /v1/admin/audit?operator=&page=&pageSize=)

**Interfaces:**
- Consumes: authz `/v1/admin/audit`(AuditListResp{Items,Total});status 枚举 pending/committed/failed 徽标渲染
- Produces: 审计闭环可视化(Task 12 验收"审计记录完整")

- [x] **Step 1: 实现**
- [x] **Step 2: 验证**:做完 Task 8-10 的操作后页面能看到对应 pending→committed 记录;build 过
- [x] **Step 3: Commit**"Task19(B11): 审计查询页(时间/类型筛选/分页/状态徽标)"

---

### Task 12: M6 全链路联调验收 + 文档回填

**Files:**
- Modify: `docs/superpowers/specs/2026-09-06-authz-implementation-design.md`(状态改"已实施"+ 实施备注段)、`docs/permission-design.md`(附实施备注:go-zero 落地无语义变更、WriteRelations 白名单护栏注记)、`docs/frontend-design.md`(若用户未在改的部分:authz-admin 登记一段;**用户并行改动优先,冲突则只改自家 apps/authz-admin 的 README**)、`services/auth/README.md`(启动方式/配置/验收记录)
- Create: `services/auth/cmd/acceptance/README.md` 或直接在 auth README 记验收清单执行记录

**Interfaces:**
- Consumes: 全部前序任务 + Phase A authz + 本地 CAS(docker)+ MySQL(引擎/业务库)

- [x] **Step 1: 起全链路**:CAS docker → MySQL(已有库)→ bootstrap(authz)→ authz(:8084)→ auth(:8083)→ 基座+authz-admin(:5273/:5276)
- [x] **Step 2: 执行 spec §6 验收清单并逐项记录**(命令+输出):
  1. CAS 登录(https://localhost:8443 真实账号)→ callback → JWT 含 roles → /v1/me 200
  2. 管理界面建组织/授角色/配规则 → 操作落审计(pending→committed)
  3. 改规则 → /me/permissions 派生变化 → 基座菜单实时反映(refresh-permission)
  4. 无权限用户打受保护业务路由(sample Require)→ 403 AUTHZ_DENIED;停 authz → 503
  5. 共享资源给 u2 → u2 Check 立即 true;撤销后立即 false
  6. 无 token/过期 token → 401 AUTHZ_UNAUTHENTICATED(前端统一登出)
  7. 模型四场景在 `make test` 覆盖确认(Phase A engine 测试矩阵即 CI 入口,核对即可)
- [x] **Step 3: 文档回填(各文件最小改动)**
- [x] **Step 4: `make test/lint/build/contract-check` + `pnpm -r build` 全过**
- [x] **Step 5: Commit**"Task20(M6): 全链路联调验收 + 文档回填"

---

---

## Phase C 遗留清单(全分支终审 2026-09-07 分诊固化)

**强制项(对外演示/联调硬门槛)**:

1. 基座登录切 CAS + 前端接票页——callback 契约(POST vs CAS GET 重定向 405)一并定;基座 Login/SimpleLogin 仍指已不存在的口令端点(404)
2. MicroAppFrame(iframe 直连)token/permissions 运行期通道——未补则六页 UI 全 401(Task 12 实测);token 不进 URL
3. authz 重启模型重放(P1)——重启后已注册资源类型判定 503(fail-closed 半瘫),启动时重放 ModelVersionRepo 合并模型

**功能回填**:

4. 后端 AuditFilter 回填 action/时间范围参数(repo 结构已在),前端翻服务端过滤
5. 共享撤销端点(元组删除)——Shares 页目前仅授权
6. 角色分配存量回显接口(与 ListOrgMembers 同类)
7. kit ServiceUnavailable 构造器收口 + 503 码统一(Phase A 遗留,kit 双 owner)

**文档/体验顺手项**:backend-design §1 表 auth「规划中」过时;authz-admin README 登记段;demo「权限管理」条目清理;cas 3.0 p3 + attributes 复核(Email 回落 username);auth↔permission store 循环导入;fetch 并发去重;撤权容器销毁策略;Orgs 移除成员二次确认;直达无权限服务 URL 的空白引导;user 服务不在 workspace 的运行方式登记。

## Self-Review 记录

- **Spec 覆盖**:spec §3.2 auth(CAS/me/refresh/api-key)→ Task 1-4;§3.4 前端六页 → Task 8-11,基座接入+权限过滤 → Task 5-6;§4 M4/M5/M6 → Task 1-4/5-11/12;§5 测试策略(auth 单测:回调映射/续签/角色失败 fail-closed)→ Task 2-3 测试步骤;§7 文档影响面 → Task 12。排除项(网关/Redis 缓存/多租户切换/用户迁移 go-zero/CAS SLO)无任务,符合边界。
- **类型一致性**:TokenResp/MeResp/CasCallbackReq 在 Task 1 定义、2-4 消费;cas.Client/authzclient/repo 在 Task 2 定义、3-4 消费;hasPermission/usePermissionStore/props.permissions 在 Task 5 定义、6-10 消费;六路由 path 在 Task 7 注册表与 Task 8-11 页面一一对应。
- **占位符**:无 TBD;CAS serviceValidate 路径是 spec 明示的联调探测项(候选路径已给),非占位;前端无既有单测设施,Task 5 以 vitest 最小配置落地 permission.ts 测试,页面层以 dev 手测清单为验证(与 demo 现状一致)。
