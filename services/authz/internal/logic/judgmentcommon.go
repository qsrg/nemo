// judgmentcommon.go 承载判定面（Check/ListObjects/MePermissions/MeMenus）的
// 公共小函数：身份提取（fail-closed 401）与权限码集合的引擎实时查询。
//
// 去角色化改造：权限码不再由 roles claim × 规则表派生，改为对授权引擎
// （type permission）做 ListObjects 实时查询——权限点对象 id = 权限码，
// 授权 = 人/组织 × 权限点一条 can 元组，撤权/组织变更即时生效。
// permGranted 过滤语义不变（me/menus 逐行判定消费，与前端 hasPermission 对齐）。
package logic

import (
	"context"
	"sort"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
)

// identityFrom 从 ctx 提取当前用户身份（JWT claims.sub）与角色列表。
// ctx 无身份（未经 Identity 中间件的异常路径）返回 401 fail-closed。
// roles 为空切片不是错误（roles claim 保留仅供兼容展示，不再参与权限派生）。
func identityFrom(ctx context.Context) (sub string, roles []string, err error) {
	sess, err := authzkit.IdentityFrom(ctx)
	if err != nil || strings.TrimSpace(sess.Sub) == "" {
		return "", nil, errors.Unauthorized("AUTHZ_UNAUTHORIZED", "缺少用户身份")
	}
	return sess.Sub, sess.Roles, nil
}

// permGranted 服务端判定权限码 code 是否被权限集合 perms 授予——匹配语义
// 已上收 kit/authz（PermGranted：精确码 / 段内通配码 / 纯服务码三级，其余
// 一律不授予，与前端 hasPermission / permission.test.ts 对齐），本函数保留
// 为包内别名，me/menus 既有调用点零改动。
func permGranted(code string, perms []string) bool {
	return authzkit.PermGranted(code, perms)
}

// effectivePermCodes 经授权引擎实时查询主体的有效权限码集合（去角色化改造）：
// ListObjects(user:{sub}, can, permission) → 引擎沿 permission.can 的
// [user, org#member] 两种主体形态求值（直接授予 + 组织成员含 parent 链
// 继承，全部原生），对象 id 即权限码——去 "permission:" 前缀后升序返回
// （通配对象 permission:*:read → *:read，与前端 hasPermission 通配语义衔接）。
// 无权限返回空切片（非 nil）。引擎错误由调用方转 503 fail-closed。
func effectivePermCodes(ctx context.Context, h *engine.Handle, sub string) ([]string, error) {
	objects, err := h.ListObjects(ctx, "user:"+sub, "can", "permission")
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objects))
	codes := make([]string, 0, len(objects))
	for _, o := range objects {
		code := permCode(o)
		if code == "" || set[code] {
			continue
		}
		set[code] = true
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, nil
}

// menuManageGranted 判定主体是否持有菜单管理码（页面写操作权限点，如
// kafka-topics:manage）：effectivePermCodes 实时查询 + permGranted 语义
// （精确码/通配）。kafka Topic 演示域写接口（create/update/delete）统一以
// 它替代 ABAC X-User-Level 判定——菜单级授权取代等级模拟，授权测试即在
// 人员授权页授 read/manage 码。引擎错误由调用方转 503 fail-closed。
func menuManageGranted(ctx context.Context, h *engine.Handle, sub, manageCode string) (bool, error) {
	perms, err := effectivePermCodes(ctx, h, sub)
	if err != nil {
		return false, err
	}
	return permGranted(manageCode, perms), nil
}
