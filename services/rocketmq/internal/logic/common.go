// common.go 承载 rocketmq 判定的公共小函数：身份提取、权限码判定、
// rmq_cluster/rmq_topic 关系集合加载（全部经集中式 authz 实时查询）。
// 授权模型（2026-10-12 定稿）：集群级——集群可见（rmq_cluster can_view）
// 即该集群下全部 topic 可见可操作；rmq_topic 不再单独授权。
package logic

import (
	"context"
	"strconv"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/rocketmq/internal/svc"
)

// RocketMQTopicsManageCode 是 rocketmq 服务管理码（管理员全权语义：
// 持有者天然可见全部集群与 topic + 可增删改，无需逐资源授权）。
const RocketMQTopicsManageCode = "rocketmq-topics:manage"

// RocketMQTopicsCreateCode 是新建 Topic 的动作码（无实例的全局功能，与
// 管理码二选一即可创建）。
const RocketMQTopicsCreateCode = "rocketmq-topics:create"

// RocketMQClustersReadCode 是集群管理页菜单码（页面入口过滤，非判定真源）。
const RocketMQClustersReadCode = "rocketmq-clusters:read"

// operatorFrom 从 ctx 提取当前操作人（JWT claims.sub，经 authzkit.IdentityFrom）。
// ctx 无身份（未经 Identity 中间件的异常路径）返回 401 fail-closed。
func operatorFrom(ctx context.Context) (string, error) {
	sess, err := authzkit.IdentityFrom(ctx)
	if err != nil || strings.TrimSpace(sess.Sub) == "" {
		return "", errors.Unauthorized("AUTHZ_UNAUTHORIZED", "缺少用户身份")
	}
	return sess.Sub, nil
}

// codeGranted 判定当前用户是否持有权限码 code：经 authz ListObjects
// 实时查有效权限码集合（permission:xxx → 反解冒号码）再精确/通配匹配
// （kit/authz PermGranted 语义）。管理码回退的公共底座。
func codeGranted(ctx context.Context, sc *svc.ServiceContext, code string) (bool, error) {
	perms, err := sc.Authz.ListObjects(ctx, "permission", "can")
	if err != nil {
		return false, err
	}
	granted := false
	for _, p := range perms {
		rest, ok := strings.CutPrefix(p, "permission:")
		if !ok {
			continue
		}
		if authzkit.PermGranted(code, []string{strings.Replace(rest, ".", ":", 1)}) {
			granted = true
			break
		}
	}
	return granted, nil
}

// rmqTopicObject 构造 rmq_topic 对象 id（rmq_topic:{id}，与引擎元组一致）。
func rmqTopicObject(id int64) string {
	return "rmq_topic:" + strconv.FormatInt(id, 10)
}

// rmqClusterObject 构造 rmq_cluster 对象 id（rmq_cluster:{id}，与引擎元组一致）。
func rmqClusterObject(id int64) string {
	return "rmq_cluster:" + strconv.FormatInt(id, 10)
}

// clusterRelationSet 加载当前用户对 rmq_cluster 某出口的直接关系对象集合
//（ListObjects 返回 rmq_cluster:{id}），转 object id → true 的 map。
func clusterRelationSet(ctx context.Context, sc *svc.ServiceContext, relation string) (map[string]bool, error) {
	objs, err := sc.Authz.ListObjects(ctx, "rmq_cluster", relation)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objs))
	for _, o := range objs {
		set[o] = true
	}
	return set, nil
}

// relationSet 加载当前用户对 rmq_topic 某出口的直接关系对象集合（保留：
// 对账等遗留路径用；常规列表已切集群级）。
func relationSet(ctx context.Context, sc *svc.ServiceContext, relation string) (map[string]bool, error) {
	objs, err := sc.Authz.ListObjects(ctx, "rmq_topic", relation)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objs))
	for _, o := range objs {
		set[o] = true
	}
	return set, nil
}
