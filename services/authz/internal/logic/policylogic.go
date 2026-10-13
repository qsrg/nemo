// policylogic.go ABAC 策略管理业务逻辑（方案 A：deny/allow 策略 CRUD）。
//
// 策略是叠加在 ReBAC 授权之上的属性约束（deny 命中即拒绝；allow 是
// 正向门槛——适用资源须命中至少一条才放行，语义见 abac.go），不替代
// 资源关系。写接口走管理面（AdminRole）；评估侧（logic/abac.go）直接经
// PolicyRepo + AttributeRepo 读取，不经此层。
package logic

import (
	"context"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"
)

// policyFromReq 把请求体归一为 Policy 实体（公共校验：effect 白名单、
// op 白名单、subjectAttr 非空、比较侧 resourceAttr/value 至少一方有值）。
func policyFromReq(name, resourceType, effect, subjectAttr, op, resourceAttr, value string, enabled bool) (repo.Policy, error) {
	name = strings.TrimSpace(name)
	subjectAttr = strings.TrimSpace(subjectAttr)
	op = strings.TrimSpace(op)
	resourceAttr = strings.TrimSpace(resourceAttr)
	value = strings.TrimSpace(value)
	if name == "" {
		return repo.Policy{}, kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "策略名不能为空")
	}
	if subjectAttr == "" {
		return repo.Policy{}, kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "subjectAttr 不能为空")
	}
	if !repo.ValidPolicyOp(op) {
		return repo.Policy{}, kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "op 仅支持 lt/lte/gt/gte/eq/neq")
	}
	if !repo.ValidPolicyEffect(effect) {
		return repo.Policy{}, kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "effect 仅支持 deny/allow")
	}
	if resourceAttr == "" && value == "" {
		return repo.Policy{}, kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "resourceAttr 与 value 至少填写一项")
	}
	return repo.Policy{
		Name:         name,
		ResourceType: strings.TrimSpace(resourceType),
		Effect:       repo.EffectOf(effect),
		SubjectAttr:  subjectAttr,
		Op:           repo.PolicyOp(op),
		ResourceAttr: resourceAttr,
		Value:        value,
		Enabled:      enabled,
	}, nil
}

// policyItemFrom 实体 → API 响应（id 转字符串）。
func policyItemFrom(p repo.Policy) types.PolicyItem {
	return types.PolicyItem{
		Id:           strconv.FormatInt(p.ID, 10),
		Name:         p.Name,
		ResourceType: p.ResourceType,
		Effect:       p.Effect,
		SubjectAttr:  p.SubjectAttr,
		Op:           string(p.Op),
		ResourceAttr: p.ResourceAttr,
		Value:        p.Value,
		Enabled:      p.Enabled,
	}
}

// CreatePolicyLogic 新建策略。
type CreatePolicyLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreatePolicyLogic 构造新建策略逻辑。
func NewCreatePolicyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePolicyLogic {
	return &CreatePolicyLogic{ctx: ctx, svcCtx: svcCtx}
}

// CreatePolicy 新建策略（默认 disabled：确认后再启用，避免误拦截）。
func (l *CreatePolicyLogic) CreatePolicy(req *types.CreatePolicyReq) (*types.CreatePolicyResp, error) {
	p, err := policyFromReq(req.Name, req.ResourceType, req.Effect, req.SubjectAttr,
		req.Op, req.ResourceAttr, req.Value, req.Enabled)
	if err != nil {
		return nil, err
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	var created repo.Policy
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_policy",
		"policy:"+p.Name, req, func() error {
			var werr error
			created, werr = l.svcCtx.PolicyRepo.Create(l.ctx, p)
			return werr
		})
	if err != nil {
		return nil, err
	}
	return &types.CreatePolicyResp{Policy: policyItemFrom(created)}, nil
}

