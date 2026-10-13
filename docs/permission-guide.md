# 权限配置使用指南

> 面向：平台管理员（配权限的人）与业务服务开发者（接入权限的服务方）。
> 定位：**怎么配、在哪配、配完会发生什么**——操作手册；模型与链路设计见 `permission-design.md`。
> 更新：2026-09-16（方案 B：资源驱动菜单——资源级授权自动解锁菜单入口，见 §0/§1/§4；菜单管理页支持配置 resource_type，见 §1/§8）。

---

## 0. 权限分几层？各在哪配？

一次访问要过三层，**每层独立配置、逐层叠加**：

| 层 | 回答的问题 | 配置入口 | 配置对象 |
|---|---|---|---|
| ① 菜单级（RBAC） | 能不能进这个页面、能不能在页面里做写操作 | 授权中心 → **人员授权** | 用户/部门 × 菜单（只读/管理） |
| ② 资源级（ReBAC） | 这个具体资源（如某个 Topic）归谁、谁能看/能改 | 业务页面行内「授权对象」按钮 | 用户/部门 × 单个资源（viewer/editor） |
| ③ 属性策略（ABAC） | 属性上还有没有附加门槛（安全等级、业务线） | 授权中心 → **资源属性 / ABAC 策略** | 属性 + 策略（deny 拒绝 / allow 门槛） |

支撑配置（管理员低频维护）：

| 配置 | 入口 | 作用 |
|---|---|---|
| 部门与成员 | 部门与成员 | 只读视图——组织事实源在 CAS，本页只能「立即同步」 |
| 菜单管理 | 菜单管理 | 菜单注册表 = 权限码目录：新服务的页面/权限码在此登记；**声明 `resource_type` 的菜单 = 资源驱动**（见下） |
| 资源类型 | （已下线） | 类型不再独立注册——由菜单/按钮声明驱动派生（§3）；只读列表供「资源共享」页聚合展示 |

**判定顺序**：菜单入口（菜单码 **或** 资源级，见下）→ 资源关系控归属 → ABAC 叠加约束（deny 命中即拒，allow 门槛零命中即拒）。

> **方案 B：资源驱动菜单（2026-09-16）**——菜单可见性 = 该菜单的 `read` 码被授予 **或** 用户对该菜单声明 `resource_type` 下的**任一资源**有查看/编辑/管理关系（`can_view` 级）。也就是说：**给用户授了某个 Topic 的查看/编辑权，Kafka 的「Topic 管理」菜单就会自动出现**，无需再单独授菜单只读码。页面内数据仍按资源级过滤（能看到菜单 ≠ 能看到全部资源）。未声明 `resource_type` 的菜单（如 redis 实例管理）保持纯菜单码语义。

---

## 1. 菜单级授权：人员授权页

**任务流**：选人（或部门）→ 看「当前有效权限」→ 在矩阵里拨档位 → 保存即生效。

- **档位三选**：无 / 只读 / 管理。管理包含只读；「管理」控制页面内写操作（新建、删除等）。
- **给用户**：直接授予该用户。
- **给部门**：授权对**该部门子树全部成员**（直属 + 所有子孙部门）生效——给父部门授权，子部门的人都有；给叶子部门授权，不影响兄弟和祖先部门。
- **通配**：「全部菜单·只读/管理」一档搞定批量授权（矩阵顶部）。
- **继承锁定**：经部门继承来的权限在矩阵里显示为灰色锁定态，悬停可见来源部门；要调整去「部门」目标下改，不能在用户身上取消。
- **保存语义**：与当前直接授予做差集，新增 grant / 移除 revoke，幂等，即时生效（`/me/permissions` 实时查询）。

**排查工具**：「模拟用户视角」预览某人登录后看到什么；每行「授权对象」反向查某菜单授给了谁。

