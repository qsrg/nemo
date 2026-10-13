// policy.go 定义 ABAC 策略（方案 A：属性 + 策略统一收口 authz 引擎）。
//
// 定位：策略是叠加在 ReBAC 授权之上的属性约束，不替代资源关系：
//   - effect=deny（拒绝）：条件命中即拒绝，优先于一切授权；
//   - effect=allow（正向门槛）：对适用该策略的资源，须至少命中一条
//     allow 策略才可访问——表达"只有 trade 业务线的人可看"这类正向
//     属性门槛；没配属性/属性不满足的主体一律被挡在门槛外。
//
// 当前只做二元条件（无表达式引擎，KISS）：
//   - 条件形式一：subjectAttr op resourceAttr（主体属性 vs 资源属性），
//     如 user.securityLevel < resource.securityLevel（复现旧 required_level）；
//   - 条件形式二：subjectAttr op literal（主体属性 vs 字面量），
//     如 user.bizLine == "trade"。
// op 限定 lt/lte/gt/gte/eq/neq（数值或字符串 eq/neq，数值比较自动转 float64）。
//
// effect 与 enabled 的过滤由评估层（logic/abac.go）承担；EvaluatePolicy
// 只负责"这组属性下条件是否成立"的纯求值。
//
// 缺属性语义：条件涉及的属性缺失视为条件不成立（不命中）——deny 是
// "有证据证明违反才拒绝"；allow 门槛则相反地"没证据通过就挡下"，
// 两者的放行/拦截不对称性由评估层组合实现。
package repo

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// PolicyOp 支持的比较操作符。
type PolicyOp string

// 策略比较操作符（限白名单，避免表达式注入/复杂度失控）。
const (
	OpLt  PolicyOp = "lt"
	OpLte PolicyOp = "lte"
	OpGt  PolicyOp = "gt"
	OpGte PolicyOp = "gte"
	OpEq  PolicyOp = "eq"
	OpNeq PolicyOp = "neq"
)

// EffectDeny / EffectAllow 策略效果（评估语义见 logic/abac.go）。
const (
	EffectDeny  = "deny"
	EffectAllow = "allow"
)

// EffectOf 归一化效果取值（大小写不敏感；空与其他非法值归默认 deny，
// 写入口在 policylogic 侧前置校验，此处兜底只保证评估面拿到合法值）。
func EffectOf(e string) string {
	if strings.ToLower(strings.TrimSpace(e)) == EffectAllow {
		return EffectAllow
	}
	return EffectDeny
}

// ValidPolicyEffect 校验效果取值（空串合法——logic 层归一为默认 deny）。
func ValidPolicyEffect(e string) bool {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case "", EffectDeny, EffectAllow:
		return true
	}
	return false
}

// Policy 一条 ABAC 策略。
type Policy struct {
	// ID 自增主键（落库时生成）。
	ID int64
	// Name 策略名（展示与审计用）。
	Name string
	// ResourceType 目标资源类型（如 kafka_topic）；空 = 全部资源类型。
	ResourceType string
	// Effect 策略效果：deny（拒绝）| allow（正向门槛）。
	Effect string
	// SubjectAttr 主体属性键（如 securityLevel、bizLine）。
	SubjectAttr string
	// Op 比较操作符（PolicyOp 白名单）。
	Op PolicyOp
	// ResourceAttr 资源属性键（与 Value 二选一；非空优先）。
	ResourceAttr string
	// Value 字面量（ResourceAttr 为空时与 SubjectAttr 比较）。
	Value string
	// Enabled 策略开关：false 一律不评估（回滚开关）。
	Enabled bool
	// CreatedAt/UpdatedAt 时间戳（实现层填充）。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PolicyRepo 是策略表的存取接口（内存/MySQL 双实现，共享断言组）。
type PolicyRepo interface {
	// Create 新建策略（ID=0 由实现生成），返回落库后记录。
	Create(ctx context.Context, p Policy) (Policy, error)
	// List 返回全部策略（按 ID 升序；评估侧自行过滤 Enabled+ResourceType）。
	List(ctx context.Context) ([]Policy, error)
	// Get 按 ID 查单条；未命中返回 ErrNotFound（管理面更新/删除前置存在性检查用）。
	Get(ctx context.Context, id int64) (Policy, error)
	// Update 按 ID 全量覆盖（ID 不变）；未命中返回 ErrNotFound。
	Update(ctx context.Context, p Policy) (Policy, error)
	// Delete 按 ID 删除；未命中返回 ErrNotFound。
	Delete(ctx context.Context, id int64) error
}

// ValidPolicyOp 校验操作符在白名单内。
func ValidPolicyOp(op string) bool {
	switch PolicyOp(op) {
	case OpLt, OpLte, OpGt, OpGte, OpEq, OpNeq:
		return true
	}
	return false
}

// EvaluatePolicy 求值单条策略的条件是否命中（纯求值：effect/enabled 的
// 过滤由评估层承担，本函数不关心——条件命中 ≠ 触发拦截）。
//
// 规则：
//   - 缺属性（subject 或比较侧）→ 不命中；
//   - 数值比较：两侧都转 float64，转换失败 → 不命中（非数值仅允许 eq/neq
//     做字符串比较）；
//   - 资源属性键与字面量二选一（ResourceAttr 非空优先）。
func EvaluatePolicy(p Policy, subAttrs, resAttrs map[string]string) bool {
	left, ok := subAttrs[p.SubjectAttr]
	if !ok {
		return false
	}
	var right string
	if p.ResourceAttr != "" {
		right, ok = resAttrs[p.ResourceAttr]
		if !ok {
			return false
		}
	} else {
		right = p.Value
	}

	// 数值优先：两侧都能转 float 才做数值比较（old required_level 语义）。
	lf, lerr := strconv.ParseFloat(strings.TrimSpace(left), 64)
	rf, rerr := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if lerr == nil && rerr == nil {
		switch p.Op {
		case OpLt:
			return lf < rf
		case OpLte:
			return lf <= rf
		case OpGt:
			return lf > rf
		case OpGte:
			return lf >= rf
		case OpEq:
			return lf == rf
		case OpNeq:
			return lf != rf
		}
		return false
	}
	// 非数值：仅 eq/neq 按字符串比较。
	switch p.Op {
	case OpEq:
		return left == right
	case OpNeq:
		return left != right
	}
	return false
}
