# 权限模型设计（ReBAC / OpenFGA）

> 状态：评审稿 v1.1
> 日期：2026-09-06（v1.0：2026-09-04；v1.1 增补：登录通道统一 CAS、租户自建角色预留、JWT TTL 与生效窗口、/me/permissions 派生口径、资源规模修正）
> 定位：全平台统一权限底座——网关路由级、前端菜单/按钮显隐、业务资源级、API key scope 四层授权的模型设计与实现方案
> 前置：`architecture.md` §4.1 两级授权模型预留的"权限模型独立项目"即本设计；定稿后按 §11 回填 architecture.md

---

## 1. 背景与范围

### 1.1 需求画像（2026-09 评审确认）

**覆盖范围**（四层全包）：

| 层 | 判定内容 | 消费方 |
|---|---|---|
| 网关路由级 | 平台角色能否访问某路由 | 网关 |
| 前端菜单/按钮 | 用户能看到哪些服务、页面、操作入口 | 前端基座 |
| 业务资源级 | 资源归属、租户隔离、组织层级继承、共享 | 业务服务 |
| API key scope | 程序化调用方能调哪些路由 | 网关 |

**授权场景**（四类全有）：

1. **平台角色**：super_admin / operator / readonly 等功能级角色
2. **组织层级继承**：集团 → 子公司 → 部门，下级组织成员对上级组织挂的数据有权（成员沿树向上聚合进父组织成员集合；给某部门授权则其子树全员生效，见 §4.2 `member from child`）
3. **资源归属与共享**：资源有 owner，可共享给他人（可撤销）
4. **多租户隔离**：不同租户数据严格隔离，租户内再分组织与角色

**约束**：

- 全新设计，无存量权限迁移（Java 栈无成型权限体系）
- 中小规模：用户 < 1 万、租户 < 50、资源千级（首批预估几百~几千，2026-09-06 确认）、判定 QPS 峰值千级
- 交付物含管理界面 API、用户自助共享、审计日志
- 部署环境：虚拟机 + Docker Compose，无 K8s；技术栈 go-zero + MySQL/Redis

### 1.2 模式选择：ReBAC

四类场景中，组织层级继承与资源共享是**关系上的传递判定**（"A 是 B 的上级组织的成员，B 是资源 R 的归属组织，所以 A 能看 R"）——这是关系型访问控制（ReBAC，Google Zanzibar 一族）的标准应用场景，简单 RBAC（角色 × 权限点矩阵）表达不了传递继承，逐服务硬编码则碎片化不可维护。平台角色（场景 1）在 ReBAC 模型里退化为一种简单关系类型，不冲突。

---

## 2. 选型：OpenFGA 引擎（方案 A）

### 2.1 为什么是 OpenFGA 而不是自研 / Casbin

| 方案 | 结论 | 理由 |
|---|---|---|
| **OpenFGA**（采用） | ✓ | CNCF 开源 Zanzibar 实现，Go 编写、**支持以 Go module 嵌入**（不部署独立服务）；重写规则（关系继承）、环检测、ListObjects 反向展开——ReBAC 最容易写错的部分引擎全包；模型声明式 DSL + 自带测试框架 + 多版本模型灰度 |
| 自研 Zanzibar-lite | 备选 | MySQL 递归 CTE 实现可行，中小规模性能无虞；但图遍历正确性（环、深度、重写语义、反向展开）自研自负，安全关键路径，估 2~4 周 + 长期维护 |
| Casbin | 否决 | 角色图继承表达不了"组织层级 + 资源归属"的传递闭包，四类场景只能干净做两类，退化成半个自研 |

决定性因素：四类场景全有 + 列表页反向展开（ListObjects）是控制台刚需——两者都落在"引擎强项、自研高危"的区间。

### 2.2 存储：复用现有 MySQL（已查证）

OpenFGA 官方支持的生产存储：PostgreSQL 14+、**MySQL 8**、SQLite (beta)。**MySQL 8 为正式支持**（社区贡献适配器），复用现有 MySQL，不引入新数据库。

注意事项：

- 独立 database（如 `nemo_authz`）+ 独立数据库账号，与业务库隔离，便于单独备份与迁移
- MySQL 引擎对 tuple 字段有比其他后端更严的长度限制——本项目关系数据（类型名、对象 id、用户 id）远在限制之内
- In-memory 存储仅用于本地开发，生产必须 MySQL

---

## 3. 总体架构：authz（内嵌引擎）+ 消费面

```
authz 服务（自研，一个进程）×2 双实例
services/authz
  ├ 管理 API：组织/成员、平台角色、路由规则表、
  │           资源类型注册、共享授权、审计查询
  ├ 判定 API：Check（单点判定）/ ListObjects（反向展开）
  │           ← 业务服务经 kit client（带缓存）调用
  ├ 审计与对账
  └ OpenFGA 引擎内核（Go module 嵌入）
       └ 原生 storage/mysql 适配器 → MySQL（nemo_authz 独立库）
  Redis：Check 结果缓存（kit client 侧）

消费面：网关（路由规则表，本地缓存）/ 业务服务（Check、ListObjects） / 前端基座（/me/permissions）
```

**三条铁律**：

1. **写路径只走管理 API**：所有关系变更（加成员、配角色、共享资源）必须经 authz 的管理 API——authz 负责合法性校验 + 写引擎 + 落审计。业务团队不直接调引擎写接口（这也是审计能闭环的原因：引擎不带审计，写路径收敛到 authz 才有完整审计面）。
2. **读路径只走判定 API**：业务服务经 kit 封装的 client 调 authz 的 Check / ListObjects（带 Redis 缓存）。判定与管理同进程但不同 handler 组——判定路径不经过任何管理逻辑与校验开销。
3. **网关粗粒度不实时依赖 authz**：路由规则表在网关本地内存，authz 宕机不影响存量路由级判定（§9）。