> **资源驱动菜单**：声明了 `resource_type` 的菜单（如 Kafka → Topic 管理 → `kafka_topic`），其只读档位是"资源授权后自动解锁"的——给用户授了该类型下任一资源的 viewer/editor，菜单入口即出现，**不必在矩阵里单独勾只读**。矩阵里的只读/管理档位仍适用于：纯菜单（无 `resource_type`）与"需要只读码但无具体资源可授"的批量场景（如通配「全部菜单·只读」）。
>
> **按钮级动作码（2026-09-28 新增）**：菜单管理页的「附加动作码」字段（或 `migrations/008_menu_seed.sql` 基线 seed 的 `action_codes` 列）登记页面的按钮级功能码（如 `kafka-topics:export`）——登记后**人员授权矩阵**该菜单行出现动作勾选框，授给谁谁能用对应按钮（部门授予、继承锁定、通配与 read/manage 同语义）；**改目录会自动清理被移除动作码的存量授予**（防孤儿）。业务侧：前端 `has('kafka-topics:export')` 显隐按钮，后端路由挂 `authz.RequireCode` 守卫——资源作用范围（如"只能重置自己可编辑的 Topic"）由 logic 层叠加资源级判定（动作权 × 对象权 AND）。

> **配置入口（2026-09-16 新增，2026-09-18 改为声明派生）**：菜单管理页的新增/编辑对话框支持「资源类型」「资源关系」字段——接入方声明 `resource_type`/`relation` 无需 SQL/seed，页面操作即可（类型由声明派生，无需预注册；格式：小写字母开头的字母/数字/下划线，最长 64，空 = 纯菜单；页面级按钮不可填资源类型）。保存后该菜单即成为资源驱动菜单，人员授权页对应行显示「资源授权解锁」标签。

> 已知边界：**停用或删除菜单时，其权限码（只读 + 管理）名下已存在的授予会自动一并清理**（仍被其他启用菜单共享的码不误清），清理与菜单变更在同一审计单元内完成——重新启用后需重新授权。

---

## 2. 部门与成员

- 组织架构（部门树、成员归属）**事实源在 CAS**，平台侧只读；「立即同步」拉取 CAS 最新架构。
- **继承语义（重要）**：成员关系沿子树向上聚合——给部门 D 授权，D 及其全部子孙部门的成员都获得；反之，用户的有效权限 = 直接授予 + 所在各级部门的授予。
- 在人员授权页选「部门」目标时，总览显示本部门直接授予 + 沿父链各上级部门的授予，方便核对"这个人为什么有这个权限"。

---

## 3. 资源类型声明（写菜单即声明类型）

新业务域要有自己的资源（如 `kafka_topic`），**在菜单声明里带上资源维度即可，无需预注册**：

**方式（唯一）菜单/按钮声明 + 自动派生**：在**菜单管理页**声明（新环境基线 `migrations/008_menu_seed.sql` 由 `scripts/init-env.sh` 初始化下发；2026-10-11 起 `deploy/menus.yaml` + `apply-config.py` 已下线，运行时事实源是 authz 的 menus 表，管理面 CRUD 维护）：
- 菜单行带 `resourceType: <类型名>` ⇒ 该类型隐含 `can_view`（页面入口 = 列表可见性）；
- 资源级按钮行 `relation: <can_*出口> + resourceType: <类型名>` ⇒ 贡献该出口。

类型 actions = 全部声明的并集，按引擎白名单顺序渲染。菜单写路径（IaC 下发或管理面操作）落库后由 authz 自动同步注册表与引擎模型（`logic/resourcetypesync.go`）：

```yaml
menus:
  - {serviceKey: report, title: 报表中心, path: /report, resourceType: report}
  - {serviceKey: report:edit, type: button, parentKey: report, title: 编辑, relation: can_edit, resourceType: report}
```

上例派生 `report{can_view, can_edit}`——**无需在别处注册类型**。管理面等价操作：菜单管理 → 新建菜单/按钮时填"资源类型"与"资源关系"字段。

> 说明：`deploy/resource-types.yaml` 与独立注册/编辑/删除 API 已下线（2026-09-18 收敛）；「资源共享」页的类型下拉来自注册表只读列表（GET /v1/admin/resource-types），展示派生结果。

