// zookeepercommon.go 承载 zookeeper 独立服务的公共小函数：引擎对象标识、
// 管理码常量与统一错误构造（经 kit/authz 远程调用集中式 authz，不内嵌引擎）。
//
// 授权模型（菜单声明派生，与引擎固定模板一致）：zk_cluster 的可见性
// can_view = member_view or owner（member_view = viewer or editor or
// member from org——本服务不写 org 元组，org 维度恒空）；本服务只读浏览
// （znode 无写操作），故无 can_edit/can_delete 业务出口。"看到集群才能
// 浏览节点"= 节点接口按集群 can_view 守卫（路由层）。
package logic

import (
	"context"
	"strconv"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
)

// ZookeeperManageCode 是节点浏览页的页面级管理码：控制"管理员全权"
//（全部集群可见/可建/可删/可配名单）。集群进引擎（zk_cluster），创建写
// owner 元组；普通用户按集群 can_view 可见。
const ZookeeperManageCode = "zookeeper:manage"

// clusterObject 返回集群的引擎对象标识（zk_cluster:{int64}）。
func clusterObject(id int64) string { return "zk_cluster:" + strconv.FormatInt(id, 10) }

// parseClusterID 把 API 层字符串形式的集群 ID 解析为 int64；非法格式返回
// 400。API 契约保留字符串 ID（前端 JSON 数字精度兼容），repo 与判定面统一
// 用 int64。
func parseClusterID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.BadRequest("ZK_CLUSTER_INVALID", "集群 ID 非法")
	}
	return id, nil
}

// clusterEngineErr 是判定/授权服务故障的统一 503（fail-closed，同 authz
// 判定面口径：不区分未注册类型与引擎故障）。
func clusterEngineErr() error {
	return errors.New(503, "ZK_ENGINE_ERROR", "权限判定服务异常")
}

// menuManageGranted 判定调用者（ctx 内 JWT 用户）是否持有菜单管理码：
// 委托 kit/authz CodeGranted（EffectiveCodes 实时查询 + PermGranted 三级
// 匹配）。引擎故障由调用方转 503 fail-closed。
func menuManageGranted(ctx context.Context, c authzkit.Checker, manageCode string) (bool, error) {
	return authzkit.CodeGranted(ctx, c, manageCode)
}