**嵌入的代价（明记）**：管理与判定同进程，authz 任何发版都会重启判定面——存量 Check 缓存仍在 Redis（kit client 侧），未命中部分在重启窗口 fail-closed。接受该代价的依据：authz 发版低频（管理面迭代为主），发布走摘流量流程（架构 §7.2）缓解；换来的是少一个部署单元、少一条运维链路，且判定 API 由 authz 直接定义（不再受独立引擎的 API 形态约束）。

### 3.1 组件定位

| 组件 | 回答的问题 | 职责 | 变化频率 |
|---|---|---|---|
| **auth**（认证服务） | 你是谁 | CAS 对接与用户映射（平台侧与租户侧统一走 CAS，无本地账密通道；开发期对接本地简易 CAS，上线仅改服务地址）、JWT 签发/续签/吊销（TTL 1h + 静默续签，§5.1）、API key 凭证生命周期（生成、存储、吊销）、登出（含 CAS SLO） | 认证协议与凭证规范，几乎不变 |
| **authz**（授权服务，**内嵌 OpenFGA 引擎**） | 你能做什么 + 此刻行不行 | 管理：组织/成员/角色/规则表/资源类型/共享/审计；判定：Check / ListObjects（引擎内核以 Go module 嵌入，无独立部署） | 业务组织形态与权限策略，中频迭代 |

故障半径：auth 挂 = 全站登录不了；authz 挂 = 权限变更停止 + 缓存未命中的资源级判定 fail-closed，路由级/菜单级不受影响（本地缓存）。两个故障半径不绑在一个进程里。

边界案例的归属：

- **登录**：auth 校验 CAS ticket → 调 authz 取用户平台角色 → 把 user_id / tenant / roles 写入 JWT 签发。auth 不懂角色含义，只负责装进凭证。平台侧与租户侧统一走 CAS（无本地账密通道），开发期对接本地简易 CAS 服务，上线仅改服务地址。
- **续签**：JWT TTL 定为 1h；基座在剩余寿命低于阈值（如 10 分钟）时携未过期 token 调 auth 续签接口换发新 token，auth 实时重取角色（复用登录链路调 authz），不走 CAS。活跃用户无感，是路由级角色变更生效窗口（§5.1）的配套机制。
- **API key**：key 本体（字符串、状态、过期）是凭证，归 auth 签发/吊销；scope 的路由口径对照 authz 维护的规则表。网关运行时只看 key 内的 scope 声明，不回源。
- **`/me/permissions`**：权限展开是授权域，归 authz 提供（auth 只发 token）。

---

## 4. 授权模型

### 4.1 类型与关系总览

| 类型 | 说明 | 关键关系 |
|---|---|---|
| `user` | 主体（人） | —（用户与权限的关系全部挂在其他类型上） |
| `org` | 组织树节点，**租户 = 树的根** | `parent` / `child` / `member` / `admin` |
| `platform_role` | 平台角色（super_admin / operator / readonly…） | `assignee` |
| `permission` | 功能权限点本身（去角色化），对象 id = 权限码（如 `redis:read`、通配 `*:*`） | `can` |
| `<resource_type>` | 业务资源类型（order、report…），各服务向 authz 注册 | `org` / `owner` / `editor` / `viewer` / 共享关系 / `can_*` 出口 |
| `api_key` | 程序化主体 | **不建资源关系**（§5.4），资源级以 owner 身份判定 |

### 4.2 模型 DSL（核心示例）

```text
model
  schema 1.1

type user

type org
  relations
    define parent: [org]                            # 父指针（org:父#parent@org:子），防环遍历用
    define child: [org]                             # 子指针（org:子#child@org:父），判定聚合用
    define admin: [user] or admin from child        # 组织管理员：直属 + 子树管理员沿 child 向上聚合
    define member: [user] or admin or member from child   # 成员 = 直属成员 + 本组织管理员 + 子树（直属及全部子孙）全部成员

type platform_role
  relations
    define assignee: [user, org#member]             # 角色可授给组织，成员（含子树继承）即获得该角色

type permission                                      # 功能权限点本身是引擎对象（去角色化后）
  relations
    define can: [user, org#member]                  # 授权 = 人/组织 × 权限码一条 can 元组

type order                                          # 业务资源类型示例，各服务注册时替换
  relations
    define org: [org]                               # 资源挂组织树 → 租户隔离天然成立
    define owner: [user]                            # 归属人
    define sharer: [user] or owner or admin from org    # 可共享权：显式授予 + owner + 组织管理员
    define editor: [user] or owner                  # 可编辑：显式授予 + owner
    define viewer: [user] or editor                # 可见：显式授予 + editor 链
    define member_view: viewer or member from org   # 组织成员可见（按资源类型选配——机密资源不配）
    define can_view: member_view                   # ── 权限出口，业务代码只查这些
    define can_edit: editor
    define can_delete: owner                        # 删除仅 owner 专属（editor 不含删除）
    define can_share: sharer
```

### 4.3 设计要点

