// kafkacommon.go 承载 kafka 独立服务的公共小函数：对象标识、授权名单
// （viewer/editor 双名单）元组构造与资源级判定（经 kit/authz 远程调用
// 集中式 authz，不内嵌引擎）。
//
// 授权模型（v6，与原内嵌实现逐字一致）：kafka_topic 的可见性 = owner or
// viewer or editor；编辑 = owner or editor（页面级 manage 码保留"新建入口
// + 管理员全权"语义）。
//
// 判定面映射：资源级判定经 kit/authz Checker（authz /v1/authz/check、
// /v1/authz/list-objects，user 取 JWT claims.sub，check 内置 ABAC 叠加）；
// 元组读写经 kit/authz Remote 的 relations（authz /v1/authz/relations，名单
// 管理权由 authz 侧按对象类型校验）；权限码集合查询与三级匹配语义已上收
// kit/authz（CodeGranted/PermGranted——authz judgmentcommon 同语义的权威
// 实现，服务侧不再各持一份拷贝）。
package logic

import (
	"context"
	"strconv"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/kafka/internal/types"
)

// KafkaTopicsManageCode 是 Topic 管理页的页面级菜单码：控制"管理员全权"
// （全部 Topic 可编辑/可改授权/可删除）与无实例全局操作的兜底（新建/对账）；
// 单 Topic 的编辑/重置位点由 can_edit（editor，含 owner）判定、删除由
// can_delete（owner，2026-09-30 起独立边界）、名单管理由 can_manage_grants
// （owner），查看由 can_view（owner or viewer or editor）判定。导出供
// handler 侧路由守卫（guard.go）声明回退链使用。
const KafkaTopicsManageCode = "kafka-topics:manage"

// KafkaClustersManageCode 是集群管理页的写操作管理码：read 码（kafka-clusters:read）
// 控菜单可见与列表读取，manage 码控全部写操作（新建/编辑/删除）——集群是
// 基础配置数据，无资源关系模型，管理档天然全权（与人员授权三档语义一致：
// 管理无需另勾按钮码）。供 handler 侧路由守卫 requireClusterManageCode 使用。
const KafkaClustersManageCode = "kafka-clusters:manage"

// 动作码（按钮级功能权限）：无实例的全局功能按钮的独立管控码——与
// read/manage 同族（服务:动作），授给谁谁就能用对应按钮；后端路由守卫
// （authz.RequireCode）与前端 has() 显隐同源。行内资源级操作（编辑/删除/
// 重置位点/配置权限）不建码，走 kafka_topic 资源关系（can_edit/can_delete/
// can_manage_grants）。
const (
	// KafkaTopicsCreateCode 工具栏「新建 Topic」按钮（全局操作，无实例）。
	KafkaTopicsCreateCode = "kafka-topics:create"
	// KafkaTopicsExportCode 工具栏「导出清单」按钮（全局操作，无实例）。
	KafkaTopicsExportCode = "kafka-topics:export"
)

// topicObject 返回 Topic 的引擎对象标识（kafka_topic:{int64}）。
func topicObject(id int64) string { return "kafka_topic:" + strconv.FormatInt(id, 10) }

// parseTopicID 把 API 层字符串形式的 topic ID 解析为 int64；非法格式返回 400。
// API 契约保留字符串 ID（前端 JSON 数字精度兼容），repo 与判定面统一用 int64。
func parseTopicID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.BadRequest("AUTHZ_TOPIC_INVALID", "topic ID 非法")
	}
	return id, nil
}

// topicDenied 是菜单管理码判定不满足的统一 403（写接口共用）。
func topicDenied() error {
	return errors.Forbidden("AUTHZ_DENIED", "无操作权限")
}

// topicEngineErr 是判定/授权服务故障的统一 503（fail-closed，同 authz
// 判定面口径：不区分未注册类型与引擎故障）。
func topicEngineErr() error {
	return errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
}

