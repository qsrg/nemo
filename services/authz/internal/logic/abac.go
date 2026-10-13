// abac.go ABAC 策略评估（方案 A：deny 拒绝 + allow 正向门槛，叠加到资源鉴权链路）。
//
// 链路（RBAC → ReBAC → ABAC）：
//  1. RBAC：菜单码（read/manage）控页面入口与操作面；
//  2. ReBAC：资源关系（owner/viewer/editor + 部门继承）控资源归属；
//  3. ABAC：属性策略叠加在上述结果之上——
//     - deny（拒绝）：命中即拒绝，优先于一切**资源关系授权**（viewer/editor/
//       owner 判定通过的路径，含 Check/ListObjects/BatchCheck 判定面与名单
//       管理路径 grantsManageable）；
//     - allow（正向门槛）：对存在适用 allow 策略的资源，须至少命中一条
//     才放行——表达"只有 trade 业务线的人可看"这类正向属性门槛：
//     属性不满足、乃至没配属性的主体一律被挡在门槛外（缺属性 ≠ 放行）。
//
// 豁免边界（层级模型）：页面级管理码（如 kafka-topics:manage）是平台管理员
// 的显式全权，与资源级授权不同层级——业务服务侧的管理码回退链/全权取页
// 路径豁免本策略（ABAC 数据在 authz 侧，业务侧无法低成本过滤；管理码授予
// 本身由管理员显式操作）。如需对管理码也施加属性约束，须将回退链收口到
// 判定面扩展（后续工作，不在本文件范围）。
//
// 适用规则（一条策略是否作用于某次判定）：
//   - enabled 且 resourceType 为空或等于判定的资源类型；
//   - allow 策略另有资源侧适用性：字面量形式（无 resourceAttr）对该类型
//     全部资源生效（整型门槛）；resourceAttr 形式仅对**设置了该属性**的
//     资源生效（资源未声明该属性 = 未上门槛，不受约束）——支持按资源
//     粒度开门槛，如仅 kafka_topic:1 设 allowedBizLine=trade 时只有它受限。
//
// 求值顺序：先 deny（命中即拒绝，无视 allow 命中），再 allow 门槛
// （存在适用策略且零命中 → 拒绝）。属性/策略缺省时行为不变（默认全 disabled）。
//
// 通用接入（abacBlocked 是唯一评估入口）：
//   - 通用判定面 /v1/authz/check（checklogic）：任意资源类型的 Check 自动
//     叠加——后续业务域经 kit client 走通用 Check 即生效，无需逐处接线；
//   - Topic 演示域：列表可见性过滤（快照批量）与 topicCanEdit（单点）。
//
// fail-open 说明：属性/策略存储故障按放行处理（返回不拦截）——deny/allow
// 是叠加约束而非授权本身，存储故障不额外拦既有授权（与 Check 引擎故障的
// fail-closed 区分：引擎给不出判定必须拒，属性表故障不应误杀全部资源）。
package logic

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// abacView 是一次判定的策略快照：全部策略 + 主体属性（批量场景下多个
// 资源共用一次加载，逐资源只查资源属性）。nil 视图（无启用策略或加载
// 失败 fail-open）恒不拦截。
type abacView struct {
	policies []repo.Policy
	subAttrs map[string]string
}

// abacLoad 拉取策略快照 + 主体属性（批量评估复用；存储故障 fail-open 返回 nil）。
func abacLoad(ctx context.Context, svcCtx *svc.ServiceContext, sub string) *abacView {
	policies, err := svcCtx.PolicyRepo.List(ctx)
	if err != nil {
		logx.WithContext(ctx).Errorf("ABAC 策略读取失败 sub=%s: %v", sub, err)
		return nil
	}
	// 快速路径：无启用策略直接放行（策略评估零开销）。
	active := false
	for _, p := range policies {
		if p.Enabled {
			active = true
			break
		}
	}
	if !active {
		return nil
	}
	subAttrs, err := svcCtx.AttributeRepo.Get(ctx, "user:"+sub)
	if err != nil {
		logx.WithContext(ctx).Errorf("ABAC 主体属性读取失败 sub=%s: %v", sub, err)
		return nil
	}
	return &abacView{policies: policies, subAttrs: subAttrs}
}

// blocked 评估快照对某资源是否拦截（deny 命中，或 allow 门槛适用且零命中）。
// 资源属性读取失败按 fail-open（不拦截）。
func (v *abacView) blocked(ctx context.Context, svcCtx *svc.ServiceContext, resourceType, objectID string) bool {
	if v == nil {
		return false
	}
	resAttrs, err := svcCtx.AttributeRepo.Get(ctx, resourceType+":"+objectID)
	if err != nil {
		logx.WithContext(ctx).Errorf("ABAC 资源属性读取失败 %s:%s: %v", resourceType, objectID, err)
		return false
	}
	allowApplicable, allowHit := false, false
	for _, p := range v.policies {
		if !p.Enabled || (p.ResourceType != "" && p.ResourceType != resourceType) {
			continue
		}
		switch repo.EffectOf(p.Effect) {
		case repo.EffectDeny:
			if repo.EvaluatePolicy(p, v.subAttrs, resAttrs) {
				logx.WithContext(ctx).Infof("ABAC deny 命中 policy=%s(%d) %s:%s",
					p.Name, p.ID, resourceType, objectID)
				return true
			}
		case repo.EffectAllow:
			// resourceAttr 形式的门槛仅对设置了该属性的资源适用；
			// 字面量形式对该类型全部资源适用（整型门槛）。
			if p.ResourceAttr != "" {
				if _, ok := resAttrs[p.ResourceAttr]; !ok {
					continue
				}
			}
			allowApplicable = true
			if repo.EvaluatePolicy(p, v.subAttrs, resAttrs) {
				allowHit = true
			}
		}
	}
	if allowApplicable && !allowHit {
		logx.WithContext(ctx).Infof("ABAC allow 门槛拦截 resource=%s:%s", resourceType, objectID)
		return true
	}
	return false
}

// abacBlocked 单资源判定入口（快照加载 + 评估，true = ABAC 拦截）。
func abacBlocked(ctx context.Context, svcCtx *svc.ServiceContext, resourceType, objectID, sub string) bool {
	return abacLoad(ctx, svcCtx, sub).blocked(ctx, svcCtx, resourceType, objectID)
}