- **租户隔离是结构不是规则**：资源必须挂 org 节点，跨租户之间不存在任何关系路径，隔离由模型拓扑保证，不靠某条 deny 规则。
- **单一 Store**：全平台共用一个 OpenFGA Store，租户 = org 树根。**不采用 store-per-tenant**（每租户一个 Store）：Store 之间关系图不连通，跨租户的平台角色（super_admin、审计员）在 FGA 内表达不了，只能绕到库外——那是我们"权限判定单一出口"要避免的；租户 < 50 的量级单 Store 无任何压力。
- **层级继承靠 `member from child` 重写（模型 v5）**：给任一部门授权，该部门子树内（直属 + 全部子孙部门）的人员均获得权限——与"给父部门全员生效"的直觉一致；授权叶子不再影响兄弟/祖先部门。组织层级同时写 `parent`（防环）与 `child`（聚合）两条互逆元组，传递性与环安全由引擎保证（自研最容易错的部分归引擎）。
- **条件关系（CEL）暂不启用**：引擎原生支持条件关系（时间窗口、IP 范围等，OpenFGA CEL conditions），当前无需求不启用；将来需要时是模型配置层面的开关，不是自研开发项。
- **权限出口统一命名**（`can_view` / `can_edit` / `can_delete` / `can_share` / `can_manage`），业务代码只查出口，不感知关系组合细节；资源类型想改授权口径（如"组织成员是否可见"）只改模型定义，调用方零改动。
- **防越权转授**：共享权（`sharer`）本身是模型里的关系——只有 owner 和组织管理员能共享，被共享的 viewer 不能转授给第三人。
- **资源类型声明驱动派生（隐式注册）**：类型不再独立注册——资源类型就是"资源驱动菜单 + 资源级按钮"声明的资源归类（写菜单/按钮时即定义类型与出口，消除先注册后挂菜单的割裂）。派生规则：菜单行带 `resourceType` ⇒ 该类型隐含 `can_view`；资源级按钮（`relation` 非空 + `resourceType` 必填）⇒ 贡献该 `relation` 出口；类型 actions = 并集按引擎白名单顺序渲染。菜单写路径（create/update/delete）落库后自动同步注册表与引擎模型（`logic/resourcetypesync.go`），初始化 seed（`migrations/008_menu_seed.sql`，2026-10-11 起 yaml/apply-config 下线）与页面操作同一机制。存量注册表保留（只读列表供授权/名单聚合），管理码为存量能力、派生保留历史值。
- **平台角色的路由权限**：platform_role → 路由前缀的映射不在 FGA 模型里（它不是资源关系），是 authz 维护的规则表数据（§5.1）。
- **租户自建角色预留不实现（2026-09-06 确认）**：模型不设租户内自定义角色与权限点目录，功能权限 = platform_role（规则表承载）+ 资源 can_* 出口（模型承载）。将来需要时新增类型与关系即可，是加法不是改造。

---

## 5. 四层判定数据流

### 5.1 网关路由级（消费方：网关）

- 数据源：authz 维护"平台角色 → 路由前缀"规则表（管理界面可配），authz 将其**扁平化导出**（角色名 × 前缀清单，量级 KB）到 Redis。
- 网关启动拉取 + 定时刷新（Redis 变更通知或 30s 轮询），判定时**纯本地内存比对**：JWT claims 里的 roles × 规则表 → 403 或放行。
- 网关不实时调 authz；authz 宕机不影响存量路由级判定。
- **角色变更的生效窗口（已定语义）**：roles 只在签发时写入 token，无状态 JWT 不可吊销——per-user 的角色收回/授予在路由级最迟一个 TTL（1h）内生效，窗口内数据面不受影响（资源级 Check 实时、fail-closed）。规则表本身的变更（角色能访问哪些路由）仍 30s 生效，与 token 无关。
- **立即踢出（预留，不实现）**：离职/封禁需即时生效时，由 auth 维护用户级吊销名单导出到 Redis，网关随规则表一起 30s 刷新本地比对，命中即 401（复用既有导出-刷新机制）。触发条件：合规/安全要求"立即踢出"；当前数据面已实时收紧，无此硬需求。

### 5.2 前端菜单/按钮（消费方：前端基座）

- authz 提供 `GET /api/v1/me/permissions`：**由路由规则表派生，不建独立权限点目录**——用户各角色允许的路由前缀归并出"服务 × 动作"集合（如 `["user:read", "order:manage", "llm:use"]`）。与网关判定同源（同一张导出规则表），口径天然一致，无需对账机制。
- **动作级一次到位（2026-09-06 确认）**：动作 = 规则表中路由前缀的分组标签，authz 规则表管理界面配置前缀时打标；派生逻辑服务级与动作级是同一套，无额外阶段。菜单显隐按 code 前缀（服务级）匹配；按钮级用 `hasPermission("<服务>:<动作>")`（packages/shared 统一入口）。
- 基座登录后拉取一次并缓存；权限变更经 wujie bus `refresh-permission` 事件广播刷新（frontend-design §6 已预留该事件）。
- 菜单显隐逻辑：基座菜单注册表 × 权限集合取交集，服务不可用即隐藏（不是置灰——用户不需要看见自己没有的东西）。隐藏语义本身 fail-closed（code 不在集合即隐藏）；dev 环境对"服务存在但菜单 code 无法匹配"打告警即可，不建跨仓 CI 对账。
- **资源级行内按钮不由前端判权**：依赖具体对象的操作（如"共享这一行"），由业务接口在响应中下发每对象可用操作（服务端 Check 后带出，如 `my_perms: ["view", "edit"]`），前端只渲染；该能力随各服务接入资源级判定自然具备，平台层无独立排期。

