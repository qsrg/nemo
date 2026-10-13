// modelgen.go 是资源类型注册配置 → DSL 块的确定性模板生成器：
// 模板决定一切，不做自由 DSL——非法配置在 Render 入口即被拒绝，
// 不可能产出越权结构（越出固定模板的关系/出口组合）。
//
// 白名单设计（permission-design §6.3"注册配置驱动生成"）：
//   - 基础关系固定：org（挂组织树，租户隔离的结构保证）、owner、editor、viewer；
//   - 选配开关由 Actions 勾选驱动：勾选 can_share ⇒ 渲染 sharer 关系；
//     勾选 can_view ⇒ 渲染 member_view 关系（组织成员可见）；
//   - can_* 出口固定集：can_view / can_edit / can_delete / can_share，
//     Actions 只能是该集合的子集，业务代码只查出口不感知关系组合细节。
//
// 确定性：动作码先去重再按固定顺序渲染，同参数同输出（模型文件退化为
// 配置数据的准自动生成产物，从注册配置任意重生成皆安全）。
package engine

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// resourceTypeNamePattern 是资源类型名的合法形态：小写字母开头的小写
// 字母/数字/下划线（与 FGA type 名与模型 DSL 语法约束一致）。
var resourceTypeNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// reservedTypeNames 是禁止注册的类型名：model.fga 基础模型已占用的类型
// （合并时重名会导致模型非法，且语义上属于平台内建类型而非业务资源）。
var reservedTypeNames = map[string]bool{
	"user":          true,
	"org":           true,
	"platform_role": true,
	"order":         true,
	// topic 原为基础模型内建（Phase C v3），2026-09-16 迁移至注册表
	//（name=topic，见 model.fga 头注），不再保留，可正常注册。
}

// canExits 是 can_* 出口的固定白名单集与渲染顺序（index 即输出顺序）。
// can_manage_grants 2026-09-30 加入：名单管理权（改授权名单）是独立业务
// 授权边界——能编辑属性 ≠ 能改授权名单，必须有自己的出口，禁止后端散算。
var canExits = []string{"can_view", "can_edit", "can_delete", "can_share", "can_manage_grants"}

// ResourceType 是资源类型注册配置（Render 的输入）：
// Actions 为勾选的 can_* 出口集合（必须是固定白名单集的子集）。
type ResourceType struct {
	// Name 是资源类型名（如 report），作为 FGA type 名。
	Name string
	// Actions 是勾选的 can_* 出口清单（顺序无关，渲染前规范化）。
	Actions []string
}

// Render 按固定模板从注册配置生成该资源类型的 DSL 块（纯函数，无 IO）：
//   - 校验：类型名合法且非保留名；Actions 去重后必须是 can_* 固定白名单
//     的非空子集——任一不满足即报错，绝不产出部分渲染的块（fail-closed）；
//   - 输出与入参动作顺序无关（规范化后按固定顺序渲染），同参数同输出。
//
// 选配关系：勾选 can_share 才渲染 sharer（防越权转授：只有 owner 与组织
// 管理员可共享）；勾选 can_view 才渲染 member_view（组织成员可见，机密
// 资源可不配）。viewer 固定接受 [user, org#member]（组织可直接授查看）；
// editor 同样接受 [user, org#member]（2026-09-23：组织可按资源批量授"可
// 编辑"，支撑跨团队隔离——张三团队只编辑自己的 topic）；can_delete 固定
// 为 owner（2026-09-30 起与 can_edit 拆分：删除是独立授权边界，可编辑者
// 不再自动可删）；can_manage_grants 固定为 owner（名单管理权，编辑不放大
// 为名单管理权，防提权）；can_view 固定含 owner。
// owner 接受 [user, org#member]（2026-10-09：部门可被授"可管理"档——部门
// 成员整体继承 owner 全权；此前仅用户主体，授权对话框勾部门+可管理会被
// 引擎以类型限制拒绝）。
func Render(r ResourceType) (string, error) {
	if !resourceTypeNamePattern.MatchString(r.Name) {
		return "", fmt.Errorf("资源类型名 %q 非法（须为小写字母开头的小写字母/数字/下划线）", r.Name)
	}
	if reservedTypeNames[r.Name] {
		return "", fmt.Errorf("资源类型名 %q 为平台保留类型名，禁止注册", r.Name)
	}

	// 动作规范化：去重 + 固定顺序，保证同参数同输出。
	chosen := map[string]bool{}
	for _, a := range r.Actions {
		if !isKnownExit(a) {
			return "", fmt.Errorf("动作码 %q 不在 can_* 出口固定白名单 %v 内", a, canExits)
		}
		chosen[a] = true
	}
	if len(chosen) == 0 {
		return "", errors.New("动作码不能为空（至少勾选一个 can_* 出口）")
	}

	// 固定模板渲染：基础关系 + 选配关系 + 勾选的 can_* 出口。
	var b strings.Builder
	b.WriteString("type " + r.Name + "\n  relations\n")
	b.WriteString("    define org: [org]\n")
	b.WriteString("    define owner: [user, org#member]\n")
	if chosen["can_share"] {
		// sharer 是 can_share 的中间关系（隐含契约）：勾选 can_share 必然渲染
		// sharer、勾选 can_view 必然渲染 member_view（下方），二者无独立开关
		// ——有意为之，勿当缺陷修（终审 Task6① 注释钉死）。
		b.WriteString("    define sharer: [user] or owner or admin from org\n")
	}
	b.WriteString("    define editor: [user, org#member] or owner\n")
	b.WriteString("    define viewer: [user, org#member] or editor\n")
	if chosen["can_view"] {
		b.WriteString("    define member_view: viewer or member from org\n")
	}
	for _, e := range canExits {
		if !chosen[e] {
			continue
		}
		switch e {
		case "can_view":
			// member_view or owner：owner 必可看自己的资源（与内置 topic 语义对齐）。
			b.WriteString("    define can_view: member_view or owner\n")
		case "can_edit":
			b.WriteString("    define can_edit: editor\n")
		case "can_delete":
			// owner：删除是比编辑更严的独立授权边界（2026-09-30 拆分）。
			// 此前 can_delete=owner or editor 与 can_edit 等价（editor 已含
			// owner），等于没有独立删除档；现收口为仅 owner 可删。
			b.WriteString("    define can_delete: owner\n")
		case "can_manage_grants":
			// owner：名单管理权收口 owner（与后端 canManageGrants 同源）——
			// 编辑属性不放大为授权名单管理权，防提权（沿用原注释语义）。
			b.WriteString("    define can_manage_grants: owner\n")
		case "can_share":
			b.WriteString("    define can_share: sharer\n")
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// isKnownExit 判定动作码是否在 can_* 出口固定白名单内。
func isKnownExit(a string) bool {
	for _, e := range canExits {
		if a == e {
			return true
		}
	}
	return false
}

// ExtractActions 从已渲染的 DSL 块反解 can_* 出口清单（按模板固定顺序
// can_view/can_edit/can_delete/can_share），供资源类型列表接口从注册表
// DslBlock 回填 Actions 字段（注册表不另存动作列）。
func ExtractActions(dslBlock string) []string {
	var out []string
	for _, e := range canExits {
		// 行首四空格 + define <出口>: 为模板的出口定义形态。
		if strings.Contains(dslBlock, "    define "+e+":") {
			out = append(out, e)
		}
	}
	return out
}
