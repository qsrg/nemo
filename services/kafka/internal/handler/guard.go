// guard.go 提供 kafka 路由的声明式权限守卫：kit/authz RequireWith/RequireCode
// 的 kafka_topic 装配。带实例 ID 的行级判定（编辑/查看）与无实例 ID 的
// 集合入口判定（新建）全部上收路由层，对应 logic 不再内联权限判定
// （业务代码只关注业务）。
//
// 判定链路（RequireWith/RequireCode）：kafka_topic:{relation} 主判定（经
// authz check，内置 ABAC 叠加）→ 明确无权时回退权限码 KafkaTopicsManageCode
// （经 authz list-objects 实时查有效权限码集合，页面级管理码"管理员全权"
// 语义）→ 无权 403 / 判定服务故障 503 fail-closed / 路径 ID 非法 400。
package handler

import (
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest/pathvar"

	"nemo/kit/authz"
	"nemo/services/kafka/internal/logic"
	"nemo/services/kafka/internal/svc"
)

// requireTopicPerm 返回保护 next 的 kafka_topic 路由守卫：
//   - relation 主判定关系（can_edit / can_view）；
//   - 对象 ID 取路径参数 :id（经 go-zero pathvar，多级路径同样成立），
//     非正整数按 400 拒绝（kit AUTHZ_OBJECT_INVALID），不进判定；
//   - 回退链 [KafkaTopicsManageCode]：管理码持有者全权。
func requireTopicPerm(sc *svc.ServiceContext, relation string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireWith(sc.Authz, authz.Options{
		Perm:          "kafka_topic:" + relation,
		ObjectID:      topicIDFromPath,
		FallbackCodes: []string{logic.KafkaTopicsManageCode},
	})
}

// requireTopicManageCode 返回保护集合路由（无实例 ID，如对账）的
// 管理码守卫：须持有 KafkaTopicsManageCode（管理员全权语义）。
func requireTopicManageCode(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, logic.KafkaTopicsManageCode)
}

// requireClusterManageCode 返回保护集群写路由（新建/编辑/删除，无实例关系
// 模型）的管理码守卫：须持有 KafkaClustersManageCode。集群是基础配置数据，
// 管理档天然全权，无按钮动作码（与人员授权三档语义一致）。
func requireClusterManageCode(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, logic.KafkaClustersManageCode)
}

// requireCodeOrManage 返回「指定动作码 或 管理码」守卫：无实例的全局功能
// 按钮（新建等）差异化用户可单独授动作码（kafka-topics:create），管理码
// 持有者天然兜底全权——与页面级三档语义一致，不必重复授权。
func requireCodeOrManage(sc *svc.ServiceContext, code string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, code, logic.KafkaTopicsManageCode)
}

// requireActionCode 返回按钮级功能动作码守卫（如导出）：须持有指定动作码
// （kafka-topics:export 等），无码 403 / 判定服务故障 503 fail-closed。
// 对象范围（该动作能作用于哪些资源）由各 logic 层叠加资源级判定
// （两层 AND）。
func requireActionCode(sc *svc.ServiceContext, code string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, code)
}

// requireTopicManagePerm 返回授权名单管理守卫（grant/revoke 接口）：
// owner 或管理码回退——原 topicCanManage 语义（**不含 editor**，可编辑
// 属性不放大为名单管理权，防提权）。
func requireTopicManagePerm(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return requireTopicPerm(sc, "owner")
}

// topicIDFromPath 从 go-zero 路由匹配注入的路径参数取 :id 并校验为
// 正整数（与 logic.parseTopicID 同口径）；缺失/非法返回 ok=false。
func topicIDFromPath(r *http.Request) (string, bool) {
	id, err := strconv.ParseInt(pathvar.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}