> **现状更新（2026-09-16，去角色化 + 菜单落库 + 方案 B）**：
>
> - **数据源**：`GET /v1/me/menus` 已改为从**权限元组**派生（`type permission` 的 `can` 元组，ListObjects 求值），不再由角色规则表派生；菜单注册表落库（`menus` 表，管理页可维护，见 authz README / §6）。
> - **方案 B：资源驱动菜单**——菜单可见性 = `permission_code`（read 码）被授予 **或** 该菜单声明 `resource_type` 下用户存在 `can_view` 级关系（viewer/editor/owner，ListObjects 判定）。声明方式：菜单注册表 `menus.resource_type` 列（如 `kafka:topics` → `topic`）。语义：**资源级授权自动解锁菜单入口**，授权只授一处；页面内数据仍按资源级过滤（菜单可见性与数据可见性脱钩）。未声明 `resource_type` 的菜单保持纯 read 码语义。
> - 上述模型取代 §5.2 上半部分"角色 × 规则表"的描述（该段为历史设计，保留存档）。
> - **类型来源统一（2026-09-16）**：`topic` 原内置在 `model.fga` 基础模型（Phase C 演示域），已迁移为注册类型并按命名规范改名为 **`kafka_topic`**（注册表 `resource_types`，actions=can_view/can_edit/can_delete），存量授权元组/菜单声明/ABAC 策略/注册表块已随改名迁移生效（2026-09-23 迁移文件整合清理）；基础模型不再含 topic/level_ok；注册模板的 `viewer` 支持 `org#member`、`can_view` 含 owner，与内置语义对齐。基础模型现仅保留框架类型：`user`/`org`/`platform_role`/`permission`/`order`。
> - **类型声明驱动派生（2026-09-18，隐式注册）**：独立注册/编辑/删除 API 与 `deploy/resource-types.yaml` 已下线——类型由菜单/按钮声明派生（§4 按钮实体化规则 + §6.3）：菜单行带 `resourceType` ⇒ 隐含 `can_view`；资源级按钮 `relation` ⇒ 贡献出口；actions 取并集按白名单顺序渲染。菜单写路径（create/update/delete）落库后调 `SyncResourceTypesFromMenus`（`logic/resourcetypesync.go`）同步注册表 + 重建引擎模型（与启动重放同源 modelmerge）；页面级按钮禁声明资源类型（400）。存量注册表保留（只读列表 + 授权聚合），名单管理码为存量能力、派生保留历史值，未被引用类型不自动删除。
> - **名单策略收敛（2026-10-11）**：kafka_topic 专用名单分支（`resourcegrantcommon.go` 的 topicCanManage/parseTopicID 等）已删除，读/写/撤销名单统一走注册配置驱动的通用策略（`grantpolicy.go`，管理码来自注册表 `ManageCode` 声明）；新增管理面配置接口 `PUT /v1/admin/resource-types/{name}` 更新 ManageCode（替代手改库）。管理路径放行后叠加 ABAC（deny 命中即拒绝，与资源判定面同语义——`grantsManageable`）。管理码语义统一为 owner-or-管理码、**不含 editor**（editor 可编辑属性不放大为名单管理权）。
> - **注册模板跨团队隔离（2026-09-23）**：模板 `editor` 支持 `org#member`（组织可按资源批量授可编辑——张三团队只编辑自己团队的 topic）；`can_delete` 固定为 `owner or editor`（可编辑者即可删除，避免查看/编辑/删除三档独立配置；名单管理权仍收口 owner/管理码防提权）。类型声明变更（菜单/按钮增删改）即重渲染生效。
> - **按钮实体化（2026-09-28，菜单就近声明）**：按钮从菜单行 `actionCodes` 字符串升级为 `menus` 表一级实体（`type=menu/button`，migration 003 加 `type/parent_key/relation` 三列），按钮行挂父菜单下（`parent_key`），不入侧边栏（`/v1/me/menus` 过滤）。**两类按钮语义**：
>   - **页面级按钮**（`relation` 空，如 新建/导出）：无资源实例的操作，授权 = 功能权限勾选，权限码即 `serviceKey`（`服务:动作` 形态，如 `kafka-topics:create`），复用既有 permission 元组机制、零注册；
>   - **资源级按钮**（`relation` 非空 + `resourceType` 必填，如 冻结/共享某行）：有资源实例的操作，授权跟随该资源「数据权限」三档（`can_view/can_edit/can_delete`），授权界面只读展示、不单独勾选——**复用优先**：常见操作直接映射既有 `can_*` 关系，不新增语义。
>   - **就近声明 + IaC 对账（fail-closed）**：写完页面直接在菜单管理里声明按钮（授权界面自动出现），不再去资源类型单独注册、解决流程割裂；资源级按钮的 relation 必须命中引擎出口白名单 `can_view/can_edit/can_delete/can_share`，超出报错（新语义需先扩展引擎模型并重放，禁止静默裸奔）。声明经菜单管理页/008 seed 基线落地（2026-10-11 起 `deploy/menus.yaml` + `scripts/apply-config.py` 已下线；drift 对账由管理面 CRUD + seed 幂等取代）。
>   - **迁移**：既有 `actionCodes` 字符串（如 `kafka:topics=[create,export]`）由 IaC 声明驱动迁为按钮行（菜单行 `actionCodes` 清空），授权界面兼容读取两者（按钮行优先、旧目录码并入勾选）。
>   - **三级菜单（2026-09-28）**：菜单行放开 `parent_key` 约束——`parent_key` 空 = 服务根行/一级菜单（key 冒号 ≤1 段）；`parent_key` 非空 = 二级菜单（key 恰好 2 段冒号 `服务:一级:二级`，父级须为存在的一级菜单行，type=menu）。按钮行父级可为服务根行/一级/二级菜单行，禁止按钮再挂按钮。删除菜单递归级联删除全部后代（子菜单行 + 按钮行）并回收孤儿授权码。`/v1/me/menus` 下发 `parentKey`，基座侧边栏按它组装嵌套（服务 → 一级菜单 → 二级菜单）；授权界面与菜单管理按同构三级树展示（服务分组 → 一级 → 二级），按钮仍聚合在所属菜单行。
> - **菜单模型简化（2026-10-08，id 引用 + permission_code 统一列）**：按 docs/menu-simplify-design.md 终稿落地（migration 007）——`menus` 表删除 `service_key/parent_key` 键名列，行身份 = 自增 `id`、父子引用 = `parent_id`（顶级为 0，三级层级约束改沿 parent 链校验）；菜单码与按钮授权码合并为 `permission_code` 一列（`服务:动作` 格式不变，UNIQUE 索引，空码存 NULL，重复 409）。**码语义（方案 B）**：菜单行码可选——空 = 全员可见（不过滤，准入兜底在网关路由/API 权限/按钮码层），非空 = 准入门槛；页面级按钮码必填；资源级按钮无码（授权跟随资源关系三档）。API 随之调整：`PUT/DELETE /admin/menus/:id`（原 `:key` 路由删除）；`/me/menus` 下发 `id/parentId`（不再有 serviceKey/parentKey），前端按 path 首段归组服务（`/{service}/{page}` 路由约定）、按 parentId 组装树；按钮声明查询改 `GET /v1/authz/menu-buttons?path=<父菜单路由>`，`MenuButtonItem.serviceKey` 改名 `permissionCode`。资源类型/关系机制不动（`SyncResourceTypesFromMenus` 派生保留，§5.2 类型声明驱动派生规则照旧）。`deploy/menus.yaml` 比对锚（serviceKey/parentKey）随之失效（2026-10-11 文件已删除，菜单事实源改为库 + 008 seed 基线）。

### 5.3 业务资源级（消费方：业务服务）

