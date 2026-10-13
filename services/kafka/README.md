# services/kafka（Kafka Topic 管理微服务）

独立可启动的 Kafka Topic 管理演示服务（2026-09-23 从 authz 拆出）：
- **业务数据自管**：topics / audits 表位于**独立业务库 `nemo_kafka`**（2026-09-23
  建库迁移拆分，不再复用 authz 的 nemo_authz_biz；"每个服务自己的库表"规范）；
  引擎库 `nemo_authz` 仅 authz 持有，授权元组仍由集中式 authz 管理。
- **权限集中判定**：不内嵌 OpenFGA 引擎，全部判定/名单经 `kit/authz` Remote 远程调用集中式 authz（:8084）——
  - `POST /v1/authz/check`（单点判定，含 ABAC 叠加）、`POST /v1/authz/list-objects`（列表反向展开）；
  - `GET/POST/DELETE /v1/authz/relations`（授权名单读/写/删，通用收敛接口）。
- **身份透传**：与 authz 共享 `Auth.AccessSecret` 解析平台 JWT；`authn.Identity` 中间件把 claims.sub 注入 ctx 并同步 `kit/authz.WithToken`，logic 调 authz 时自动带 Authorization 头。

## 目录

```
services/kafka/
├── kafka.go                # 服务入口（kit 接线：日志/错误出口/RequestID/AccessLog/Identity）
├── etc/kafka-api.yaml      # 默认配置（Port 8085、AccessSecret 与 authz 同、Mysql.Dsn=nemo_kafka、Authz.BaseURL）
├── cmd/provision/          # 建库 + 迁移 runner（nemo_kafka；MYSQL_DSN env 执行，幂等）
├── migrations/             # 000_provision.sql（建库）+ 001_biz.sql（topics/audits 表）
├── go.mod                  # module nemo/services/kafka
└── internal/
    ├── config/             # Config：RestConf + Auth + Mysql + Authz（kit/authz Config）
    ├── types/              # API 契约类型（与原 authz 内嵌 kafka 逐字一致）
    ├── authn/              # JWT 身份工具（Identity 中间件/IdentityFrom/TestToken）
    ├── repo/               # TopicRepo/AuditRepo：接口契约 + 内存实现 + MySQL 实现
    ├── svc/                # ServiceContext：Repo 装配 + kit/authz Remote + ReadyHandler
    ├── logic/              # 业务规则层（CRUD + 名单三件套 + reconcile 对账 + remoteretry 远程重试 + common）
    └── handler/            # handler + routes.go（/v1/kafka/* + reconcile + healthz/readyz）
```

## 使用方式

本地启动（必须在服务根目录，`-f` 相对 cwd 解析）：

```bash
cd services/kafka
# 首次（或库不存在时）建库 + 迁移：创建 nemo_kafka 并建 topics/audits 表
MYSQL_DSN="root:***@tcp(127.0.0.1:3306)/" go run ./cmd/provision
go build -o /tmp/kafka-api-new kafka.go
nohup /tmp/kafka-api-new > /tmp/kafka-api.log 2>&1 &
curl localhost:8085/healthz     # 存活探针 → ok
curl localhost:8085/readyz      # 就绪探针（MySQL 探活）
```

> 2026-09-23 拆分数据迁移（一次性）：nemo_authz_biz.topics 数据按原 id 复制进
> nemo_kafka.topics（引擎元组 kafka_topic:{id} 依赖 id 不变）；源表随后删除。

一键恢复（含 authz + kafka + 前端三件套 + IaC 声明下发）：`bash scripts/restore-dev.sh`。

## API（/v1/kafka/*，全部 JWT 鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /v1/kafka/topics | 列表（manage 码 List 全量；普通用户 ListObjects can_view 取可见 ID 下推 ListByIDs 查询，页内行经 kit/authz BatchCheck 一次批量复核，owner/can_edit/can_manage_grants 徽标批量取集合） |
| POST | /v1/kafka/topics | 新建（守卫：kafka-topics:create 码或 manage 码；写库 + owner/org 元组，远程写入带重试。空对象首写由 authz 创建引导放行（create_code 声明），普通用户可创建。可选 `orgId` 指定归属组织——该组织含子树成员可见，缺省回退创建人所在部门；org_id 列与 org 元组双写） |
| PUT | /v1/kafka/topics/:id | 更新（守卫：can_edit 或 manage 码） |
| DELETE | /v1/kafka/topics/:id | 删除（守卫：can_edit 或 manage 码；先删库后清元组，资源不存在幂等返成功） |
| POST | /v1/kafka/reconcile | 授权对账（守卫：需 manage 码；扫描补写缺失 owner，返回 {scanned, repaired}） |
| GET | /v1/kafka/topics/:id/grants | 名单（守卫：can_view 或 manage 码；viewer/editor，经 authz relations） |
| POST | /v1/kafka/topics/:id/grants | 授权（守卫：owner 或 manage 码，不含 editor；viewer/editor） |
| GET | /v1/kafka/topics/:id/consumers | 查看消费者（守卫：can_view 或 manage 码；demo 无集群返回空列表） |
| DELETE | /v1/kafka/topics/:id/grants/:targetType/:targetId | 撤权（守卫：owner 或 manage 码；幂等） |

