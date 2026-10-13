// guard.go 提供 zookeeper 路由的声明式权限守卫：kit/authz RequireWith/
// RequireCode 的 zk_cluster 装配。集群写操作（新建/删除/对账）挂管理码，
// 节点浏览挂集群 can_view 主判定 + 管理码回退——"看到集群才能浏览节点"
// 与集群列表同口径（判定面同一引擎）。
package handler

import (
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest/pathvar"

	"nemo/kit/authz"
	"nemo/services/zookeeper/internal/logic"
	"nemo/services/zookeeper/internal/svc"
)

// requireClusterManageCode 返回集群写操作守卫：新建/删除/对账单挂管理码
// zookeeper:manage——管理档天然全权（与 redis 集群管理同口径）。
func requireClusterManageCode(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, logic.ZookeeperManageCode)
}

// requireClusterView 返回节点浏览守卫："看到集群才能浏览节点"——
// zk_cluster:can_view 主判定（引擎 check，与集群列表 ListObjects 同源），
// 管理码持有者回退全权；对象 ID 取路径参数 :id，非正整数按 400 拒绝
// （kit REDIS_OBJECT_INVALID 同源语义），不进判定。
func requireClusterView(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireWith(sc.Authz, authz.Options{
		Perm:          "zk_cluster:can_view",
		ObjectID:      clusterIDFromPath,
		FallbackCodes: []string{logic.ZookeeperManageCode},
	})
}

// clusterIDFromPath 从 go-zero 路由匹配注入的路径参数取 :id 并校验为
// 正整数（与 logic.parseClusterID 同口径）；缺失/非法返回 ok=false。
func clusterIDFromPath(r *http.Request) (string, bool) {
	id, err := strconv.ParseInt(pathvar.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}