// topicGrantSubject 把授予目标（user/org）转为元组 user 形态：
// user → user:{id}；org → org:{id}#member（部门成员集合，v5 聚合语义下
// 授予部门即覆盖其子树全部成员）。非法 targetType 返回 400。
func topicGrantSubject(targetType, targetID string) (string, error) {
	id := strings.TrimSpace(targetID)
	if id == "" {
		return "", errors.BadRequest("AUTHZ_TOPIC_INVALID", "授予目标 ID 不能为空")
	}
	switch targetType {
	case "user":
		return "user:" + id, nil
	case "org":
		return "org:" + id + "#member", nil
	default:
		return "", errors.BadRequest("AUTHZ_TOPIC_INVALID", "targetType 仅支持 user 或 org")
	}
}

// topicGrantTuple 构造授予元组：{subject} {viewer|editor} kafka_topic:{int64}。
func topicGrantTuple(id int64, subject, relation string) authzkit.Tuple {
	return authzkit.Tuple{User: subject, Relation: relation, Object: topicObject(id)}
}

// menuManageGranted 判定调用者（ctx 内 JWT 用户）是否持有菜单管理码：
// 委托 kit/authz CodeGranted（EffectiveCodes 实时查询 + PermGranted 三级
// 匹配，权威语义见 kit/authz/codes.go——三级：精确码 / 段内通配码 /
// 纯服务码；跨服务通配不授予）。引擎故障由调用方转 503 fail-closed。
func menuManageGranted(ctx context.Context, c authzkit.Checker, manageCode string) (bool, error) {
	return authzkit.CodeGranted(ctx, c, manageCode)
}

// topicCanEdit 判定调用者对某 Topic 的编辑权（模型 v6 can_edit = owner or
// editor，经集中式判定面判定——check 内置 ABAC 叠加；页面级管理码
// kafka-topics:manage 持有者全权）。**仅 list 的逐条徽标判定使用**——update/
// delete/grant/revoke/consumers 的入口判定已全部上收路由守卫
// （handler/guard.go：requireTopicPerm/requireTopicManagePerm）。
func topicCanEdit(ctx context.Context, c authzkit.Checker, id int64) (bool, error) {
	allowed, err := c.Check(ctx, topicObject(id), "can_edit")
	if err != nil {
		return false, err
	}
	if allowed {
		return true, nil
	}
	return menuManageGranted(ctx, c, KafkaTopicsManageCode)
}

// topicGrantSubjectOf 把元组 user 形态解析回目标类型/ID（topicGrantSubject
// 的逆变换）；非 user:/org: 形态原样按 user 处理（健壮性兜底）。
func topicGrantSubjectOf(user string) (targetType, targetId string) {
	if rest, ok := strings.CutPrefix(user, "org:"); ok {
		return "org", strings.TrimSuffix(rest, "#member")
	}
	return "user", strings.TrimPrefix(user, "user:")
}

// listGrantsOf 整理授予列表：元组 user 形态解析为目标类型/ID，按
// （targetType, targetId, relation）升序（grant/list 共用）。
func listGrantsOf(tuples []authzkit.Tuple) []types.TopicGrantResp {
	out := make([]types.TopicGrantResp, 0, len(tuples))
	for _, t := range tuples {
		targetType, targetId := topicGrantSubjectOf(t.User)
		out = append(out, types.TopicGrantResp{TargetType: targetType, TargetId: targetId, Relation: t.Relation})
	}
	// 冒泡排序保持确定性（与 authz 原实现 sort.Slice 语义一致）。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, b := out[j-1], out[j]
			if a.TargetType > b.TargetType ||
				(a.TargetType == b.TargetType && (a.TargetId > b.TargetId ||
					(a.TargetId == b.TargetId && a.Relation > b.Relation))) {
				out[j-1], out[j] = b, a
			}
		}
	}
	return out
}