// ListPoliciesLogic 策略列表。
type ListPoliciesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListPoliciesLogic 构造策略列表逻辑。
func NewListPoliciesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPoliciesLogic {
	return &ListPoliciesLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListPolicies 返回全部策略（按 ID 升序）。
func (l *ListPoliciesLogic) ListPolicies() (*types.ListPoliciesResp, error) {
	items, err := l.svcCtx.PolicyRepo.List(l.ctx)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("list policies: %v", err)
		return nil, kiterrors.New(503, "AUTHZ_POLICY_UNAVAILABLE", "策略存储读取失败")
	}
	out := make([]types.PolicyItem, 0, len(items))
	for _, p := range items {
		out = append(out, policyItemFrom(p))
	}
	return &types.ListPoliciesResp{Items: out, Total: len(out)}, nil
}

// UpdatePolicyLogic 更新策略。
type UpdatePolicyLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdatePolicyLogic 构造更新策略逻辑。
func NewUpdatePolicyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePolicyLogic {
	return &UpdatePolicyLogic{ctx: ctx, svcCtx: svcCtx}
}

// UpdatePolicy 更新策略（ID 定位，其余字段全量覆盖；未命中 404）。
func (l *UpdatePolicyLogic) UpdatePolicy(req *types.UpdatePolicyReq) (*types.CreatePolicyResp, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(req.Id), 10, 64)
	if err != nil {
		return nil, kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "策略 ID 非法")
	}
	p, err := policyFromReq(req.Name, req.ResourceType, req.Effect, req.SubjectAttr,
		req.Op, req.ResourceAttr, req.Value, req.Enabled)
	if err != nil {
		return nil, err
	}
	p.ID = id
	// 前置存在性检查：404 必须在 writeWithAudit 之前返回（writeWithAudit 只认存储故障）。
	if _, gerr := l.svcCtx.PolicyRepo.Get(l.ctx, id); gerr != nil {
		if gerr == repo.ErrNotFound {
			return nil, kiterrors.NotFound("AUTHZ_POLICY_NOT_FOUND", "策略不存在")
		}
		logx.WithContext(l.ctx).Errorf("get policy %d: %v", id, gerr)
		return nil, kiterrors.New(503, "AUTHZ_POLICY_UNAVAILABLE", "策略存储读取失败")
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	var updated repo.Policy
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "update_policy",
		"policy:"+strconv.FormatInt(id, 10), req, func() error {
			var werr error
			updated, werr = l.svcCtx.PolicyRepo.Update(l.ctx, p)
			return werr
		})
	if err != nil {
		return nil, err
	}
	return &types.CreatePolicyResp{Policy: policyItemFrom(updated)}, nil
}

// DeletePolicyLogic 删除策略。
type DeletePolicyLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeletePolicyLogic 构造删除策略逻辑。
func NewDeletePolicyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePolicyLogic {
	return &DeletePolicyLogic{ctx: ctx, svcCtx: svcCtx}
}

// DeletePolicy 删除策略（未命中 404）。
func (l *DeletePolicyLogic) DeletePolicy(req *types.PolicyIdReq) error {
	id, err := strconv.ParseInt(strings.TrimSpace(req.Id), 10, 64)
	if err != nil {
		return kiterrors.BadRequest("AUTHZ_POLICY_INVALID", "策略 ID 非法")
	}
	if _, gerr := l.svcCtx.PolicyRepo.Get(l.ctx, id); gerr != nil {
		if gerr == repo.ErrNotFound {
			return kiterrors.NotFound("AUTHZ_POLICY_NOT_FOUND", "策略不存在")
		}
		logx.WithContext(l.ctx).Errorf("get policy %d: %v", id, gerr)
		return kiterrors.New(503, "AUTHZ_POLICY_UNAVAILABLE", "策略存储读取失败")
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_policy",
		"policy:"+strconv.FormatInt(id, 10), req, func() error {
			return l.svcCtx.PolicyRepo.Delete(l.ctx, id)
		})
}
