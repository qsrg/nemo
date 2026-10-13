// guard.go 提供 redis 路由的声明式权限守卫：kit/authz RequireWith/RequireCode
// 的 redis_instance 装配。带实例 ID 的行级判定（编辑/删除）与无实例 ID 的
// 集合入口判定（新建）全部上收路由层，对应 logic 不再内联权限判定
// （业务代码只关注业务）。
//
// 判定链路（RequireWith/RequireCode）：redis_instance:{relation} 主判定（经
// authz check，内置 ABAC 叠加）→ 明确无权时回退权限码 RedisManageCode
// （经 authz list-objects 实时查有效权限码集合，页面级管理码"管理员全权"
// 语义）→ 无权 403 / 判定服务故障 503 fail-closed / 路径 ID 非法 400。
package handler

import (
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest/pathvar"

	"nemo/kit/authz"
	"nemo/services/redis/internal/logic"
	"nemo/services/redis/internal/svc"
)

// requireInstancePerm 返回保护 next 的 redis_instance 路由守卫：
//   - relation 主判定关系（can_edit / can_delete）；
//   - 对象 ID 取路径参数 :id（经 go-zero pathvar），非正整数按 400 拒绝
//     （kit REDIS_OBJECT_INVALID），不进判定；
//   - 回退链 [RedisManageCode]：管理码持有者全权。
func requireInstancePerm(sc *svc.ServiceContext, relation string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireWith(sc.Authz, authz.Options{
		Perm:          "redis_instance:" + relation,
		ObjectID:      instanceIDFromPath,
		FallbackCodes: []string{logic.RedisManageCode},
	})
}

// requireCodeOrManage 返回「指定动作码 或 管理码」守卫：无实例的全局功能
// 按钮（新建）差异化用户可单独授动作码（redis:create），管理码持有者天然
// 兜底全权——与页面级三档语义一致，不必重复授权。
func requireCodeOrManage(sc *svc.ServiceContext, code string) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, code, logic.RedisManageCode)
}

// requireInstanceManagePerm 返回授权名单管理守卫（通用 relations 接口在
// authz 侧按对象类型校验——owner 或管理码，**不含 editor**，可编辑属性不
// 放大为名单管理权，防提权）。本服务不承载名单端点（前端走通用
// ResourceGrantDialog → /v1/authz/relations），保留构造器与徽标口径同源。
func requireInstanceManagePerm(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return requireInstancePerm(sc, "owner")
}

// requireClusterManageCode 返回集群写操作守卫：集群是基础配置数据（分组
// 维度，不进权限引擎、无资源关系），全部写操作（新建/删除）单挂管理码
// redis:manage——管理档天然全权，无按钮动作码（与 kafka 集群同口径）。
func requireClusterManageCode(sc *svc.ServiceContext) func(http.HandlerFunc) http.HandlerFunc {
	return authz.RequireCode(sc.Authz, logic.RedisManageCode)
}

// instanceIDFromPath 从 go-zero 路由匹配注入的路径参数取 :id 并校验为
// 正整数（与 logic.parseInstanceID 同口径）；缺失/非法返回 ok=false。
func instanceIDFromPath(r *http.Request) (string, bool) {
	id, err := strconv.ParseInt(pathvar.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}