- **单点判定**：handler 拿到资源 id 后调 kit client：`authz.Check(ctx, "order:123", "can_view", user)` → bool。
- **列表过滤**：用 ListObjects 反向展开"该用户可见的 order id 集合"作为查询条件，**禁止逐行 Check**（一页 50 行 = 50 次判定的反模式）。当前资源规模千级（§1.1），展开结果直接作查询条件即可，无需裁剪/分页策略。
- kit client 内置：Redis 结果缓存（§7）、authz 地址发现（注册中心）、超时与熔断（go-zero breaker）。

### 5.4 API key scope（消费方：网关；已定决策）

**只做路由前缀 scope，不做 key 级细粒度资源授权（YAGNI）**：

- key 内声明 scope = 路由前缀白名单（如 `["llm", "report"]`），auth 签发时写入，网关本地比对，越界 403。
- 资源级判定以 key 的 **owner（签发人）身份**走正常 Check——key 的实际能力 = scope 允许的路由 ∩ owner 本人的资源权限，任一不满足即拒绝。
- 不做的部分：逐 key 配置资源清单（GitHub 细粒度 PAT 模式）。理由：当前程序化调用方是 agent 与内部脚本，无此需求。扩展路径保留：将来需要时把 `api_key` 作为主体写入 FGA 关系即可，是加法不是改造。

**key 管理规范（auth 实现遵循，安全基线）**：

- 只存 SHA-256 hash，不存明文；创建时一次性展示原始 key
- 支持过期时间、备注（标签说明用途）、吊销
- 记录 `last_used_at`（网关校验命中时滑动更新），用于发现僵尸 key 与审计

---

## 6. 管理面设计（services/authz）

### 6.1 API 面（REST，经网关，走统一认证）

| 模块 | 接口（示意） | 使用方 |
|---|---|---|
| 组织管理 | 组织树 CRUD、成员增删移 | 对内控制台（管理员） |
| 角色管理 | platform_role 定义、assignee 分配 | 对内控制台 |
| 规则表 | 角色 → 路由前缀规则维护、导出状态查询 | 对内控制台 |
| 资源类型声明 | 菜单/按钮行声明类型与 can_* 口径，派生生成 DSL 块（§6.3） | 各服务团队在菜单声明（IaC/管理面）中配置 |
| 共享授权 | 资源共享 / 撤销 / 共享列表（owner 自助） | 业务控制台（普通用户） |
| 权限查询 | `/me/permissions`、资源的可见者列表 | 前端基座、业务服务 |
| 审计 | 审计日志查询 / 导出 | 对内控制台 |

### 6.2 写入语义与审计

**幂等**：authz 写接口服务端查重——目标 tuple 已存在则跳过写入，审计只记实际生效的变更。重复提交（管理界面双击、重试）不产生重复数据与重复审计。

**审计一致性**（写 OpenFGA 与写审计表是两个存储，无法共事务，必须显式设计）：

1. **审计先行**：先落审计记录（状态 `pending`，含变更内容快照）
2. 写 OpenFGA 成功 → 审计记录标记 `committed`
3. **定时对账**：`pending` 超时未 `committed` 的记录回查 OpenFGA 实际状态并修正——已生效补记 committed，未生效标记 failed

杜绝"权限改了但没审计"与"有审计但没生效"两种不可见中间态。

**审计内容**：所有写操作（谁、何时、改了哪条关系/规则、变更前后值、来源 IP、操作入口）落 **MySQL 审计表**（authz 自己的库，与 OpenFGA 的 nemo_authz 库分开）。

- 审计只记管理面写路径——读路径（Check）不落审计（量级与隐私都不合适），需要行为审计的业务操作由业务服务自己记录。
- 审计表只追加不修改，导出走管理界面。

### 6.3 管理界面

界面本身归前端项目（对内控制台的一个微应用，按 frontend-dev 规范接入）；本设计交付其依赖的全部 API。第一版界面范围：组织树 + 成员 + 角色 + 规则表 + 共享管理 + 审计查询。

**授权模型管理（配置化生成，不做自由编辑）**：资源类型的 DSL 块由**注册配置驱动生成**——控制台表单（或注册 API）收类型名、权限出口勾选、选配开关（组织成员可见？管理员可编辑？可共享？），authz 按固定模板确定性生成（同参数同输出）。**配置项是白名单**：生成器只能产出受限模板的合法结构，控制台不可能配出绕过租户隔离的模型；**不做可视化拖拽建模 / 自由 DSL 编辑**。支持模型 **JSON 导出/导入**——备份、测试夹具、批量迁移用，导入走同一套模板校验。模型文件退化为配置数据的准自动生成产物，注册配置存 authz 业务库（可查询、可审计）。

**模型变更的生效机制（无需重启）**：授权模型是引擎里的**数据**而非代码——写模型 = 经 authz 管理 API 向引擎写入新版本（存 MySQL nemo_authz 库），Check 请求按模型版本（默认最新）判定。模型变更不重启任何服务：authz 不重启、业务服务无感知（它们只调 Check，不内嵌模型）；灰度 = 部分请求带旧版本 id 判定。模型版本变更时广播全量缓存失效（§7），防止旧版本算出的缓存结果残留。仅 authz 自身**代码**升级涉及重启，走架构 §7.2 摘流量流程。

**模型的存储位置（三层）**：源头 = 注册配置（authz 业务库的结构化数据，控制台操作的对象）；运行时真源 = 引擎内模型版本（MySQL nemo_authz，经管理 API 写入，带版本 id）；只读参照 = 模型 JSON 快照导出进 git（评审、diff、回滚对照），不作为部署来源。生成确定性保证：同配置同输出，从注册配置任意重生成皆安全。

### 6.4 资源类型生命周期与上线顺序

**推荐顺序：声明先行**——资源类型声明（菜单/按钮行）是服务发布 checklist 的一步：菜单声明提交 → 写路径自动派生模型新版本 → CI 模型测试 → 服务上线。

**支持"先上线、后配置"（fail-closed 兜底）**：未注册的服务调用判定，引擎因类型不存在报错 → 统一按 fail-closed 处理（403）。不出安全事故（宁拒不错），代价是该服务在补注册前不可用——所以只作应急路径，不作常规流程。

