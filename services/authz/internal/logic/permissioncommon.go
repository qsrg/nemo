// permissioncommon.go 承载权限点授权面（/v1/admin/permissions/*）的公共小函数：
// 权限码合法性校验、授予主体归一化（user / org#member）、直接授予元组读取、
// 用户所在组织枚举。全部为 logic 包内小函数，供各 permission logic 复用。
package logic

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// permissionCodePattern 是权限码的合法形态：`服务:动作` 两段（如 redis:read），
// 服务段允许通配 `*`（如 *:read 全部服务只读）；服务段小写字母开头的
// 小写字母/数字/连字符（与菜单 service_key 同族），动作段小写字母开头的
// 小写字母/数字/下划线。对象 id = 权限码，完整对象 permission:{code}。
var permissionCodePattern = regexp.MustCompile(`^(\*|[a-z][a-z0-9-]*):[a-z][a-z0-9_-]*$`)

// validPermissionCode 判定权限码是否合法（前后空白不允许，交由调用方先 Trim）。
func validPermissionCode(code string) bool { return permissionCodePattern.MatchString(code) }

// grantSubject 归一化授予主体（去角色化改造的元组形态）：
//   - user 目标 → "user:{id}"（裸用户名即 JWT sub）；
//   - org 目标 → "org:{id}#member"（userset 主体，组织成员含 parent 链继承
//     均获得授权；组织存在性由调用方前置校验）。
//
// targetType 非法返回错误（调用方前置校验后不会命中）。
func grantSubject(targetType, targetID string) (string, error) {
	switch targetType {
	case "user":
		return "user:" + targetID, nil
	case "org":
		return "org:" + targetID + "#member", nil
	default:
		return "", fmt.Errorf("非法 targetType %q", targetType)
	}
}

// grantTarget 是审计条目的 target 形态：user:{id} / org:{id}（不含 #member
// userset 后缀，保持与组织/成员动作的 target 口径一致）。
func grantTarget(targetType, targetID string) string {
	return targetType + ":" + targetID
}

// permObject 权限码 → 引擎对象。OpenFGA 对象 id 不允许包含冒号（"type:id"
// 单冒号约束，实测 v1.19 报 invalid object field format），故权限码的冒号
// 在对象 id 侧替换为点：redis:read ↔ permission:redis.read（码内恰一冒号，
// 服务名/动作均不含点，映射为双射；通配码 *:read 同理 → permission:*.read）。
func permObject(code string) string {
	return "permission:" + strings.Replace(code, ":", ".", 1)
}

// permCode 引擎对象 → 权限码（permObject 的逆映射）。
func permCode(object string) string {
	id := strings.TrimPrefix(object, "permission:")
	return strings.Replace(id, ".", ":", 1)
}

// grantTuples 构造一批 can 授权元组：逐码 permission:{code}#can@{subject}。
func grantTuples(subject string, codes []string) []engine.Tuple {
	tuples := make([]engine.Tuple, 0, len(codes))
	for _, code := range codes {
		tuples = append(tuples, engine.Tuple{
			User:     subject,
			Relation: "can",
			Object:   permObject(code),
		})
	}
	return tuples
}

