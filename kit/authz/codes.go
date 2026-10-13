// codes.go 提供平台权限码（permission 对象）的判定件：EffectiveCodes 实时
// 查询当前用户的有效权限码集合、PermGranted 三级匹配语义、CodeGranted 单码
// 判定。
//
// 平台契约（与 authz judgmentcommon、前端 hasPermission 对齐——本文件是该
// 语义在 kit 侧的唯一权威实现，服务侧不得再复制）：
//   - 权限码形如 "<服务>:<动作>"（如 kafka-topics:manage）；
//   - 引擎对象形如 permission:<服务>.<动作>（OpenFGA 对象 id 不允许冒号，
//     码内冒号在对象侧替换为点，映射为双射）；
//   - 授予 = 主体（user:{id} 或 org:{id}#member）对 permission 对象持有 can
//     关系，组织成员含 parent 链继承均原生生效，撤权即时生效。
//
// 职责边界（kit 纪律）：permission 是平台内建对象类型（与 user/org 同级），
// 非业务名词；本文件不含任何服务专属逻辑。
package authz

import (
	"context"
	"sort"
	"strings"
)

// EffectiveCodes 经 Checker 实时查询当前用户的有效权限码集合：
// ListObjects("permission", "can") 枚举当前用户可用的权限点对象（用户身份
// 由 Checker 从 ctx 内 JWT 透传），对象 id 去前缀、首点还原为冒号后升序去重
// 返回。无权限返回空切片（非 nil）。判定服务故障返回 error，调用方转 503
// fail-closed。
func EffectiveCodes(ctx context.Context, c Checker) ([]string, error) {
	objects, err := c.ListObjects(ctx, "permission", "can")
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objects))
	codes := make([]string, 0, len(objects))
	for _, o := range objects {
		code := codeFromObject(o)
		if code == "" || set[code] {
			continue
		}
		set[code] = true
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, nil
}

// PermGranted 判定权限码 code 是否被权限集合 perms 授予——三级语义（逐字
// 与前端 hasPermission / authz judgmentcommon 对齐）：
//  1. 精确码：perms 含 code 本身；
//  2. 段内通配码：*:动作（跨服务同动作）/ 服务:*（本服务跨动作）/
//     * 与 *:*（全部）；
//  3. 纯服务码：perms 含 code 的服务段（裸服务名，授予整个服务）。
//
// 其余一律不授予（无 read 兜底其他动作；前端显隐 fail-closed，资源级安全
// 由服务端 Check 把守）。code 须为 "<服务>:<动作>" 形态，无冒号直接不授予。
func PermGranted(code string, perms []string) bool {
	for _, p := range perms {
		if p == code {
			return true
		}
	}
	sep := strings.Index(code, ":")
	if sep <= 0 {
		return false
	}
	service, action := code[:sep], code[sep+1:]
	for _, p := range perms {
		switch p {
		case "*:" + action, service + ":*", "*:*", "*", service:
			return true
		}
	}
	return false
}

// CodeGranted 判定当前用户是否持有权限码：EffectiveCodes 实时查询 +
// PermGranted 匹配。管理码守卫（如页面级管理码）用它一行完成；无码返回
// (false, nil)，故障返回 error（调用方转 503 fail-closed）。
func CodeGranted(ctx context.Context, c Checker, code string) (bool, error) {
	perms, err := EffectiveCodes(ctx, c)
	if err != nil {
		return false, err
	}
	return PermGranted(code, perms), nil
}

// codeFromObject 从引擎对象 id 反解权限码：permission:<服务>.<动作> →
// <服务>:<动作>（首点还原为冒号，与 authz permissioncommon.permObject 的
// 双射一致）；非 permission 前缀返回空串（调用方跳过）。
func codeFromObject(object string) string {
	rest, ok := strings.CutPrefix(object, "permission:")
	if !ok {
		return ""
	}
	return strings.Replace(rest, ".", ":", 1)
}
