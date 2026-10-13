# rocketmq 服务

RocketMQ 管控微服务（独立后端 :8088 + 前端微应用 :5279），授权走集中式 authz（OpenFGA 引擎）。

## 授权模型（集群级）

- 资源对象 = `rmq_cluster`（集群），topic 不单独入引擎。
- **集群可见（`rmq_cluster` can_view）即该集群下全部 topic 可见且可操作**：列表过滤 + 编辑/删除守卫均以集群为准。
- 页面级功能按钮（新建/删除 Topic 等）按权限码（`rocketmq-topics:*`），由服务端路由守卫判定。
- 平台管理员（`permission:*.*`）与持有 `rocketmq-topics:manage` 管理码的用户拥有全量能力（回退链）。
- 所有授权操作通过页面完成：人员授权页 + 资源权限对话框（写引擎元组）。

## 启动方式

```bash
# 后端（必须在 services/rocketmq 目录下启动，相对路径读 etc/rocketmq-api.yaml）
go build -o /tmp/rocketmq-api-new rocketmq.go
./rocketmq-api-new          # 或一键脚本 scripts/restore-dev.sh

# 前端（apps/rocketmq，端口 5279）
cd console-frontend/apps/rocketmq
./node_modules/.bin/vite --port 5279 --host
```

依赖：MySQL（库 `nemo_rocketmq`，`migrations/` 建库迁移，provision 幂等）+ authz 服务（:8084，判定 + 元组写）。

## 关键配置（etc/rocketmq-api.yaml）

- `Auth.AccessSecret`：与 authz 共享的签发密钥（解析平台 JWT）。
- `Mysql.Dsn`：业务库连接串（本地 dev 值，生产环境变量注入）。
- `Authz.BaseURL`：集中式授权服务地址；`Authz.Timeout` 单次判定超时。

## 数据库

- `topics`：Topic 主表（含 `cluster_id` 归属集群、审计字段）。
- `clusters`：集群表（`code` 唯一；建集群即写 `rmq_cluster` owner 元组）。
- 迁移：`migrations/001_*.sql`（建库建表）、`migrations/002_clusters.sql`（集群表 + topics 加 cluster_id + 默认集群收纳存量）。

## 接口

- `GET/POST /v1/rocketmq/clusters`、`DELETE /v1/rocketmq/clusters/:id`（写操作挂 manage 码）。
- `GET /v1/rocketmq/topics?clusterId=`（必传集群；可见性过滤）。
- `POST /v1/rocketmq/topics`（clusterId 必填，JSON 数字；非 manage 需集群 can_view）。
- `PUT/DELETE /v1/rocketmq/topics/:id`（守卫：集群 can_view，manage 码回退）。
