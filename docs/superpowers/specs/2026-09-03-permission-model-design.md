# Nemo 权限模型设计(ReBAC / OpenFGA)

> 状态:评审稿 v1.0
> 日期:2026-09-03
> 范围:全平台授权体系——权限模型、auth 服务(身份与权限中心)、OpenFGA 决策引擎、kit authz 接入、前端权限链路、迁移与验收
> 关联文档:`docs/architecture.md`(§4.1 职责边界、§8.3 统一身份与权限中心)、`docs/backend-design.md`(§6 认证)、`docs/frontend-design.md`(§3.2 菜单、§4 会话、§6 事件)

---

## 1. 背景与目标

### 1.1 现状缺口

现有文档只设计了**认证**(JWT 签发校验、Traefik ForwardAuth、token 透传),**授权是留白**:architecture §4.1 只划了边界("授权决策归业务服务"),没有回答决策依据什么数据、用什么机制。散落的线索(基座"拉取用户权限列表控制菜单"、llm-proxy"按租户区分 key/配额"、frontend `refresh-permission` 预留事件)都没有统一落点。

### 1.2 需求确认记录(2026-09-03 评审)

| # | 需求项 | 结论 |
|---|--------|------|
| 1 | 主体形态 | **平台侧 + 租户侧两层**(平台运营人员 + 租户管理员/成员) |
| 2 | 授权粒度 | **功能权限(菜单/按钮/API)+ 资源实例级授权 + 组织树继承**——后两项是引入 ReBAC 的真实动机,纯 RBAC 表达不了 |
| 3 | 角色管理 | 租户侧可在控制台**自建角色**、勾选权限点;平台角色系统内置 |
| 4 | 存量兼容 | 旧体系有权限数据但**推倒重建**,一次性导入 |
| 5 | 生效时效 | 权限变更**即时生效**(排除"权限点全量进 JWT"方案) |
| 6 | 机器身份 | 本期只覆盖用户;API key / agent 主体归 llm-proxy 侧另行设计 |
| 7 | 登录协议 | 平台侧走**企业 CAS SSO**(auth 作 CAS 客户端);租户侧本地账密。若 CAS 服务端支持 OIDC 则优先 OIDC |
| 8 | 引擎部署 | **OpenFGA 进程级嵌入 auth**(用户决策,风险见 §6.3 落档) |
| 9 | 服务发现 | 本设计**不启用注册中心**,auth 地址走环境变量(与 demo 现状一致);Consul 启用节点由整体架构既定计划决定 |

---

## 2. 总体分层

**核心架构:管理面(auth MySQL)与决策面(OpenFGA)分离,auth 是唯一写者与唯一入口。**

```
管理面(写路径,低频)                      决策面(读路径,高频)
┌────────────────────────────────┐      ┌───────────────────────────┐
│ auth 服务(services/auth,8083)   │      │ OpenFGA(进程内嵌入,回环端口) │
│ · 登录(两通道)/JWT 签发           │      │ 关系元组存储(共享 MySQL)     │
│ · 租户/组织树/角色管理 API        │─写──► │ Check / ListObjects       │
│ · /internal/authz/check|grant    │      │ 官方 Go client 经 127.0.0.1 │
│ · 权限点目录聚合                  │      └───────────────────────────┘
│ · 对账任务(投影重建)              │
└────────────────────────────────┘
        ▲                ▲
业务服务 ×20(kit authz)  前端基座(/v1/me/permissions)
```

职责不变式(承接 architecture §4.1 的"验证/决策"分界):

- **认证验证**:Traefik ForwardAuth 调 auth 校验 JWT(demo 无网关时服务内 auth.Middleware 兜底,现状不变)
- **授权决策**:业务服务声明"本操作需要什么权限",决策数据与求值由 auth+引擎承担;资源归属比对("这订单是不是本租户的")仍在业务服务内按领域规则硬编码
- **权限数据只有一个写者**(auth),业务服务与前端只读决策结果——保证权威源唯一

---

## 3. 身份与认证

### 3.1 登录:协议适配层(两通道一出口)

**原则:登录协议只在 auth 出现,签发 JWT 之后全链路与协议无关。** 20 个业务服务与前端微应用永远不感知 CAS。

