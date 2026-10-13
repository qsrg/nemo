# services/redis（Redis 实例管理微服务）

独立可启动的 Redis 实例管理演示服务（2026-09-30 新建，与 kafka 同模式拆分）：
- **业务数据自管**：clusters / instances / audits 表位于**独立业务库 `nemo_redis`**（"每个服务自己的库表"规范）；引擎库 `nemo_authz` 仅 authz 持有，授权元组仍由集中式 authz 管理。
- **集群分组（基础配置数据，2026-09-30 迭代）**：实例归属集群（`instances.cluster_id`，应用层保证一致性，禁外键）——前端「先选集群再出实例」级联；code 唯一且创建后不可改，删除非空集群 409 拒绝；存量实例由 002 迁移自动收纳进 `default` 默认集群。
- **集群双通道可见性（2026-09-30 迭代，集群已进权限引擎）**：菜单声明根行 `resourceType: redis_instance,redis_cluster`（逗号多值，菜单管理页或 008 seed 维护）派生注册 redis_cluster 类型（can_view 出口，注册表 manage_code=redis:manage——首个 owner 元组写入的判权依赖，**注册后需手工 UPDATE 设置**，注册表管理码暂无 IaC 声明源）；集群创建写 `user:{sub} owner redis_cluster:{id}` 元组（创建人即名单管理员）、删除先删库后清元组。普通用户可见集群 = **引擎授权（redis_cluster can_view——空集群也可直接授权）∪ 实例派生（可见实例归属的集群，向后兼容）**；manage 码持有者全量。页面入口：集群工具条「集群权限」（owner 或 manage 码可见）→ ResourceGrantDialog（redis_cluster + viewer 单关系）。语义自洽：用户新建实例即 owner，所属集群即时可见；普通用户直猜 clusterId 也只得到空页（实例可见性兜底）。
- **纯引擎可见性口径（无归属列）**：instances 表**不建 org_id 列**（与 kafka topics 的列谓词方案不同）——实例量级小（个位/百级），列表可见性直接用引擎 list-objects 返回的 can_view ID 集合做 `id IN (...)` 下推（集群维度与可见性谓词 AND 组合），无双写与漂移问题；量级涨到几万再考虑列谓词/游标分页（不为想象的规模提前优化）。
- **权限集中判定**：不内嵌 OpenFGA 引擎，全部判定/名单经 `kit/authz` Remote 远程调用集中式 authz（:8084）——`POST /v1/authz/check`、`POST /v1/authz/list-objects`、`POST /v1/authz/batch-check`、`GET/POST/DELETE /v1/authz/relations`。
- **身份透传**：与 authz 共享 `Auth.AccessSecret` 解析平台 JWT；`authzkit.Identity` 中间件把 claims.sub（裸用户名）注入 ctx，logic 取其作操作人（`user:{sub}` owner 元组）。
- **创建路径自举放行（2026-10-04，替换此前的服务账号方案）**：普通创建者对新对象无任何元组、也不持名单管理码，用户身份写 owner 元组曾被 authz manageGranted 拒（"先有 owner 才能写 owner"）。现由 authz 的**创建引导放行分支**解决：写 relation 为 owner/org 且对象无 owner 元组时，持有注册表 `create_code`（redis:create，006 迁移声明）或管理码的调用者放行——业务操作权由路由守卫验 create/manage 码，服务无需任何专用账号/凭证（用户 token 直透传）。预占（抢注未来 id 的 owner）由创建闭包**失败回滚**防御：元组写失败即删业务行，无半写、无越权（预占退化为可检测的创建失败）。
- **授权对账**：`POST /v1/redis/reconcile`（manage 码）扫描 instances+clusters 补写缺失 owner——创建元组写重试耗尽的最终兜底（重试 → 审计 failed → 对账补偿），历史半写数据跑一遍即修复。**审计 failed 的 target 不补**（回滚失败的脏行补 owner 会被洗白成有效资源）：对账读审计状态跳过并计入 `skipped`,交人工清理。

## 目录