- 模型按模板确定性生成（配置驱动，**不做自由 DSL 编辑**）——声明 `can_view` 隐含组织成员可见（member_view），声明 `can_share` 隐含共享转授（sharer），`can_edit ⇒ editor`、`can_delete ⇒ owner or editor`（2026-09-23：可编辑者即可删除，两档语义；`editor`/`viewer` 均支持 `org#member` 组织主体，组织可按资源批量授可编辑，支撑跨团队隔离）。
- 声明即时生效（菜单写路径自动 sync）；未声明类型调用判定统一 fail-closed（403）。
- 类型名不得与保留类型（user / org / platform_role / order）重名；基础模型其余已占用类型（permission 等）声明同样会重复定义被引擎拒绝。
- 页面级按钮（`relation` 空）**不得**声明资源类型；资源级按钮必须同时声明 `resourceType` 与白名单内 `relation`（can_view/can_edit/can_delete/can_share），配置错误在菜单写路径 400 拒绝。
- 存量注册表保留：被菜单/按钮引用的类型自动派生更新，未被引用的存量类型不自动删除；类型名不可在注册表改名（改名 = 改菜单声明 + 迁移存量授权）——**上线前想好类型名**，取名用业务域明确词（如 `kafka_topic`，勿用泛词 `topic`）。
- **名单管理码**（可空，`{base}:manage` 形态）：持有者对该类型**任意**资源拥有授权名单的查看/写/撤销权——业务页挂上 ResourceGrantDialog 后整个授权名单管理就通了，**新类型零 authz 改码**。留空 = 不设全权码，名单只能由各资源的 owner 管理（**不含 editor**，防提权）。管理码由注册表 `ManageCode` 声明驱动，经管理面配置接口配置（2026-10-11 新增 `PUT /v1/admin/resource-types/{name}`，替代手改库；菜单派生 sync 保留该值不被覆盖）。

## 4. 资源级授权：单个资源的「谁可看 / 谁可改」

**管理员/资源负责人在业务页面操作**（不在授权中心）：资源列表行内点「权限」（如 Topic 管理页每行的授权按钮），弹出的就是通用授权对话框：

- 名单 = viewer（可看）/ editor（可改）两个关系，支持**用户与部门**（部门含子树成员）；
- 添加 / 移除即时生效，移除幂等；
- 管理权由后端按资源类型校验（如 kafka_topic → owner 或页面 manage 码，**不含 editor**，防提权：editor 仅能编辑属性不能改名单）；无管理权时只读查看名单。
- **授权即解锁入口（方案 B）**：被授权用户获得该资源的 `can_view` 关系后，该资源所在菜单（声明了对应 `resource_type`）自动出现在其侧边栏——**给用户授一个 Topic 的查看权，他登录就能看到「Kafka → Topic 管理」并只看到被授权的 Topic**，无需在人员授权页另行勾菜单只读码。

**业务服务接入（开发者）**：该对话框是基础能力层通用组件 `packages/shared/src/ResourceGrantDialog.vue`，新业务页接入只需传 `resourceType + resourceId + resourceName + relations`（如 `['viewer','editor']`）与 `manageable`，组件内部封装名单拉取/添加/移除。后端走通用 `/v1/authz/relations` 读写（须已注册类型，见 §3）。

> **创建时直接指定可见组织（2026-09-30）**：新建 Topic 弹窗提供「可见组织」下拉（数据源 `GET /v1/me/orgs`，登录即可读）——选定组织后该组织**含子树成员**立即可见该 Topic（归属组织 `org_id` 列 + org 元组双写）；不选则回退创建人所在部门（存量行为）。编辑态不支持改归属；要给更多组织/个人开可见，用行内「权限」对话框加 viewer。

---

## 5. ABAC：属性策略（deny 拒绝 / allow 正向门槛）

**ABAC 配置全部是通用的**：不绑定任何资源类型——策略用 `resourceType` 字段指定作用域，评估挂在通用判定面（`/v1/authz/check`），**新业务域接入后零接线自动生效**。