// codesFromCanTuples 从 can 元组列表中筛出 subject 主体直接授予的权限码
// （升序去重）：主体形如 user:{id} / org:{id}#member，对象形如 permission:{code}。
func codesFromCanTuples(tuples []engine.Tuple, subject string) []string {
	set := make(map[string]bool)
	for _, t := range tuples {
		if t.Relation != "can" || t.User != subject {
			continue
		}
		code := permCode(t.Object)
		if code != "" {
			set[code] = true
		}
	}
	codes := make([]string, 0, len(set))
	for c := range set {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

// permCodesOf 主体（user:{id} 或 org:{id}#member userset）经引擎判定的有效
// 权限码（ListObjects 展开组织继承）。引擎此版本（v1.19）Read 不支持 user
// 维度过滤（实测报 object type required），主体反查一律走 ListObjects。
func permCodesOf(ctx context.Context, sc *svc.ServiceContext, subject string) ([]string, error) {
	objs, err := sc.Engine.ListObjects(ctx, subject, "can", "permission")
	if err != nil {
		return nil, fmt.Errorf("ListObjects(can): %w", err)
	}
	codes := make([]string, 0, len(objs))
	for _, o := range objs {
		if c := permCode(o); c != "" {
			codes = append(codes, c)
		}
	}
	sort.Strings(codes)
	return codes, nil
}

// directGrantCodes 主体直接授予的权限码（挂在该主体上的 can 元组，不含任何
// 继承展开——继承来源的展示走 permissionoverview 的 viaOrgs，各自只报本部门
// 直接挂的码）。org/user 两种主体统一按元组过滤（全量 Read 后应用层匹配）。
// 不能用 ListObjects——它返回的是"主体经判定的有效权限"（模型 v5 下含全部
// 祖先部门的授予）：org 主体会把继承码误报为直接授予，user 主体的差集算法
// 会把与部门授予重叠的直接码剔除出直接授予（两个方向的失真都出现过）。
func directGrantCodes(ctx context.Context, sc *svc.ServiceContext, subject string) ([]string, error) {
	if strings.HasPrefix(subject, "org:") {
		// Read 无 user 维度过滤（v1.19 实测报 object type required），全量读后
		// 应用层过滤；引擎元组量级 = 授权关系数，规模可控。
		tuples, err := sc.Engine.ReadTuples(ctx, "", "")
		if err != nil {
			return nil, fmt.Errorf("ReadTuples(全量): %w", err)
		}
		codes := make([]string, 0)
		seen := make(map[string]bool)
		for _, t := range tuples {
			if t.Relation == "can" && t.User == subject && strings.HasPrefix(t.Object, "permission:") {
				if c := permCode(t.Object); c != "" && !seen[c] {
					seen[c] = true
					codes = append(codes, c)
				}
			}
		}
		sort.Strings(codes)
		return codes, nil
	}
	// user 主体：同样按元组过滤（忠实直接授予）。此前实现是"有效权限 − 组织
	// 来源"的差集——与部门授予重叠的直接码会被剔除出直接授予（如张三直接持
	// 有 kafka-topics:read、root 部门也授了 read，直接列表里就不显示），用户
	// 看到的是"明明直接授了却不在直接授予里"的分裂。直接授予就该忠实反映
	// 元组；与部门来源的重叠在总览里两处如实显示。
	tuples, err := sc.Engine.ReadTuples(ctx, "", "")
	if err != nil {
		return nil, fmt.Errorf("ReadTuples(全量): %w", err)
	}
	codes := make([]string, 0)
	seen := make(map[string]bool)
	for _, t := range tuples {
		if t.Relation == "can" && t.User == subject && strings.HasPrefix(t.Object, "permission:") {
			if c := permCode(t.Object); c != "" && !seen[c] {
				seen[c] = true
				codes = append(codes, c)
			}
		}
	}
	sort.Strings(codes)
	return codes, nil
}

// userOrgIDs 枚举用户直接所在的组织 ID（org.member 元组按 user 反查；
// 含 parent 链的继承成员关系由引擎判定面承担，此处只列直接归属）。
func userOrgIDs(ctx context.Context, sc *svc.ServiceContext, userID string) ([]string, error) {
	// 用户所属组织（org.member 元组反查）：此版本 Read 无 user 过滤（实测），
	// 走 ListObjects(user, member, org)——含经 parent 链继承的上级组织
	objs, err := sc.Engine.ListObjects(ctx, "user:"+userID, "member", "org")
	if err != nil {
		return nil, fmt.Errorf("ListObjects(member): %w", err)
	}
	ids := make([]string, 0, len(objs))
	for _, o := range objs {
		if id := strings.TrimPrefix(o, "org:"); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// ---------------------------------------------------------------------------
// 菜单停用/改码/删除的孤儿授权防护：人员授权矩阵只展示启用菜单，离开
// 矩阵管理的权限码若不清授予元组，会变成不可见也不可撤销的存量授权。
// ---------------------------------------------------------------------------

// menuPermCodes 菜单关联的权限码全集：read 码（PermissionCode）+ 派生
// manage 码（base:manage，与授权矩阵 Grants.vue 的码推导同构）。
func menuPermCodes(m repo.Menu) []string {
	base := strings.SplitN(m.PermissionCode, ":", 2)[0]
	codes := []string{m.PermissionCode}
	if base != "" && base != "*" {
		codes = append(codes, base+":manage")
	}
	return codes
}

// revokeOrphanedMenuCodes 清理孤儿权限码的授予元组（菜单停用/改码/删除时
// 调用）：candidates 为该菜单变更前后关联的码全集，凡未被其他启用菜单
// 使用、也不在 keep（变更后仍启用的码集）中的码，其 permission:{code}#can
// 元组一并删除（幂等，返回清理条数）。skipID 排除正在变更的菜单行自身。
// 引擎读写失败按错误透传（调用方经 writeWithAudit 转 503 fail-closed）。
func revokeOrphanedMenuCodes(ctx context.Context, sc *svc.ServiceContext, skipID int64, keep map[string]bool, candidates []string) (int, error) {
	others, err := sc.MenuRepo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("列菜单注册表: %w", err)
	}
	referenced := make(map[string]bool)
	for _, m := range others {
		if !m.Enabled || m.ID == skipID {
			continue
		}
		for _, c := range menuPermCodes(m) {
			referenced[c] = true
		}
	}
	n := 0
	for _, code := range candidates {
		if code == "" || keep[code] || referenced[code] {
			continue
		}
		tuples, err := sc.Engine.ReadTuples(ctx, permObject(code), "can")
		if err != nil {
			return n, fmt.Errorf("读 %s 授予元组: %w", code, err)
		}
		if len(tuples) == 0 {
			continue
		}
		if err := sc.Engine.DeleteTuples(ctx, tuples); err != nil {
			return n, fmt.Errorf("清 %s 授予元组: %w", code, err)
		}
		n += len(tuples)
	}
	return n, nil
}