```
services/redis/
├── redis.go                # 服务入口（kit 接线：日志/错误出口/RequestID/AccessLog/Identity）
├── etc/redis-api.yaml      # 默认配置（Port 8086、AccessSecret 与 authz 同、Mysql.Dsn=nemo_redis、Authz.BaseURL）
├── cmd/provision/          # 建库 + 迁移 runner（nemo_redis；MYSQL_DSN env 执行，幂等）
├── migrations/             # 000_provision.sql（建库）+ 001_biz.sql（instances/audits 表）
├── go.mod                  # module nemo/services/redis
└── internal/
    ├── config/             # Config：RestConf + Auth + Mysql + Authz（kit/authz Config）
    ├── types/              # API 契约类型（InstanceResp 含行级能力徽标字段）
    ├── repo/               # InstanceRepo/AuditRepo：接口契约 + 内存实现 + MySQL 实现
    ├── svc/                # ServiceContext：Repo 装配 + kit/authz Remote + ReadyHandler
    ├── logic/              # 业务规则层（CRUD + 纯引擎列表 + remoteretry 远程重试 + common）
    └── handler/            # handler + routes.go（/v1/redis/* 守卫路由 + healthz/readyz）
```

## 使用方式

本地启动（必须在服务根目录，`-f` 相对 cwd 解析）：

```bash
cd services/redis
# 首次（或库不存在时）建库 + 迁移：创建 nemo_redis 并建 instances/audits 表
MYSQL_DSN="root:***@tcp(127.0.0.1:3306)/" go run ./cmd/provision
go build -o /tmp/redis-api-new redis.go
nohup /tmp/redis-api-new > /tmp/redis-api.log 2>&1 &
curl localhost:8086/healthz     # 存活探针 → ok
curl localhost:8086/readyz      # 就绪探针（MySQL 探活）
```

一键恢复（含 authz + kafka + redis + 前端微应用 + IaC 声明下发）：`bash scripts/restore-dev.sh`。

## API（/v1/redis/*，全部 JWT 鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /v1/redis/clusters | 集群全量列表（读无守卫——页面可见即可读；选择器数据源，id 升序不分页；普通用户双通道过滤：引擎授权 ∪ 实例派生） |
| POST | /v1/redis/clusters | 新建集群（守卫：redis:manage 码；code 必填唯一 409 重码；写库 + owner 元组 `user:{sub} owner redis_cluster:{id}`（空对象首写由 authz 创建引导放行）） |
| DELETE | /v1/redis/clusters/:id | 删除集群（守卫：redis:manage 码；非空 409 拒绝——先迁走/删实例；先删库后清元组，清理失败记日志待对账） |
| POST | /v1/redis/reconcile | **授权对账**（守卫：redis:manage 码）：扫描 instances+clusters，对"库中存在但引擎缺失 owner 元组"的孤儿按 created_by 补写（创建路径元组写失败的最终兜底）；返回 {scanned, repaired} |
| GET | /v1/redis/instances | 列表（clusterId 必传——先选集群再出实例，与可见性谓词 AND 组合；manage 码全量；普通用户 ListObjects can_view 取可见 ID 下推 `Page{IDs}` 查询，页内行经 kit/authz BatchCheck 一次批量复核，owner/can_edit/can_manage_grants 徽标批量取集合） |
| POST | /v1/redis/instances | 新建（守卫：redis:create 动作码或 redis:manage 码；clusterId 必填且集群须存在；写库 + owner 元组 `user:{sub} owner redis_instance:{id}`（空对象首写由 authz 创建引导放行），带重试，**不写 org 元组**） |
| PUT | /v1/redis/instances/:id | 更新（守卫：资源关系 can_edit 或 redis:manage 回退） |
| DELETE | /v1/redis/instances/:id | 删除（守卫：资源关系 can_delete 或 redis:manage 回退；先删库后清元组，资源不存在幂等返成功） |
| GET | /healthz、/readyz | 存活/就绪探针 |

> 前端 `apps/redis` 的名单管理走通用组件（`@nemo/shared` ResourceGrantDialog → `/v1/authz/relations`），本服务不承载 grants 端点。

## 前端联调

微应用 `console-frontend/apps/redis`（端口 5277）：vite proxy 把 `/api/v1/redis` 分流到 `http://localhost:8086`；
`/api/v1/admin`、`/api/v1/authz`、`/api/v1/me` 指 authz（:8084），`/api/v1/users` 指用户服务（:8081）。