### 5.1 两种效果

| 效果 | 语义 | 典型场景 |
|---|---|---|
| **deny（拒绝）** | 条件命中即拒绝，优先于一切授权（含管理码持有者） | 安全等级不足的人不能看机密资源 |
| **allow（门槛）** | 适用的资源须**至少命中一条**才放行；属性不满足、乃至没配属性，一律被挡 | 「只有 trade 业务线的人可看」 |

求值顺序：先 deny（命中即拒，无视 allow 命中），再 allow 门槛（有适用策略且零命中 → 拒）。

### 5.2 配置步骤（以「只有 trade 业务线的人可看 topic」为例）

1. **配主体属性**：授权中心 → 资源属性 → 主体属性 tab → 输入 `user:zhangsan` → 查询 → 新增属性 `bizLine = trade`（每个相关用户都配）。
2. **建策略**：ABAC 策略 → 新建：
   - 策略名：`仅 trade 可见`
   - 效果：**门槛（allow）**
   - 资源类型：`topic`
   - 主体属性：`bizLine`，操作符：`eq`，字面量：`trade`
3. **先建后开**：新建默认停用，确认无误后行内开关启用，即刻参与鉴权。**开关即回滚开关**。

### 5.3 门槛的两种粒度

- **字面量形式**（比较侧填「字面量」）：对**整个资源类型**生效——上例所有 topic 都只允许 trade 业务线。
- **资源属性形式**（比较侧填「资源属性」）：只对**设置了该属性的资源**生效。例：建策略 `bizLine eq resource.allowedBizLine`，然后只给 topic:1 配 `allowedBizLine=trade`——只有 topic:1 受限，其他 topic 照常。

资源属性在「资源属性 → 资源属性 tab」配，标识形如 `topic:1`。

### 5.4 deny 用法对照

安全等级门槛（低等级用户拒绝访问高等级资源）：

- 资源 `topic:1` 配 `securityLevel = 4`；用户配 `securityLevel = 2`；
- 策略：效果**拒绝（deny）**，条件 `user.securityLevel lt resource.securityLevel`。

注意不对称性：**deny 缺属性 = 放行**（要证据才拦），**allow 缺属性 = 挡下**（要证据才放）——正向门槛场景选 allow，不要用 deny+neq 凑（那样没配属性的人会漏进来）。

### 5.5 边界与注意

- `resourceType` **留空 = 对所有资源类型生效**（含任何经通用 Check 判定的对象），配置时注意作用域。
- 属性/策略存储故障按放行处理（fail-open——ABAC 是叠加约束不是授权本身）；引擎故障仍拒绝（fail-closed）。
- 关闭全部策略 = 回到纯菜单码 + 资源关系模型，行为完全一致。
- 操作符白名单：lt / lte / gt / gte / eq / neq；两侧都能转数值则数值比较，否则仅 eq/neq 按字符串比较。

---

## 6. 场景速查

| 想实现 | 去哪配 | 怎么配 |
|---|---|---|
| 某人能进某页面 | 人员授权 | 选人 → 该菜单拨「只读」 |
| 某人能在页面里增删改 | 人员授权 | 该菜单拨「管理」 |
| 整个部门用一个页面 | 人员授权 | 选部门 → 拨档位（子树全部成员生效） |
| 新人入职一把梭 | 人员授权 | 「全部菜单·只读」通配，或按部门授权 |
| 某个资源只给特定人/部门看 | 业务页该资源行 →「权限」 | 加 viewer 名单 |
| 资源交给别人维护 | 业务页该资源行 →「权限」 | 加 editor 名单 |
| 某资源有安全等级门槛 | 资源属性 + ABAC 策略 | 属性 securityLevel + deny 策略（§5.4） |
| 某类资源只允许某业务线 | 资源属性 + ABAC 策略 | 属性 bizLine + allow 门槛（§5.2） |
| 新业务域接入权限 | 菜单管理 + 业务页组件 | 菜单/按钮声明（§3，类型随之派生）→ 行内挂 ResourceGrantDialog（§4） |
| 查某人为什么有某权限 | 人员授权 | 总览区看来源（直接/经部门）+ 模拟用户视角 |