```
平台侧(员工):  浏览器 → /v1/auth/cas/login → 企业 CAS → /v1/auth/cas/callback
                            → auth 映射/创建本地用户 → 签发 JWT
租户侧(客户):  /v1/login(账号密码,校验后签发 JWT)
```

- 若企业 CAS 服务端支持 OIDC(Apereo CAS 6+ 支持),实现优先走 OIDC;否则实现 CAS 3.0 协议(`serviceValidate` 校验 ticket,标准库可完成,无重依赖)
- CAS 单点登出(SLO)推送处理为**可选项**,本期仅预留
- `users` 表增加身份源字段:`idp_type`(cas/local)、`idp_subject`(IdP 返回的唯一标识),与本地 id 联合定位用户

### 3.2 JWT:保持最小 Claims

```
Claims = { sub(用户id), name, tenant_id, exp }     // 12h TTL,HS256,kit/auth 现状不变
```

- **角色与权限点一律不进 token**——即时生效(需求 #5)的前提;权限每次 check 实时求值
- `tenant_id` 为当前会话的租户上下文;多租户用户登录时选租户、切换租户 = 换发 token
- token 仍经 kit httpx.Client 全链透传,服务间调用的鉴权语义不变

---

## 4. 权限模型

### 4.1 管理面数据(auth MySQL)

```
tenants            租户(id, name, status);tenant_id=0 为保留平台租户(代码常量 PlatformTenantID)
users              用户(id, name, email, password_hash, idp_type, idp_subject)
memberships        归属关系(user_id, tenant_id, status)——用户可属多租户
groups             组织树(id, tenant_id, parent_id, name)——parent 自引用,写入时校验防环
roles              角色定义(id, tenant_id, name, builtin, description)——租户侧可自建;平台角色 builtin
permission_points  权限点目录(id, service, code, description)——全局唯一,code 前缀=服务名
```

- **平台侧复用租户机制**:`memberships` 中存在 `(user, PlatformTenantID)` 即平台人员,角色取自平台租户的内置角色。不设独立的平台管理员机制,一套表一套逻辑
- 角色→权限点、角色/部门→成员的**绑定关系不入 auth DB**,由 OpenFGA 元组承载(见 4.2),auth DB 是这些关系的唯一事实源,经投影写入引擎

### 4.2 决策面:OpenFGA 关系模型(语义描述,非最终语法)

```
type user
type group        # 组织树:member(user 或下级 group)、parent
                  # "上级可见下级资源" = 授权给 group 后沿 parent 方向传递展开成员
type role         # assignee: user 或 group#member        ← 角色可直接绑人或绑部门
type permission   # can: user / role#assignee / group#member  ← 功能权限点对象
type order        # 业务资源类型(各服务按此模式逐个扩展):
  relations       #   owner / viewer / editor: user / group#member / role#assignee
                  #   can_view = viewer or owner;can_edit = editor or owner;...
```

三类授权的表达方式:

| 授权类型 | 求值 | 管理动作 |
|---|---|---|
| 功能权限 | `check(user, can, permission:ORDER_CREATE)` | 租户管理员建角色时勾选权限点 → 写 role→permission 元组 |
| 资源实例授权 | `check(user, can_view, order:123)` | 共享/协作 → 写 viewer/editor 元组;创建资源时写 owner 元组 |
| 组织树继承 | 资源或角色授权给 group,成员沿树传递生效 | 租户管理员维护组织树 |

- 权限点 code 命名 `<服务名大写>_<动作>`(`ORDER_CREATE`、`USER_KYC_AUDIT`),与错误码规则同构,全局唯一
- **资源类型演进**:OpenFGA model 是版本化 schema;新增资源类型(order、document…)= auth 仓库内 model 文件变更,随 auth 发布执行(等同 DB 迁移纪律)。自定义角色不需要动 model(通用 role/permission 对象承载)

### 4.3 双写与对账(必须如实认知的弱点)

管理面数据在 auth MySQL、决策面元组在 OpenFGA 引擎,构成**双写**。处理方式:

1. **auth DB 是唯一事实源**;引擎元组全部可由 DB 投影重建
2. 写入引擎失败 → 重试 + 告警;管理 API 在引擎写入确认前不返回成功
3. **对账任务**:auth 内定时比对"DB 投影 vs 引擎实际元组",漂移自动重建并告警;一次性迁移导入与对账重建使用同一工具(全量投影命令)

**即时生效承诺范围**:正常写入成功后的 check 可见性(check 调用取读己之写一致性档位);不含引擎故障期间的变更传播。

---

## 5. auth 服务设计

### 5.1 API

**对外(经网关,JWT 保护,管理界面后端):**

| API | 使用方 | 说明 |
|---|---|---|
| `POST /v1/auth/cas/login`、`GET /v1/auth/cas/callback` | 基座 | 平台侧 CAS 登录流程 |
| `POST /v1/login` | 基座 | 租户侧账密登录 |
| `GET /v1/me`、`GET /v1/me/permissions` | 基座 | 会话恢复;权限点 code 列表(ListObjects 求值) |
| `PUT /v1/me/tenant` | 基座 | 切换租户(换发 token) |
| `/v1/tenants*` | 平台管理员 | 租户管理;保留租户不可改 |
| `/v1/groups*` | 租户管理员 | 组织树增删改(防环校验) |
| `/v1/roles*`、角色绑权限点/绑人/绑部门 | 租户管理员 | 角色自定义全流程 |
| `/v1/members*` | 租户管理员 | 成员管理;移出即时生效(check 实时求值) |

**对内(`/internal/` 前缀,服务间调用,kit authz 唯一入口):**

| API | 说明 |
|---|---|
| `POST /internal/authz/check` | 功能权限或资源权限求值;user 从透传 JWT 解析,不显式传参 |
| `POST /internal/authz/grant` | 写资源元组(归属/共享)。**auth 先校验请求者本人对该资源的委托权限,通过才写入**——请求者的 JWT 是唯一凭据,服务不持有引擎凭据 |
| `POST /internal/permission-points` | 服务启动时幂等注册本服务权限点目录;code 冲突启动报错 |
| `GET /internal/verify` | Traefik ForwardAuth 的 token 校验端点(架构 §4.1 既有设计,本设计补齐) |

**调用方式(需求 #9:不启用注册中心)**:业务服务经环境变量 `AUTH_SERVICE_URL` 获取 auth 地址(与 demo 现状 `USER_SERVICE_URL` 同模式);Consul discovery 由整体架构既定计划在统一节点启用,业务代码形态不变。

### 5.2 权限点目录注册流

1. 各服务在自有 `openapi.yaml` 每个操作声明 `x-permission: ORDER_CREATE`(契约归服务所有的既有原则)
2. 服务内维护权限点清单(code + 描述 + 所属资源类型),以常量包形式供 handler 引用
3. 服务启动时向 auth 幂等注册(`POST /internal/permission-points`),auth 落库供管理界面勾选
4. CI 校验:权限点 code 前缀 = 服务名;openapi `x-permission` 与注册清单一致

---

## 6. OpenFGA:选型与嵌入形态

### 6.1 选型结论:OpenFGA

| | OpenFGA | SpiceDB | Permify | Topaz |
|---|---|---|---|---|
| 治理 | Auth0/Okta 发起,CNCF | Authzed 公司 | 初创(YC) | Aserto 公司 |
| API | **REST/JSON 原生 + 官方 Go SDK** | gRPC 优先,REST 附属 | REST/JSON | REST/gRPC |
| 存储 | PostgreSQL 一等;MySQL 成熟度待 PoC | PG/MySQL/CRDB | PG 为主 | 内嵌 |
| 社区 | 最大开源 Zanzibar 实现 | 第二 | 较小 | 较小 |

决定性理由:①REST/JSON 原生,契合全栈"不引入 gRPC/proto"的既定决策(SpiceDB gRPC-first 冲突);②CNCF 治理 + 最大社区;③auth 把引擎完全包在身后,选型可逆——将来换引擎只动 auth 一个服务。

### 6.2 嵌入形态(需求 #8:进程级嵌入)

```
auth 进程(services/auth,单容器)
├── OpenFGA server:以库形式在进程内启动,监听 127.0.0.1:<内部端口>,不对外暴露
│     storage = 共享 MySQL(openfga schema)
└── auth 业务:经官方 OpenFGA Go client 访问回环地址
```

护栏:**auth 始终通过官方 HTTP client + 回环地址访问引擎,不 import 引擎内部包。** 收益:

- 将来抽离为独立容器只改一个 base URL 配置,代码零改动
- 引擎模块升级只走其对外承诺的 server API,不啃内部实现

### 6.3 风险清单(决策落档,如实告知)

进程级嵌入(相对独立部署)的代价,用户知情并选定:

1. **故障域合并**:管理面与引擎任一方内存问题/panic 带走整个 auth;独立部署时引擎挂仅影响决策面,auth 可报"引擎不可用"
2. **失去独立重启**:授权 model 变更、引擎模块升级需要重启整个 auth
3. **升级耦合**:OpenFGA 模块版本随 auth 发版,升级窗口合一
4. OpenFGA 官方产品形态是独立 server;进程内以库形式启动属可用但非重点支持的用法,升级需关注 release note

### 6.4 PoC 验证项(实施第一步,与 llm-proxy PoC 同纪律,结论落档后再实施)

1. **MySQL datastore 成熟度**(最大不确定点):功能 + 压测。退路:引擎抽离为独立容器 + 专用 PostgreSQL(一行配置可切),或改用 SpiceDB
2. 进程内嵌入(库启动 + 回环 client)稳定性
3. Check 延迟基线(组织树深、元组多时的求值性能)
4. 组织树传递关系、角色组合、资源共享的模型语义验证

---

## 7. kit `authz` 包(业务服务接入)

纯机制、不含业务名词,符合 kit 三纪律;`Checker` 为接口(单测 fake 注入),kit 单测全覆盖。

```go
// main.go 装配
checker := authz.NewRemote(authz.Config{AuthURL: os.Getenv("AUTH_SERVICE_URL")}) // httpx.Client:超时/熔断默认值,JWT 自动透传

mux.Handle("POST /v1/orders", authz.Require(checker, order.PermCreate)(h.createOrder))
```

| 能力 | 形态 | 语义 |
|---|---|---|
| 功能权限 | `authz.Require(checker, "ORDER_CREATE")` 路由中间件 | deny → 403,错误码 `AUTHZ_DENIED` |
| 资源权限 | `authz.CheckObject(ctx, checker, "order", "viewer", orderID)` | service 层调用 |
| 元组写入 | `authz.GrantObject(ctx, checker, "order", orderID, "owner", userID)` | 创建资源/共享时调用,经 auth 校验后写入 |

行为约定:

- **fail-closed,区分错误类型**:check 网络失败/熔断 → 503(kit errors 新增 `ServiceUnavailable` 构造器),绝不降级放行;与明确无权的 403 严格区分,前端对 503 不登出、不刷新权限
- **默认无缓存**:即时生效是硬需求;缓存列为 kit 演进项(届时必须带失效机制)
- 中间件链:`RequestID → Recover → AccessLog → auth.Middleware` 之后,`authz.Require` 挂具体路由;公开/业务路由分 mux 的既有规则不变

---

## 8. 前端权限链路

- **登录**:基座发起(平台侧 CAS 跳转 / 租户侧账密),auth 签发 JWT;后续基座行为与通道无关
- **权限数据获取**:登录后调 `/v1/me` + `/v1/me/permissions` 存 pinia store;刷新恢复同链路
- **菜单过滤**:服务 manifest 菜单项增加 `permission` 字段(code 字符串);基座在首页目录卡片、左侧"我的菜单"、页签恢复三个入口统一过滤(`filterByPermissions` 工具收敛,微应用不做此事)——补齐 frontend-design §3.2"叠加后端权限过滤"的空缺
- **props 透传**:基座向微应用下发 `{ token, userInfo, permissions, base, entry }`(现有 props 增 permissions 项)
- **按钮级控制**:`packages/shared` 提供 `hasPermission(code)`,全站统一入口,禁止各应用自行实现
- **403/503 区分**:403(`AUTHZ_DENIED`)→ ElMessage"无权限" + 基座重新拉取 permissions(自动追平刚变更的角色);503 → 提示"权限服务暂不可用"重试,不登出、不刷新权限
- **`refresh-permission` 事件**(frontend-design §6 预留)由基座消费:触发即重拉权限、重算菜单

---

## 9. 部署、迁移、测试

### 9.1 部署

- `services/auth`:端口 8083(登记进 backend-design §7 端口表),双实例跨 VM,镜像内含嵌入式引擎
- 存储:共享 MySQL 的 auth schema + openfga schema;不新增基础设施
- 本期不注册 Consul(需求 #9);`/healthz`(存活)、`/readyz`(就绪,判定含 MySQL + 引擎连通)
- Traefik ForwardAuth → `GET /internal/verify`(随网关接入启用,demo 无网关时服务内 auth.Middleware 兜底,现状不变)

### 9.2 一次性迁移

1. 旧体系导出:用户(CAS 用户映射 `idp_subject`,本地用户含密码哈希)、租户、组织、角色定义
2. 落 auth DB(幂等导入脚本)
3. 执行**全量投影**命令生成 OpenFGA 元组(与对账重建同一工具)
4. 校验:抽样用户跑 check 比对旧体系权限结果

### 9.3 测试

| 层 | 覆盖 |
|---|---|
| kit authz | Checker 接口 fake 注入的单测;403/503 语义;熔断行为 |
| auth 服务 | 管理 API CRUD;check 语义(角色组合/组织树继承/资源共享/平台租户);CAS 回调流程;对账幂等与重建正确性 |
| demo 服务 | user/order 接入 authz,示范功能权限(Require)与资源权限(CheckObject/GrantObject)两端 |
| CI | openapi `x-permission` 与启动注册清单一致;权限点 code 前缀=服务名;kit 变更全服务构建验证(既有规则) |

### 9.4 验收标准

- 登录(两通道)→ 签发 JWT → check 授权 → 403/503 全链路在 demo(user+order+auth)打通
- 租户管理员建角色、勾权限点、绑人,前端菜单过滤实时反映
- 共享一个资源实例给另一用户,对方立即可访问(资源级授权演示)
- 组织树上/下级权限继承语义正确
- 改角色/移出成员后,下一次请求即生效(无需重登录)

---

## 10. 决策记录

| 决策 | 结论 | 否决的备选 |
|---|---|---|
| 授权模型 | ReBAC(OpenFGA) | 纯 RBAC 表达不了资源实例授权与组织树继承(需求 #2) |
| 引擎实现 | 采用开源 OpenFGA | 自研 Zanzibar-lite(一致性/性能深坑) |
| 引擎部署 | **进程级嵌入 auth**(回环 + 官方 client 护栏) | 独立容器(用户知情选择嵌入,风险 §6.3 落档) |
| 服务访问引擎 | 仅 auth,业务服务经 `/internal/authz/*` | 业务服务直连引擎(凭据面扩大、失去按请求者校验的写路径) |
| 权限数据位置 | 引擎元组,auth DB 为唯一事实源 + 对账 | 权限进 JWT(变更不即时);服务本地库(权威源碎片化) |
| check 机制 | 每请求实时求值,默认无缓存 | ForwardAuth 注入权限集合(header 体积/双路径);缓存+失效广播(过度设计) |
| 登录协议 | CAS 客户端(优先 OIDC if available)+ 本地账密,JWT 统一出口 | 20 服务各自对接 CAS |
| 服务发现 | 环境变量,暂不启用 Consul | 立即接入 discovery(需求 #9 明确后置) |
| 平台侧建模 | 保留租户(tenant_id=0) | 独立平台管理员机制(两套平行代码) |

## 11. 文档影响面(实施时同步修订)

- `docs/architecture.md`:§4.1 授权决策细化引用本设计;§8.3 统一身份与权限中心指向 services/auth;§11 决策表增补
- `docs/backend-design.md`:§6 扩为"认证与授权";新增 auth 服务接入规范与端口登记(8083);§7 环境变量增 `AUTH_SERVICE_URL`
- `docs/frontend-design.md`:§3.2 菜单项 `permission` 字段;§4 会话协议增 permissions;§6 `refresh-permission` 事件语义;§7 增 `hasPermission` 约定