**资源生命周期的关系写入（跨服务两步，无分布式事务）**：业务库写入与 authz 元组写入是两个系统、无法做本地事务，按固定顺序 + 失败补偿落地（kafka_topic 已实施，可作参照）：

- **创建**：先写业务库拿自增 ID，再写 `owner` 归属元组——owner 远程写入对 5xx/网络类错误有限次重试（4xx 不重试）；重试仍失败则审计标记 failed、资源保留为"待对账"，**不能只告警就放任孤儿**（漏 owner = 创建人自己都看不到、资源无人可操作）
- **删除**：**先删业务库、后清元组**——顺序反了且删库失败会把资源锁死（元组没了、库记录还在，谁都管不了）；删除对"资源不存在"幂等返成功；清元组失败仅记录残留、不阻断（资源已删，业务目标已达成），残留由周期清理兜底
- **对账（reconcile）**：服务提供对账接口/任务，扫描业务表，对缺失 owner 的资源按 create_by 批量补写；引擎故障时 fail-closed（503），不做半吊子对账。kafka 范例：`POST /v1/kafka/reconcile` → `{scanned, repaired}`
- 这类写入走 §6.1 的资源关系写接口（用户/服务身份凭证），同样落审计

**存量补齐（backfill）**：服务先产生数据后补注册的场景，注册完成后由对账/backfill 任务从业务库读归属字段批量生成 tuple，幂等写入；backfill 完成 + 抽查校验通过后才切模型版本。

### 6.5 初始化引导（cmd/bootstrap）

部署时按序执行，幂等设计（可重复执行）：

1. 数据库迁移（nemo_authz 库 + authz 业务库）
2. 创建 OpenFGA Store（单 Store，§4.3）
3. 写入首版授权模型
4. 创建根组织（平台租户）与超级管理员——**超管声明配置化**（2026-10-13）：初始超管由 authz `etc/authz-api.yaml` 的 `Bootstrap` 段声明（`AdminUsers` 登录名列表 + `AdminOrgs` 组织编码列表，均可多个；未声明 `AdminUsers` 时默认 `["admin"]` 向后兼容）。引导对每个超管用户授 `permission:*.*` 全通配权限并挂根组织 member；对每个超管组织授其成员集合（`org:{编码}#member`）全通配权限——组织下全部成员继承超管。引导只做初始装配（WriteTuples 读后写幂等），超管变更走控制台授权页，配置移除不自动回收旧元组。
5. 生成初始 API key（首次登录前的机器调用凭证）
6. **初始化下发菜单/资源类型基线**（`migrations/008_menu_seed.sql` 由 authz `cmd/provision` 执行——新环境 `scripts/init-env.sh` 一键初始化；资源类型随菜单/按钮声明派生，seed 固化注册表管理码基线。2026-10-11 起 `deploy/menus.yaml` + `scripts/apply-config.py` 已下线）——运行时事实源是 authz 的 menus 表，生产经管理面 CRUD 维护（见 permission-guide §3）

---

## 7. 缓存与性能（中小规模）

| 判定 | 缓存策略 | 失效方式 |
|---|---|---|
| 业务 Check / ListObjects | Redis 缓存 `(subject, permission, object)`，TTL 30~60s；**缓存键含模型版本号** | 关系写入发布失效消息（Redis pub/sub）；模型版本变更广播全量失效 |
| 网关规则表 | 网关内存全量 | 定时刷新（30s）+ Redis 通知 |
| `/me/permissions` | 结果级缓存（用户 × 权限集合），TTL 与登录会话对齐 | 同失效消息 |

容量评估：判定 QPS 千级、Redis 命中后回源 authz 的 QPS 在百级以下，authz ×2（内嵌引擎）+ MySQL 单实例余量充足。

---

## 8. 部署

| 组件 | 形态 |
|---|---|
| services/authz ×2 | go-zero api 服务，**内嵌 OpenFGA 引擎内核**（Go module import；**版本锁定 >= v1.18.x**——修复 MySQL 大小写敏感 CVE-2026-55170，用 MySQL 存储必须）；跨 VM 双实例，随 Go 栈正常部署与发布（走架构 §7.2 摘流量流程） |
| MySQL（nemo_authz 库） | 复用现有 MySQL 实例，引擎经原生 storage/mysql 适配器直连；独立 database + 独立账号；备份纳入现有 MySQL 备份体系 |
| Redis | 复用现有 |

新增部署物：**仅 authz 服务镜像**（引擎编译在内）。无独立 OpenFGA 容器、无新数据库、无 K8s 依赖。

---

## 9. 失效语义（已定：fail-closed）

| 故障 | 影响 | 语义 |
|---|---|---|
| authz 不可用（含内嵌引擎） | ① 缓存未命中的**资源级判定**拒绝 ② 不能变更权限 ③ `/me/permissions` 新拉取失败 | **fail-closed**（拒绝，403）；缓存命中的判定正常。登录链路兜底：auth 在 Redis 缓存每个用户的角色（写入时更新），authz 不可用时登录用缓存角色签发 JWT——角色新变更在恢复前不生效，但不阻断登录 |
| authz 不可用 | 路由级 / 菜单级判定 | **不受影响**（网关规则表与 /me/permissions 缓存在消费方本地） |
| auth 不可用 | 全站无法登录 | 认证域故障，与本项目无关（双实例兜底） |
| Redis 不可用 | 缓存全部回源 authz | 性能降级不降正确性；authz（内嵌引擎）直接扛千级 QPS 仍可行 |

原则：**权限宁严勿松**。fail-closed 换来的是安全默认，可用性由双实例 + 缓存兜底。

---

## 10. 测试策略

1. **模型测试（最重点）**：OpenFGA 自带模型测试框架（`.fga` 测试文件，声明 tuple 断言），四类场景全部写成表驱动用例进 CI：
   - 租户隔离：跨租户零路径（任何角色组合都不可见）
   - 层级继承：三层组织的成员对下级资源的权限矩阵
   - 共享：owner / admin / 被共享者 / 无关者的权限矩阵；**越权转授**（viewer 给第三人授权必须失败）
   - 平台角色：assignee 与路由规则表的组合