## 7. 生效时效

| 变更 | 生效时间 |
|---|---|
| 菜单码授予/撤销（人/部门） | 即时（`/me/permissions` 实时查询） |
| 资源 viewer/editor 名单 | 即时 |
| ABAC 属性、策略及开关 | 即时 |
| 组织架构（CAS 侧变更） | 下次同步后（部门与成员页可手动触发） |
| 菜单注册表变更 | 基座下次拉取菜单或重新登录后 |

---

# 第二部分：业务服务接入权限（开发者指南）

> 面向：要把自己服务的页面/资源接入权限体系的业务开发者。
> **接口全部是通用的**：判定、名单、ABAC 叠加都不绑定具体资源类型——任何服务接入走同一组接口、同一套组件，新资源类型只需在菜单/按钮声明（§3），无需 authz 侧改代码。
> 参考实现：Kafka Topic 演示域（2026-09-23 起独立为 `services/kafka` 微服务 + `console-frontend/apps/kafka/src/views/TopicManage.vue`，判定经 kit/authz 远程调用集中式 authz；早期内嵌实现见 git 历史 `services/authz/internal/logic/kafkatopic*.go`）。

## 8. 接入总览：四步

```
① 声明资源类型（菜单管理页/008 seed 的菜单/按钮行带 resourceType/relation，见 §3）—— 类型随声明派生，引擎自动认识 report 这类资源
② 声明菜单 + 权限码（菜单管理页/008 seed 加行，见 §2/§3）   —— 让页面有 read/manage 码可授
   ①②随提交进版本库（基线 seed），新环境经 scripts/init-env.sh 初始化（authz provision 执行 008_menu_seed.sql）；日常菜单调整在菜单管理页操作（库持久化）
③ 后端判定（kit/authz 或直接调通用接口）       —— 列表过滤 + 单点判定
④ 前端接组件（@nemo/shared）                   —— usePermission + ResourceGrantDialog
```

- ② 若菜单要**资源驱动**（授资源即自动解锁入口，无需单独授只读），在菜单管理页（或 008 seed）对应菜单行填 `resourceType`（如 `report`，须已在①声明）；菜单管理页对话框也可临时操作。
- 资源级授权（④ 的 ResourceGrantDialog）不是必选：纯展示菜单不声明资源类型，走菜单码即可。

ABAC（属性策略）**零接入**：评估挂在通用判定面上（见 §8.1），管理员配置策略即对你的资源生效。

## 8.1 后端：通用接口（所有服务共用）

### 判定面 `/v1/authz/*`（用户身份取 JWT，resourceId 一律字符串）

| 接口 | 用途 |
|---|---|
| `GET /api/v1/authz/check?object={type}:{id}&relation=can_view` | 单点判定：这个资源我能不能 view/edit。**ABAC 自动叠加** |
| `GET /api/v1/authz/list-objects?relation=can_view&type={type}` | 反向展开：当前用户可见的全部资源 ID（列表页过滤用） |
| `GET /api/v1/authz/relations?object={type}:{id}&relation=viewer` | 读某资源的授权名单（ResourceGrantDialog 内部用） |
| `POST /api/v1/authz/relations` | 批量写授权元组（body `{tuples: [{user, relation, object}]}`，幂等） |
| `DELETE /api/v1/authz/relations` | 撤销单条授权（object/user/relation 精确定位，幂等） |

约束（通用护栏，对全体调用者生效）：
- relation 仅 owner/editor/viewer/sharer；对象类型排除保留类型（user/org/platform_role），**未声明的类型拒绝**（先做第①步菜单/按钮声明）；
- 列表页**禁止逐行 Check**（一页 50 行 = 50 次判定）：list-objects 一次拿可见 ID，**下推成查询条件**（如 SQL `WHERE id IN`，repo 提供 `ListByIDs`），而非全量捞库后内存过滤；owner/can_edit 等行内徽标也各用一次 list-objects 批量取集合；
- 引擎故障 fail-closed（503），"明确无权"是 200 `{allowed:false}` 不是错误。

