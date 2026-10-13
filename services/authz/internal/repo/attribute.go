// attribute.go 定义鉴权属性存储（ABAC 方案 A：属性统一由 authz 引擎管理）。
//
// 背景：旧 required_level 是 topic 表硬编码的 ABAC 等级门槛（规则写死、不可
// 复用）。方案 A 将「主体属性 / 资源属性」统一收口到 authz 属性存储，配合
// ABAC deny 策略（见 policy.go）做统一评估——业务表不再承载鉴权门槛字段，
// 新资源类型复用同一套能力。
//
// 存储模型（KISS：一张表，owner 前缀区分主体/资源）：
//   - 主体属性：owner 形如 user:zhangsan / org:tech-frontend#member；
//   - 资源属性：owner 形如 kafka_topic:1（资源类型:对象 id）；
//   - 属性值为字符串（数字存字符串，策略求值时按需转换）。
package repo

import (
	"context"
	"strings"
	"time"
)

// Attribute 一条属性（owner 维度下的 key-value）。
type Attribute struct {
	// Owner 属性归属：主体 user:/org:，或资源 类型:id。
	Owner string
	// AttrKey 属性键（如 securityLevel、bizLine）。
	AttrKey string
	// AttrValue 属性值（字符串；数字比较时策略层转数值）。
	AttrValue string
	// CreatedAt/UpdatedAt 时间戳（实现层填充）。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AttributeRepo 是属性存储的存取接口（内存/MySQL 双实现，共享断言组）。
type AttributeRepo interface {
	// Set 写入单条属性（幂等 upsert：同 owner+key 覆盖 value）。
	Set(ctx context.Context, a Attribute) error
	// Get 返回某 owner 的全部属性（key → value）；无属性返回空 map。
	Get(ctx context.Context, owner string) (map[string]string, error)
	// Delete 删除某 owner 的单条属性（幂等：不存在不报错）。
	Delete(ctx context.Context, owner, key string) error
}

// NormalizeOwner 校验并归一属性归属标识：
//   - subject：必须以 user: 或 org: 开头（org 允许 #member 聚合后缀）；
//   - resource：必须含 ":"（类型:id），且类型非 user/org（避免与主体混淆）。
//
// 返回错误时由 logic 层转 400。
func NormalizeOwner(owner string) (string, error) {
	o := strings.TrimSpace(owner)
	if o == "" {
		return "", ErrInvalidOwner
	}
	lower := strings.ToLower(o)
	if strings.HasPrefix(lower, "user:") || strings.HasPrefix(lower, "org:") {
		// 主体形式：冒号后必须有非空 id（user: 空主体非法）。
		if strings.TrimSpace(o[strings.Index(o, ":")+1:]) == "" {
			return "", ErrInvalidOwner
		}
		return o, nil
	}
	idx := strings.Index(o, ":")
	if idx <= 0 || idx == len(o)-1 {
		return "", ErrInvalidOwner
	}
	kind := strings.ToLower(o[:idx])
	if kind == "user" || kind == "org" {
		return "", ErrInvalidOwner
	}
	return o, nil
}
