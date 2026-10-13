// rediscommon.go 承载 redis 独立服务的公共小函数：对象标识、权限码常量与
// 统一错误构造（经 kit/authz 远程调用集中式 authz，不内嵌引擎）。
//
// 授权模型（菜单声明派生，与引擎固定模板一致）：redis_instance 的可见性
// can_view = member_view or owner（member_view = viewer or editor or
// member from org——本服务不写 org 元组，org 维度恒空）；编辑 = can_edit
// = editor（editor 关系含 owner）；删除 = can_delete = owner（独立边界）；
// 名单管理 = can_manage_grants = owner。
//
// 判定面映射：资源级判定经 kit/authz Checker（authz /v1/authz/check、
// /v1/authz/list-objects、/v1/authz/batch-check，user 取 JWT claims.sub）；
// 元组读写经 kit/authz Remote 的 relations（authz /v1/authz/relations）。
package logic

import (
	"context"
	"strconv"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
)

// RedisManageCode 是实例管理页的页面级管理码（菜单根行 redis 的 base 派生
// 族）：控制"管理员全权"（全部实例可见/可编辑/可删除/可配置名单）与新建
// 入口的兜底；单实例的编辑/删除/名单由 can_edit/can_delete/can_manage_grants
// 资源关系判定。供 handler 侧路由守卫（guard.go）声明回退链使用。
const RedisManageCode = "redis:manage"

// RedisCreateCode 是工具栏「新建实例」按钮的动作码（全局操作，无实例）：
// 授给谁谁就能新建；管理码持有者天然兜底全权。
const RedisCreateCode = "redis:create"

// instanceObject 返回实例的引擎对象标识（redis_instance:{int64}）。
func instanceObject(id int64) string { return "redis_instance:" + strconv.FormatInt(id, 10) }

// parseInstanceID 把 API 层字符串形式的实例 ID 解析为 int64；非法格式返回
// 400。API 契约保留字符串 ID（前端 JSON 数字精度兼容），repo 与判定面统一
// 用 int64。
func parseInstanceID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.BadRequest("REDIS_INSTANCE_INVALID", "实例 ID 非法")
	}
	return id, nil
}

// clusterObject 返回集群的引擎对象标识（redis_cluster:{int64}，与资源类型
// 注册名一致——集群 2026-09-30 起进权限引擎，owner/viewer 元组的 object 用；
// 审计 target 亦复用此标识）。
func clusterObject(id int64) string { return "redis_cluster:" + strconv.FormatInt(id, 10) }

// parseClusterID 把 API 层字符串形式的集群 ID 解析为 int64；非法格式返回 400。
func parseClusterID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.BadRequest("REDIS_CLUSTER_INVALID", "集群 ID 非法")
	}
	return id, nil
}

// instanceEngineErr 是判定/授权服务故障的统一 503（fail-closed，同 authz
// 判定面口径：不区分未注册类型与引擎故障）。
func instanceEngineErr() error {
	return errors.New(503, "REDIS_ENGINE_ERROR", "权限判定服务异常")
}

// menuManageGranted 判定调用者（ctx 内 JWT 用户）是否持有菜单管理码：
// 委托 kit/authz CodeGranted（EffectiveCodes 实时查询 + PermGranted 三级
// 匹配，权威语义见 kit/authz/codes.go——三级：精确码 / 段内通配码 /
// 纯服务码；跨服务通配不授予）。引擎故障由调用方转 503 fail-closed。
func menuManageGranted(ctx context.Context, c authzkit.Checker, manageCode string) (bool, error) {
	return authzkit.CodeGranted(ctx, c, manageCode)
}