### 自助面 `/v1/me/*`

| 接口 | 用途 |
|---|---|
| `GET /api/v1/me/permissions` | 当前用户权限码集合（菜单 read/manage） |
| `GET /api/v1/me/menus` | 当前用户可见菜单（服务端已按权限过滤） |

### Go 服务：kit/authz 接入件

```go
import authzkit "nemo/kit/authz"

// svc 构造期装配一次（remote 实现经 JWT 透传调判定面）：
checker := authzkit.New(authzkit.Config{BaseURL: "http://authz:8888", Timeout: 5 * time.Second})

// 行级接口推荐：路由声明式守卫（routes.go 挂一行，logic 零权限代码）——
// 对象 ID 取路径参数 :id（须为正整数），主判定 + 管理码回退；无权 403 /
// 判定服务故障 503 / ID 非法 400。参考 services/kafka/internal/handler/guard.go。
Handler: authzkit.RequireWith(checker, authzkit.Options{
	Perm:          "report:can_edit",
	ObjectID:      reportIDFromPath,          // 提取器：从 :id 路径参数取并校验
	FallbackCodes: []string{"report-tools:manage"}, // 页面级管理码"管理员全权"
})(handler.UpdateReportHandler(ctx))

// 新建等无实例 ID 的集合入口：纯权限码守卫（须持管理码才可建资源）：
Handler: authzkit.RequireCode(checker, "report-tools:manage")(handler.CreateReportHandler(ctx))

// 无法中间件化的场景（列表过滤、对象 ID 来自 body、多步语义）才在 logic 内判定：
ok, err := authzkit.CheckObject(ctx, checker, "report", "r1", "can_edit")
if err != nil { /* 503 fail-closed：拒绝 */ }
if !ok { /* 403 AUTHZ_DENIED */ }

// 列表过滤（批量、与行数无关；错误均 fail-closed，此处省略）：
manage, _ := authzkit.CodeGranted(ctx, checker, "report-tools:manage")
viewObjs, _ := checker.ListObjects(ctx, "report", "can_view")   // 可见对象
rows, _ := reportRepo.ListByIDs(ctx, parseObjectIDs(viewObjs))  // ID 下推查询，非全量
ownerSet, _ := checker.ListObjects(ctx, "report", "owner")      // 徽标集合
editSet, _ := checker.ListObjects(ctx, "report", "can_edit")
// 组装每行：canEdit = manage || editSet[obj]；owner = ownerSet[obj]
```

写路径（资源创建/删除等改权限拓扑的操作）统一走 kit/audit 的审计先行模板
`audit.WriteWithAudit`（pending → write → committed/failed 三态 + fail-closed 错误分类），
服务 repo 经 `Sink` 三方法适配接入，见 §8.4 与 kit/README。

### 8.1.1 列表分页：列谓词下推 + 引擎逐行复核（几十万量级）

列表页的可见性过滤有两条路线，按数据量选择：

- **资源千级以内**：`list-objects(can_view)` 一次取回可见集合，把 ID 集合下推进 SQL（`WHERE id IN (...)`）——简单，但可见集合随资源总数膨胀，集合上万后失效；
- **资源万级以上**（Topic 域已切换）：**授权维度落业务列**——归属部门 `org_id` 列 + 创建人列，列表查询变列谓词分页：

```sql
SELECT * FROM topics
WHERE org_id IN (:用户部门)      -- 部门归属（org 元组的列投影）
   OR id IN (:直接授予对象)      -- owner/viewer/editor 元组并集（用户维度小集合）
ORDER BY id LIMIT 20 OFFSET :n;
```

