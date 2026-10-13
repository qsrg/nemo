// menuvalidation.go 承载菜单声明的入参校验（原内置菜单注册表
// builtin_menu_registry.go 随 seed 机制移除，校验能力独立保留，供
// create/update menu logic 复用）。
package logic

import (
	"fmt"
	"regexp"
	"strings"

	"nemo/services/authz/internal/repo"
)

// menuResourceTypePattern 校验菜单声明的资源类型名：与引擎注册类型名同口径
// （modelgen 资源类型名：小写字母开头的小写字母/数字/下划线，最长 64）——
// 空串合法（纯菜单），非空必须匹配。
var menuResourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// menuRelationPattern 校验资源级按钮声明的资源关系名（如 can_freeze）：
// 与引擎模型关系命名同族（小写字母开头的小写字母/数字/下划线，最长 64）。
var menuRelationPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// menuRelationOptions 是资源级按钮 relation 的引擎出口白名单（有序，下拉
// 下发与校验共用的唯一清单；与 engine/modelgen canExits 同源）：relation
// 必须命中，否则授权判定会静默失败（授权了也用不了）。新增语义 = 扩展引擎
// 模型出口 + 此处白名单 + 重放模型，三步一起发布（代码常量，不可运行时
// 配置——热加的关系引擎不认识、守卫没接线，必然静默断裂）。
// can_manage_grants 2026-09-30 加入（名单管理权独立边界，owner）。
var menuRelationOptions = []string{
	"can_view",
	"can_edit",
	"can_delete",
	"can_share",
	"can_manage_grants",
}

// menuRelationWhitelist 是白名单判定集（由 menuRelationOptions 派生，二者
// 永远一致——校验走 O(1) 集，下发走有序切片）。
var menuRelationWhitelist = func() map[string]bool {
	m := make(map[string]bool, len(menuRelationOptions))
	for _, r := range menuRelationOptions {
		m[r] = true
	}
	return m
}()

// MenuRelationOptions 返回资源级按钮 relation 的引擎出口白名单（有序副本），
// 供管理面 GET /admin/menus/relations 下发菜单管理页「资源关系」下拉选项。
// 清单为代码常量，新增语义须同步扩展 modelgen canExits 并重放模型后发布。
func MenuRelationOptions() []string {
	out := make([]string, len(menuRelationOptions))
	copy(out, menuRelationOptions)
	return out
}

// splitResourceTypes 将菜单声明的 resource_type 拆分为类型名列表：支持逗号
// 分隔多值（一个页面含多种业务资源时声明多个类型，me/menus 解锁 = 任一类
// 型下有任一资源可见即可见入口），逐项 trim、去空、去重，保持声明顺序。
// 空串输入返回空列表（纯菜单/页面级按钮）。单值行（存量数据）等价于长度
// 为 1 的列表，向后兼容。
func splitResourceTypes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[string]bool, 2)
	out := make([]string, 0, 2)
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// validateResourceTypes 校验菜单声明的资源类型名列表（多值逗号分隔）：
// 每个类型名须匹配引擎注册类型名口径；返回拆分结果。空输入合法（纯菜单）。
func validateResourceTypes(raw string) ([]string, error) {
	types := splitResourceTypes(raw)
	for _, t := range types {
		if !menuResourceTypePattern.MatchString(t) {
			return nil, fmt.Errorf("resourceType %q 非法（须为小写字母开头的小写字母/数字/下划线，最长 64；多个用逗号分隔）", t)
		}
	}
	return types, nil
}

// validateButtonRelation 校验资源级按钮 relation：非空时必须命中引擎出口
// 白名单（fail-closed，防绕过 IaC 直接调 API 裸奔）。
func validateButtonRelation(relation string) error {
	if relation == "" {
		return nil
	}
	if !menuRelationWhitelist[relation] {
		return fmt.Errorf("relation %q 不在引擎出口白名单 {can_view, can_edit, can_delete, can_share, can_manage_grants} 内：新语义需先扩展引擎模型并重放", relation)
	}
	return nil
}

// menuChainDepth 计算菜单行在菜单树中的深度（顶级 = 1）：沿 parentID 向上
// 遍历计数（menus 为全量注册表快照），链上父行缺失返回错误（数据不一致防
// 御，正常由外层先做父行存在性校验）。三级菜单约束：菜单行新层级 = 父链
// 深度 + 1 ≤ 3。
func menuChainDepth(menus []repo.Menu, startID int64) (int, error) {
	byID := make(map[int64]repo.Menu, len(menus))
	for _, m := range menus {
		byID[m.ID] = m
	}
	depth := 0
	cur := startID
	for cur != 0 {
		m, ok := byID[cur]
		if !ok {
			return 0, fmt.Errorf("父菜单行不存在: %d", cur)
		}
		depth++
		cur = m.ParentID
	}
	return depth, nil
}
