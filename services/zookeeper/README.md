# services/zookeeper（ZK 集群注册与节点浏览微服务）

独立可启动的 Zookeeper 服务（2026-10-03 新建，与 redis 同模式拆分）：集群注册表
自管，**"看到集群才能浏览节点"**——集群（zk_cluster）进权限引擎，节点浏览
接口按集群 can_view 守卫，与集群列表同一判定口径。

- **业务数据自管**：clusters 表位于独立业务库 `nemo_zookeeper`（"每个服务自己
  的库表"规范）；引擎库 `nemo_authz` 仅 authz 持有，授权元组由集中式 authz 管理。
- **集群级授权语义**：授集群 viewer 即全量节点只读浏览（znode 不建元组——节点
  海量且层级化，集群是唯一授权边界）；无实例派生通道（无业务子实体），可见性
  纯引擎口径（manage 码全量 / 普通用户 can_view 过滤）。
- **权限集中判定**：判定/名单经 kit/authz Remote 远程调用集中式 authz（:8084）。
- **服务与用户身份分离**：创建集群的 owner 元组走服务账号（svc-zookeeper，持
  zookeeper:manage 码；`Authz.Service` 配置段 + kit/authz `ServiceTokenFn`），
  业务操作权由路由守卫（manage 码）把关——同 redis/kafka 创建口径。
- **阶段一边界**：znode 数据为确定性 mock 演示树（不连真实 ZK）；集群表已预留
  `hosts` 列，阶段二引入 go-zookeeper/zk 按 hosts 连接替换 mock 实现——接口
  签名、守卫、前端交互均不变。只读浏览（无 znode 写操作）。

## 目录

```
services/zookeeper/
├── zookeeper.go            # 服务入口（kit 接线：日志/错误出口/RequestID/AccessLog/Identity）
├── etc/zookeeper-api.yaml  # 默认配置（Port 8087、Mysql.Dsn=nemo_zookeeper、Authz.Service=svc-zookeeper）
├── cmd/provision/          # 建库 + 迁移 runner（nemo_zookeeper；MYSQL_DSN env 执行，幂等）
├── migrations/             # 000_provision.sql（建库）+ 001_biz.sql（clusters/audits 表）
└── internal/
    ├── config/             # Config：RestConf + Auth + Mysql + Authz（含 Service 服务身份）
    ├── types/              # API 契约（ClusterResp 含行级徽标、ZnodeTreeResp 节点树）
    ├── repo/               # ClusterRepo/AuditRepo：接口契约 + 内存/MySQL 双实现
    ├── svc/                # ServiceContext：Repo 装配 + Authz（用户）+ AuthzService（服务身份）
    ├── logic/              # 集群 CRUD + 节点浏览（mock 树）+ 对账 + common
    └── handler/            # handler + routes.go（守卫路由 + healthz/readyz）
```

## 使用方式

本地启动（必须在服务根目录，`-f` 相对 cwd 解析）：

```bash
cd services/zookeeper
MYSQL_DSN="root:***@tcp(127.0.0.1:3306)/" go run ./cmd/provision  # 建库 nemo_zookeeper + clusters/audits
go build -o /tmp/zookeeper-api-new zookeeper.go
nohup /tmp/zookeeper-api-new > /tmp/zookeeper-api.log 2>&1 &
curl localhost:8087/healthz
```

一键恢复：`bash scripts/restore-dev.sh`（含 zookeeper-api :8087 与前端 :5278 拉起、
svc-zookeeper 授 zookeeper:manage 码）。

## API（/v1/zookeeper/*，全部 JWT 鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /v1/zookeeper/clusters | 集群全量列表（读无守卫；普通用户按引擎 can_view 过滤——manage 码全量；owner/can_manage_grants 徽标批量取回） |
| GET | /v1/zookeeper/clusters/:id/nodes | 节点树浏览（**守卫：zk_cluster:can_view 主判定 + zookeeper:manage 回退**——看到集群才能浏览节点，不可见集群 403；阶段一 mock 树） |
| POST | /v1/zookeeper/clusters | 新建集群（守卫：zookeeper:manage 码；code 必填唯一 409；写库 + 服务身份写 owner 元组） |
| DELETE | /v1/zookeeper/clusters/:id | 删除集群（守卫：zookeeper:manage 码；先删库后清元组，清理失败记日志待对账） |
| POST | /v1/zookeeper/reconcile | 授权对账（守卫：zookeeper:manage 码；按 created_by 补写缺失 owner，返回 {scanned, repaired}） |
| GET | /healthz、/readyz | 存活/就绪探针 |

## 前端联调

微应用 `console-frontend/apps/zookeeper`（端口 5278）：vite proxy 把
`/api/v1/zookeeper` 分流到 :8087；`/api/v1/admin`、`/api/v1/authz`、`/api/v1/me`
指 authz（:8084），`/api/v1/users` 指用户服务（:8081）。

## 判定语义（与集中式 authz 固定模板同口径）

- **两层权限正交**：菜单可见性（`zookeeper:read`）≠ 集群可见性（can_view）≠
  节点浏览权（= 集群 can_view，同源守卫）。
- 集群创建人自动成为 owner（服务身份写 `user:{sub} owner zk_cluster:{id}` 元组）；
  要给更多人可见，用集群工具条「集群权限」（ResourceGrantDialog →
  /v1/authz/relations，zk_cluster + viewer 单关系）授予；部门主体
  `org:{id}#member` 自动子树继承。
- **守卫决定性**：未授权用户直调节点接口 403（fail-closed）——节点浏览权与
  集群可见性同源，不存在"看不到集群却能调节点接口"的旁路。
- 与 redis 的差异：无实例级授权（znode 不建元组，集群是唯一授权边界）、无
  实例派生通道、无非空删除校验（无业务子实体）。
- 注册表管理码缺口（平台既有）：zk_cluster 派生注册后需手工
  `UPDATE resource_types SET manage_code='zookeeper:manage' WHERE name='zk_cluster'`
  （restore-dev/生产部署清单项，与 redis_instance/redis_cluster 同）。

## 部署清单（新增项）

1. CAS 服务账号：svc-zookeeper（密码 zookeeper-pass；生产由 IdP 建）。
2. 授码：`permissions/grants` → svc-zookeeper: zookeeper:manage；admin 同（幂等，
   restore-dev.sh 已含）。
3. 注册表管理码：上条 UPDATE（zk_cluster 派生注册后执行一次）。
4. provision：nemo_zookeeper 建库 + clusters/audits 迁移。