> 前端 `apps/kafka` 的名单管理实际走通用组件（`@nemo/shared` ResourceGrantDialog → `/v1/authz/relations`），
> 本服务 grants 端点保留兼容、不再被前端调用。

## 前端联调

微应用 `console-frontend/apps/kafka`（端口 5275）：vite proxy 把 `/api/v1/kafka` 分流到 `http://localhost:8085`；
`/api/v1/admin`、`/api/v1/authz`、`/api/v1/me` 仍指 authz（:8084）。

## 判定语义（模型 v6，与集中式 authz 同口径）

- 可见性：`owner or viewer or editor`（can_view 派生；ABAC deny 在判定面叠加）。
- 编辑/删除：`can_edit`（owner or editor，editor 支持 `org#member` 组织主体——组织可按资源批量授可编辑）或页面级 `kafka-topics:manage`。
- 名单管理：`owner` 或 `kafka-topics:manage`（**不含 editor**，防提权）。Topic 列表页支持勾选多个 Topic **批量授予**（工具栏「批量权限配置」，页面级入口不进 menu-buttons 声明）：前端按行级 `canManageGrants` 过滤目标集合（无权行跳过并提示），一次 POST 跨 object 元组到通用 `/v1/authz/relations`（整批原子），后端零接口变更；批量撤销暂未提供（行内单对象撤销已覆盖）。
- 查看消费者等行级只读动作：复用 `can_view`（不新增关系）。
- 判定形态：**除列表外全部接口为路由声明式守卫**（`handler/guard.go`，经
  kit/authz RequireWith/RequireCode：资源级主判定 + `kafka-topics:manage`
  回退；grant/revoke 用 owner 关系守卫 `requireTopicManagePerm`，不含 editor，
  防提权）。
- 列表在 logic 内编排（守卫在请求边界拿不到“可见对象集合”），但全程批量、
  不逐条 Check：普通用户 ListObjects `can_view` 取可见 ID，下推
  `TopicRepo.ListByIDs`（WHERE id IN）只查可见行；页内行 can_view 复核经
  kit/authz `BatchCheck` 一次 RPC 完成（引擎逐行语义不变，网络开销从 N 次
  降为 1 次）；`owner`、`can_edit` 徽标各经一次 ListObjects 批量取集合
  （manage 持有者 can_edit 恒真）。判定调用约 4 次，与 Topic 总数、可见数
  解耦；结果缓存（Redis）属后续 P1。
- 行徽标（`canEdit`/`canDelete`/`canManageGrants`/`owner`）为当前用户视角的
  能力字段，与路由守卫语义同源（`canManageGrants` = owner + 管理码回退，
  与名单管理守卫 `requireTopicManagePerm` 一致），前端按钮显隐直接消费，
  不再从 owner 反推。
- delete 守卫化的取舍：判定先于存在性（与 PUT 同口径，关闭了"403=存在 /
  200=不存在"的存在性枚举通道）；代价是普通 owner 重试已删资源（owner 元组
  随删除被清）将得 403 而非幂等 200——管理码持有者仍幂等成功。
- 授权服务故障 503 fail-closed：守卫路由返回 `AUTHZ_SERVICE_UNAVAILABLE`
  （无权 403 `AUTHZ_DENIED`、路径 ID 非法 400 `AUTHZ_OBJECT_INVALID`）；
  logic 内联判定返回 `AUTHZ_ENGINE_ERROR`。

## 跨服务写的一致性防护

写业务库（nemo_kafka）与写授权元组（authz）是跨服务两步、无分布式事务，按三段防护避免孤儿/锁死：

- **创建**：先写库拿自增 ID，再写 owner 元组（远程 5xx/网络错误有限次重试）；重试耗尽审计标 failed，资源进入"待对账"。
- **删除**：先删库、后清元组（顺序反了且删库失败会把资源锁死）；资源不存在幂等返成功；清元组失败仅记录残留、不阻断。
- **对账**：`POST /v1/kafka/reconcile` 扫描全量 topic，缺失 owner 且 create_by 非空则批量补写；引擎故障 fail-closed 返 503。建议部署后及定时任务中调用。