两路谓词都是**用户维度的小集合**（他的部门几个、直接授予几十个），规模不随资源总数增长；各吃 `idx_org` / 主键索引，几十万行毫秒级出页。**页内行再经引擎逐行 Check(can_view) 复核**——引擎是权限真源（ABAC deny/allow 自动叠加），列漂移被拦在出口：列多显示的行被复核滤掉（不泄漏），列漏显示的只是少看一行（不影响安全）。列是加速副本，允许最终一致（创建时与元组双写，失败告警）。

业务接入要点：资源表加 `org_id` 列 + 索引，创建时自动挂创建人部门（或表单显式选择）并双写 org 元组；列表走谓词分页 + 页内复核；`total` 为谓词口径的近似值（ABAC 门槛启用时页内可能少于 pageSize）。参考实现：`services/kafka`（repo.Page + listkafkatopicslogic）。

## 8.2 前端：权限码判定（页面/按钮显隐）

`@nemo/shared`（`packages/shared`）统一入口，微应用直接 import：

```ts
import { createMicroHttp, useApiError, usePermission } from '@nemo/shared'

// 1) API 实例：createMicroHttp 自动带 token、等基座会话就绪、401 上报基座统一登出
const http = createMicroHttp() // baseURL 默认 /api/v1

// 2) 权限码判定（实时跟随基座 session，授权变更自动刷新）
const { canManage: hasManageCode } = usePermission()
const canManage = computed(() => hasManageCode('report-tools')) // ≡ report-tools:manage

// 3) 统一错误提示（403 → 无权限话术，其余透出后端 message）
const handleApiError = useApiError()
```

模板里：`v-if="canManage"` 控制新建/删除按钮。**注意**：`canManage/canRead` 是函数，须包 `computed` 再给模板用（裸函数恒 truthy）。

基座侧统一兜底（微应用外的可见性防线，业务前端无需自己实现）：左侧菜单按
`/v1/me/menus` 服务端过滤；工作台态势卡与域态势页同样只聚合可见服务；菜单外
服务的直达 URL 一律被路由守卫重定向回工作台（前端 fail-closed 显隐；资源级
安全仍由服务端 Check 把守，前端显隐不是安全边界）。

## 8.3 前端：资源授权组件 ResourceGrantDialog（"谁可看/谁可改"）

**不用自己写**——通用组件已在 `@nemo/shared`，传资源标识即可（内部封装名单拉取 / 添加 / 移除，调 §8.1 的 relations 三接口）：

```vue
<script setup lang="ts">
import { computed, ref } from 'vue'
import { ResourceGrantDialog, usePermission } from '@nemo/shared'

const { canManage: hasManageCode } = usePermission()
const canManage = computed(() => hasManageCode('report-tools'))

const grantVisible = ref(false)
const grantId = ref('')
const grantName = ref('')
</script>

<template>
  <!-- 列表行内入口 -->
  <el-button link type="primary" @click="grantId = row.id; grantName = row.name; grantVisible = true">
    权限
  </el-button>

  <!-- 通用授权对话框：支持用户/部门（部门含子树成员），变更即时生效 -->
  <ResourceGrantDialog
    v-model="grantVisible"
    resource-type="report"          <!-- 须与①声明的类型名一致（菜单/按钮声明派生） -->
    :resource-id="grantId"
    :resource-name="grantName"
    :relations="['viewer', 'editor']" <!-- 名单关系集，按需选 -->
    :manageable="canManage"          <!-- false = 只读查看名单 -->
    :title="`${canManage ? '权限配置' : '权限查看'}：${grantName}`"
  />
</template>
```

后端对 relations 写有管理权校验（如 topic → owner/editor/manage 码），前端 `manageable` 只管显隐，**越权写由服务端拦**——两边门槛一致，不必自己写校验。