## 判定语义（与集中式 authz 固定模板同口径）

- **两层权限正交**：菜单可见性（`redis:read`，控制能否打开「实例管理」页）≠ 数据可见性（列表里看到哪些行）。看到菜单不会看到所有资源。
- 数据可见性：`can_view = member_view or owner`（引擎固定模板派生）；owner/viewer/editor 元组是权限真源，业务表无归属列参与过滤。
- 新建：创建人自动成为 owner（`user:{sub}` 元组）；要给更多人可见/可编辑，用行内「权限配置」授予 viewer/editor（组织主体 `org:{id}#member` 自动子树继承）。
- 编辑/删除：资源关系 `can_edit` / `can_delete`，或页面级 `redis:manage` 码回退（路由声明式守卫，`handler/guard.go` 经 kit/authz RequireWith）。
- 名单管理：`canManageGrants = manage 码 or owner`（**不含 editor**，防提权）；前端行内按钮显隐直接消费列表返回的行级能力字段（`canEdit`/`canDelete`/`canManageGrants`/`owner`）。列表页支持勾选多个实例**批量授予**（工具栏「批量权限配置」）：前端按行级 `canManageGrants` 过滤目标集合（无权行跳过并提示），一次 POST 跨 object 元组到通用 `/v1/authz/relations`（整批原子），后端零接口变更；批量撤销暂未提供（行内单对象撤销已覆盖）。
- 列表在 logic 内编排（守卫在请求边界拿不到"可见对象集合"），但全程批量、不逐条 Check：普通用户 ListObjects `can_view` 取可见 ID，下推 `InstanceRepo.Page`（WHERE id IN）只查可见行；页内行 can_view 复核经 kit/authz `BatchCheck` 一次 RPC 完成。谓词量级 O(用户自己的关系数)，与实例总数解耦。
- 授权服务故障 503 fail-closed：logic 内联判定返回 `REDIS_ENGINE_ERROR`（守卫路由返回 `AUTHZ_SERVICE_UNAVAILABLE`）。
- 与 kafka 的实现差异：行内按钮按行级 canX 显隐写死（未接 menu-buttons 声明接口，KISS）；无 reconcile 对账端点（owner 元组仅在创建时写一条，量级小，对账成本大于收益；引擎残留可经 `/v1/authz/relations` 手工清理）。

## 张三/李四差异化可见（验收演示）

脚本**不预置任何终端用户权限**——菜单码与资源元组两步配置全部经控制台完成（权限分层铁律，见 `scripts/restore-dev.sh` 头部注释）：

1. `bash scripts/restore-dev.sh` 拉起全链路（脚本只自举 admin 超管通配，另建 3 个演示实例 cache-order/cache-user/cache-activity；lisi/zhangsan 等演示账号无任何码，登录后看不到服务菜单——这正是预期初始态）。
2. admin 登录 authz-admin（:5276）→「人员授权」页：给张三、李四勾选「实例管理」菜单（redis:read；按需再勾 redis:create）。
3. admin 登录 console（:5273）→「实例管理」：对 cache-order 行内「权限配置」把张三（或其部门）授「可查看」；对 cache-user 把李四授「可编辑」。
4. 张三登录 → 实例管理：只见 cache-order（外加自己 owner 的实例）；李四：只见 cache-user，且行内出现编辑/权限配置按钮（editor）；admin 全量可见。
5. 验证点：张三/李四的列表 SQL 是 `WHERE id IN (可见ID)`，无任何归属列参与——可见性完全由引擎元组决定。

## 跨服务写的一致性防护

写业务库（nemo_redis）与写授权元组（authz）是跨服务两步、无分布式事务，审计先行（kit/audit pending → committed/failed 三态）+ 远程重试兜底：

- **创建**：先写库拿自增 ID，再写 owner 元组（远程 5xx/网络错误 3 次线性退避重试）；重试耗尽审计标 failed，实例进入"无 owner"状态（manage 码持有者仍可见可手工补授）。
- **删除**：先删库、后清元组（顺序反了且删库失败会把资源锁死）；资源不存在幂等返成功；清元组失败仅记录残留、不阻断。
- **更新**：纯业务库操作（覆盖 addr/description、落 update_by），不动元组。
