// guard.go 提供 rocketmq 路由的声明式权限守卫：kit/authz RequireWith/
// RequireCode 的 rmq_cluster 装配。集群级授权模型（2026-10-12 定稿）：
// 集群可见（rmq_cluster can_view）即该集群下全部 topic 可见可操作——
// 行级操作（编辑/删除）按路径 id 先查 topic 归属集群，再对该集群做
// can_view 判定；无实例 ID 的集合入口判定（新建）仍用动作码/管理码。
//
// 判定链路：rmq_cluster:{relation} 主判定（经 authz check）→ 明确无权时
// 回退管理码 rocketmq-topics:manage（页面级管理码"管理员全权"语义）→
// 无权 403 / 判定服务故障 503 fail-closed / 路径 ID 非法 400。
package handler

import (
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest/pathvar"

	"nemo/kit/authz"
	"nemo/services/rocketmq/internal/logic"
	"nemo/services/rocketmq/internal/svc"
)

// requireClusterPerm 返回保护 next 的集群级路由守卫：
//   - relation 主判定关系（can_view——集群可见即可操作，含 owner 派生）；
//   - 对象 ID 取路径参数 :id（topic 行 id）→ 查 topic 归属集群 id；
//   - 回退链 [RocketMQTopicsManageCode]：管理码持有者全权。
func requireClusterPerm(sc *svc.ServiceContext, relation string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireWith(sc.Authz, authz.Options{
		Perm:          "rmq_cluster:" + relation,
		ObjectID:      clusterIDOfTopic(sc),
		FallbackCodes: []string{logic.RocketMQTopicsManageCode},
	})
}

// requireCodeOrManage 返回「指定动作码 或 管理码」守卫：无实例的全局功能
// 按钮（新建）差异化用户可单独授动作码（rocketmq-topics:create），管理码
// 持有者天然兜底全权。
func requireCodeOrManage(sc *svc.ServiceContext, code string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, code, logic.RocketMQTopicsManageCode)
}

// requireClusterManageCode 返回「仅管理码」守卫：集群是基础配置数据 +
// 授权锚点，新建/删除集群只有管理档维护（无按钮动作码，与 redis/kafka
// 集群同口径）。
func requireClusterManageCode(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, logic.RocketMQTopicsManageCode)
}

// clusterIDOfTopic 构造「路径 topic id → 所属集群 id」的对象 ID 提取器：
// 取 :id 校验为正整数 → TopicRepo.Get 查归属集群（未命中 404 语义转
// ok=false → 400；集群 0 视作非法）。返回**纯集群 id**（RequireWith 会
// 自行拼 objType:id，勿带类型前缀）。
func clusterIDOfTopic(sc *svc.ServiceContext) func(*http.Request) (string, bool) {
	return func(r *http.Request) (string, bool) {
		id, err := strconv.ParseInt(pathvar.Vars(r)["id"], 10, 64)
		if err != nil || id <= 0 {
			return "", false
		}
		t, gerr := sc.TopicRepo.Get(r.Context(), id)
		if gerr != nil {
			return "", false
		}
		if t.ClusterId <= 0 {
			return "", false
		}
		return strconv.FormatInt(t.ClusterId, 10), true
	}
}