> **名单策略（类型声明驱动，零改码）**：通用 relations 接口的类型级校验由**类型声明**决定（`logic/grantpolicy.go`），新资源类型经菜单/按钮声明派生后即生效，不需要 authz 侧改任何代码：
> - **写/撤销名单**（POST/DELETE relations）：名单管理码持有者 or 该资源 owner（**不含 editor**——可编辑属性不放大为名单管理权，防 editor 自授/改授 owner 提权）；放行后叠加 ABAC 属性策略（deny 命中即拒绝）；
> - **查看名单**（GET relations）：名单管理码持有者 or 该资源 can_view 持有者（资源驱动菜单/菜单行声明了 can_view——未勾选的机密类型只有管理码可看名单）；
> - **资源创建归属**（首条 owner 元组）：由名单管理码/创建码持有人写入（资源尚无关系，owner/editor 路径不存在）；服务后端代写的身份凭证机制为后续扩展项。

## 8.4 后端：资源创建时落归属元组

资源创建（/归属变更）后，必须把 owner 关系写入判定面——**漏写 = 创建人自己都看不到、资源无人可操作**。写库与写元组是跨服务两步、无本地事务，按 topic 域的三段防护落地（见 services/kafka；写路径统一包在 kit/audit 的 `WriteWithAudit` 审计先行模板里，服务 repo 经 Sink 适配）：

1. **写入带重试**：owner 元组写入对 5xx/网络类错误有限次重试（4xx 不重试），见 `remoteretry.go`；
2. **审计兜底**：重试仍失败，审计标记 failed（资源已建、owner 缺失），不静默；
3. **对账补偿**：提供 reconcile 接口扫描业务表，对缺失 owner 的资源按 `create_by` 批量补写。

```go
// 创建 report:r1，归属创建人（当前 JWT 用户）：
// POST /v1/authz/relations
// {"tuples":[{"user":"user:zhangsan","relation":"owner","object":"report:r1"}]}
```

资源删除时**先删业务库、后清元组**（顺序不能反：若先清完 owner 元组而删库失败，资源还在却谁都管不了 = 锁死）；删除接口对"资源不存在"幂等返成功，清元组失败仅记录残留、不阻断（资源已删，业务目标已达成），残留由 reconcile/清理任务兜底。参考 `DeleteKafkaTopicLogic`。

对账接口范例（kafka）：`POST /v1/kafka/reconcile`（须持有页面 manage 码），返回 `{scanned, repaired}`——扫描全部资源、补写缺失 owner；引擎故障时 fail-closed 返回 503，不做半吊子对账。建议部署后与定时任务中调用。

## 8.5 ABAC：管理员配置即生效，服务零改动

deny/allow 策略的评估挂在 `GET /v1/authz/check` 内部（对象按 `type:id` 前缀识别资源类型）——你的服务只要走 §8.1 的判定接口，管理员配置的安全等级/业务线门槛（第一部分 §5）就自动作用于你的资源。**接入方唯一要注意**：`list-objects` 不含 ABAC 叠加，若你的域启用 allow 门槛，列表页需对可见集合逐个 check 复核（或按 topic 列表的做法：可见性判定后调判定面的 check）。

## 8.6 接入 checklist

| # | 事项 | 入口 |
|---|---|---|
| 1 | 声明资源类型与资源级按钮（菜单管理页/008 seed 的菜单/按钮行带 resourceType + relation；页面级按钮留空） | 菜单管理页 / `migrations/008_menu_seed.sql`（§3） |
| 2 | 登记菜单行（serviceKey + permissionCode，read 码服务段连字符如 `report-tools:read`）；**资源驱动菜单**同时填「资源类型」（声明即派生，无需预注册） | 授权中心 → 菜单管理 |
| 3 | 后端判定：行级接口优先路由守卫（kit/authz `RequireWith`），列表用 list-objects 过滤，其余场景 logic 内 kit/authz 判定 | §8.1 |
| 4 | 资源创建/删除同步 owner 元组（写路径包 kit/audit `WriteWithAudit`，含重试/审计/对账三段防护） | §8.4 |
| 5 | 前端 usePermission 控显隐 + createMicroHttp 发请求 | §8.2 |
| 6 | 需要行内授权管理：挂 ResourceGrantDialog | §8.3 |
| 7 | （管理员）给目标用户/部门授菜单码 | 第一部分 §1 |