2. **模型版本兼容**：模型变更时 CI 对新旧版本回放全量测试集；生产灰度切版本（OpenFGA 多版本并存）。
3. **authz 服务测试**：管理 API 越权用例（非 admin 改组织、共享无 can_share 的资源、伪造资源归属）；`/me/permissions` 派生口径（角色 × 规则表 → 服务×动作集合，含动作分组标签）。
4. **kit client 测试**：缓存命中/失效/回源、OpenFGA 超时熔断、fail-closed 行为。

---

## 11. 已定决策与回填清单

**已定**（v1.0）：

1. 模式：ReBAC；引擎：**OpenFGA**（自研与 Casbin 否决，理由 §2.1）。
2. 部署形态：**OpenFGA 以 Go module 嵌入 services/authz**，不部署独立服务；引擎经原生 storage/mysql 适配器使用**现有 MySQL**（MySQL 8 官方支持已查证），独立库 `nemo_authz` + 独立账号。
3. 组件分工：**auth（认证）/ authz（授权：管理 + 判定，内嵌引擎）**，故障半径分立（§3.1）。
4. 写路径收敛 authz（审计闭环），读路径业务服务直连 OpenFGA + kit 缓存封装。
5. API key：只做**路由前缀 scope**，资源级以 owner 身份判定；不做 key 级细粒度资源授权（YAGNI，模型留扩展路径）。
6. 资源级判定 **fail-closed**。
7. `/me/permissions` 由 authz 提供；前端菜单显隐 = 菜单注册表 × 权限集合，不可用即隐藏。
8. 网关路由规则表由 authz 扁平化导出，网关本地缓存判定，不实时依赖引擎。
9. 权限出口统一命名 `can_view/can_edit/can_delete/can_share/can_manage`；资源类型声明驱动派生（§6.3），模型版本灰度。
10. **单一 Store**：全平台一个 OpenFGA Store，租户 = org 根；不采用 store-per-tenant（跨租户平台角色需关系图连通，§4.3）。
11. **API key 管理规范**：仅存 SHA-256 hash、一次性展示、过期/备注/吊销、`last_used_at` 审计（§5.4）。
12. **authz 写入幂等** + **审计先行 + 对账**的一致性方案（§6.2）。
13. **初始化引导**（cmd/bootstrap）为 authz 交付物：迁移 → 建 Store → 首版模型 → 超管与根组织 → 初始 key，幂等（§6.5）。
14. OpenFGA 版本锁定 **>= v1.18.x**（MySQL CVE-2026-55170 修复，§8，以 Go 依赖形式随 authz go.mod 锁定）；判定 API 为 authz 自有 HTTP 接口（服务间调用走注册中心发现），不引入 gRPC/proto。
15. 授权模型支持 **JSON 导出/导入**，不做可视化拖拽建模（§6.3）。
16. 资源类型 DSL 由**注册配置驱动生成**：受限模板白名单（生成器只产出合法结构），不做自由 DSL 编辑；JSON 导入走同套模板校验；注册需服务 owner 发起 + 平台组审批双确认，全程落审计（§6.3）。
17. **资源类型生命周期**：注册纳入发布 checklist 先行（推荐顺序）；未注册调用判定 fail-closed（支持后配置，仅应急）；资源创建/归属变更后由业务服务经 authz 写 API 幂等落 tuple（失败告警）；存量数据走 backfill 任务补齐后才切模型版本（§6.4）。
18. **登录通道统一 CAS**（2026-09-06）：平台侧与租户侧统一走 CAS，无本地账密通道；开发期对接本地简易 CAS 服务，上线仅改地址（§3.1）。
19. **租户自建角色预留不实现**（2026-09-06）：功能权限 = platform_role + 规则表 + 模型 can_* 出口，不建租户内自定义角色与权限点目录（§4.3）。
20. **JWT TTL = 1h + 静默续签**：路由级角色变更最迟一个 TTL 生效，窗口语义写入 §5.1；"立即踢出"走预留的用户级吊销名单方案，当前不实现（§5.1）。
21. **`/me/permissions` 由规则表派生，动作级一次到位**（2026-09-06）：不建权限点目录；动作 = 规则表前缀分组标签；资源级行内按钮由业务接口下发 `my_perms`（§5.2）。
22. **资源规模下调为千级**（2026-09-06，首批几百~几千）：ListObjects 展开结果直接作查询条件（§1.1、§5.3）。

**参考设计对照**（iam-auth，2026-09 评审）：借鉴其 key 管理规范、写入幂等、bootstrap、版本锁定、审计一致性问题意识，以及**嵌入式部署形态**（初评未采纳，2026-09-04 评审改定采纳：省一个部署单元，判定 API 由 authz 直接定义，代价见 §3"嵌入的代价"）；**未采纳** store-per-tenant 多租户（平台角色需单图）、gRPC 外部 API（全栈 HTTP/JSON 决策）、独立 console 产品形态（走 monorepo + 对内控制台微前端）。

**待定**：

1. 各业务服务的资源类型初始清单与 can_* 口径（随各服务迁移到 Go 栈时逐个注册，模板见 §4.2）。
2. platform_role 的初始清单（super_admin / operator / readonly 之外还需要哪些，管理界面上线前定）。

**回填 architecture.md 清单**（本设计合入后执行）：

- §3.1 选型总表：新增 authz（内嵌 OpenFGA 引擎）行
- §4.1：权限模型段落按本设计展开（authz 内嵌引擎、两级授权的数据来源写实）
- §4.2：网关规则表来源 = authz 导出
- §6.1：仓库结构新增 `services/authz`
- §7：部署拓扑标注 authz（内嵌引擎）×2 与 nemo_authz 库
- §11：待定项 2（权限模型）移入已定

---

## 12. 实施备注（2026-09-07 Phase A+B 落地回填）

> 实施设计见 `superpowers/specs/2026-09-06-authz-implementation-design.md`；本节只记与本文口径的偏差与遗留，不重复设计。**go-zero 落地（backend-design v2.2 目标态）对本文模型/API/失效语义无变更**——以下为落地中新增的护栏、契约回填与遗留项。

**落地护栏（超出本文原文的部分，均已落码并单测/curl 取证）：**

1. **WriteRelations 白名单护栏**（§6.2 写入语义的实现级收紧）：判定面写接口仅接受 owner/editor/viewer/sharer 四种共享语义关系，对象类型排除平台保留类型（user/org/platform_role），order 之外的业务类型须已声明——防终端用户经通用写接口自授管理面关系/平台角色（对含超管的全体调用者生效；管理面关系只能走既有管理 API）。
2. **资源类型声明白名单**（§6.3 的实现细化）：Actions 只能是 can_* 出口固定集的子集，类型名排除基础模型保留名，合并 DSL 由模板确定性渲染；资源级按钮 relation 在菜单写路径过白名单校验——声明配置不可能产出越权模型结构。

**契约回填（实施中经裁决补齐的接口缺口）：**

3. `GET /v1/admin/orgs/:id/members`（ListOrgMembers）为 Phase B Task 8 计划缺口回填（成员管理抽屉需回显存量成员；engine.ReadTuples 过滤 member 实现）——§6.1 API 面以此为准增补。
4. 审计查询 `AuditFilter` 的 action/时间范围参数**待回填**：repo.AuditFilter 结构已预留，管理 API/前端过滤条件未接线（Phase B Task 11 裁决为 Task 12 后视情况回填项）。

**平台遗留项（后续任务消化）：**

5. kit/errors 缺 `ServiceUnavailable` 语义构造器——authz logic 现以 `errors.New(503, ...)` 手写（6 处）， kit 收口时补构造器统一。
6. CAS 对接实测修正（§3.1 登录通道）：本地 CAS 测试容器为明文 HTTP 且仅 CAS 2.0 `/serviceValidate`，auth etc 已按探测修正；CAS 3.0 p3 路径留待真实 CAS 接入时再启。
7. authz 重启后引擎模型回落基础 `model.fga`（bootstrap 只装配基础模型，已注册资源类型的合并模型版本未在启动时重放）——模型版本管理扩展点（§6.3）落地时一并处理。
8. **callback 契约缺口（M6 联调实测）**：CAS 回调为 POST 端点，而 CAS 登录成功重定向固定 GET——纯浏览器自动链在最后一跳 405，随基座登录接 CAS + 接票页一并定契约（Phase C）。
9. **角色生效窗口（安全属性注记）**：平台角色写入 `/v1/auth/refresh` 签发的 claims（只读快照），角色撤销后已签发 token 内旧角色最迟 1h（TTL）失效——判定面实时以引擎元组为准，管理面/网关路由级（读 claims）按此窗口生效。

---

## 13. ABAC 属性策略（deny 拒绝 + allow 正向门槛，2026-09-15）

> 实现位于 `services/authz/internal/logic/abac.go`（评估）与 `repo/policy.go`（纯条件求值）；
> 管理面：authz-admin「资源属性 / ABAC 策略」两页。本节为该机制的设计口径。

### 13.1 定位与链路

属性策略是**叠加在授权结果之上**的属性约束，不替代任何授权：

```
RBAC（菜单码控入口）→ ReBAC（资源关系控归属）→ ABAC（属性策略附加约束）
```

- **deny（拒绝）**：条件命中即拒绝，优先于一切授权（含页面级 manage 码持有者）。
- **allow（正向门槛）**：对**适用**该策略的资源，须至少命中一条 allow 才放行——
  表达「只有 trade 业务线的人可看」这类正向属性门槛。属性不满足、乃至
  主体**没配该属性**，一律被挡在门槛外（缺属性 ≠ 放行——与 deny 的
  「缺属性不命中」构成有意的不对称：deny 要证据才拦，allow 要证据才放）。
- 求值顺序：先 deny（命中即拒，无视 allow 命中），再 allow 门槛（有适用策略且零命中 → 拒绝）。

### 13.2 适用规则

一条策略作用于某次判定，须满足：enabled，且 resourceType 为空或等于判定的资源类型。
allow 策略另有**资源侧适用性**：

- 字面量形式（无 resourceAttr）：对该资源类型**全部**资源生效（整型门槛，含管理码持有者）；
- resourceAttr 形式（如 `user.bizLine eq resource.allowedBizLine`）：仅对**设置了该属性**的
  资源生效——资源未声明该属性 = 未上门槛，不受约束。支持按资源粒度开门槛
  （如仅 topic:1 设 `allowedBizLine=trade` 时只有它受限）。

条件形式（无表达式引擎，KISS）：`subjectAttr op (resourceAttr | 字面量)`，
op 白名单 lt/lte/gt/gte/eq/neq（两侧均可转数值则数值比较，否则仅 eq/neq 字符串比较）。
effect 白名单 deny/allow（缺省 deny）；策略默认建为 disabled（防误拦），enabled 即回滚开关。

### 13.3 通用接入（后续业务域零接线）

评估入口唯一（`abacBlocked`），已接入：

1. **通用判定面 `/v1/authz/check`**：ReBAC 判定通过后自动叠加（对象按 `type:id`
   取 type 段）——后续业务域经 kit client 走通用 Check 即生效，**无需逐处接线**；
2. Topic 演示域：列表可见性过滤（快照批量：策略 + 主体属性每请求拉一次，逐资源只查
   资源属性）与 `topicCanEdit`（单点）。

注意：resourceType 留空的策略对任意对象生效，配置时注意作用域。

### 13.4 失效语义（fail-open，与 §9 的 fail-closed 区分）

属性/策略存储故障按放行处理（不拦截既有授权）——deny/allow 是叠加约束而非授权本身，
属性表故障不应误杀全部资源；引擎故障仍按 §9 fail-closed。配套缺省安全：无任何启用
策略时评估零开销直接放行（行为与无 ABAC 完全一致）。

> **使用指南**：配置操作层面的手册（含各配置入口、步骤与场景速查）见 `permission-guide.md`。
