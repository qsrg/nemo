// attributeslogic.go 属性管理业务逻辑（ABAC 方案 A：主体/资源属性 CRUD）。
//
// 与授权关系（relations）不同：属性是"主体是什么 / 资源是什么"的描述，
// 由 ABAC deny 策略在判定链叠加使用。写接口走管理面（AdminRole），
// 读取接口同面（authz-admin 维护页）；策略评估时由 logic 直接经
// svcCtx.AttributeRepo 读取，不经此层。
package logic

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"
)

// newAttrOwner 校验归属标识并归一（主体/资源统一入口）。
func newAttrOwner(owner string) (string, error) {
	o, err := repo.NormalizeOwner(owner)
	if err != nil {
		return "", kiterrors.BadRequest("AUTHZ_ATTR_INVALID", "owner 须为 user:/org: 主体或 类型:id 资源")
	}
	return o, nil
}

// setAttrs 覆盖式写一组属性（每个 key upsert）。
func setAttrs(ctx context.Context, svcCtx *svc.ServiceContext, owner string, attrs map[string]string) error {
	for k, v := range attrs {
		if err := svcCtx.AttributeRepo.Set(ctx, repo.Attribute{Owner: owner, AttrKey: k, AttrValue: v}); err != nil {
			logx.WithContext(ctx).Errorf("set attribute %s/%s: %v", owner, k, err)
			return kiterrors.New(503, "AUTHZ_ATTR_UNAVAILABLE", "属性存储写入失败")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 主体属性
// ---------------------------------------------------------------------------

// SetSubjectAttributesLogic 设置主体属性。
type SetSubjectAttributesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewSetSubjectAttributesLogic 构造主体属性设置逻辑。
func NewSetSubjectAttributesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetSubjectAttributesLogic {
	return &SetSubjectAttributesLogic{ctx: ctx, svcCtx: svcCtx}
}

// SetSubjectAttributes 设置主体属性（owner 校验 + 覆盖式 upsert）。
func (l *SetSubjectAttributesLogic) SetSubjectAttributes(req *types.SetSubjectAttributesReq) (*types.SetSubjectAttributesResp, error) {
	owner, err := newAttrOwner(req.Subject)
	if err != nil {
		return nil, err
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "set_subject_attrs",
		"attr:"+owner, req, func() error {
			return setAttrs(l.ctx, l.svcCtx, owner, req.Attrs)
		})
	if err != nil {
		return nil, err
	}
	return &types.SetSubjectAttributesResp{Subject: owner}, nil
}

// GetSubjectAttributesLogic 查询主体属性。
type GetSubjectAttributesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGetSubjectAttributesLogic 构造主体属性查询逻辑。
func NewGetSubjectAttributesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSubjectAttributesLogic {
	return &GetSubjectAttributesLogic{ctx: ctx, svcCtx: svcCtx}
}

// GetSubjectAttributes 查询主体属性（无属性返回空 map）。
func (l *GetSubjectAttributesLogic) GetSubjectAttributes(req *types.GetSubjectAttributesReq) (*types.GetSubjectAttributesResp, error) {
	owner, err := newAttrOwner(req.Subject)
	if err != nil {
		return nil, err
	}
	attrs, err := l.svcCtx.AttributeRepo.Get(l.ctx, owner)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("get attributes %s: %v", owner, err)
		return nil, kiterrors.New(503, "AUTHZ_ATTR_UNAVAILABLE", "属性存储读取失败")
	}
	return &types.GetSubjectAttributesResp{Subject: owner, Attrs: attrs}, nil
}

// DeleteSubjectAttributeLogic 删除主体单条属性。
type DeleteSubjectAttributeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteSubjectAttributeLogic 构造主体属性删除逻辑。
func NewDeleteSubjectAttributeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteSubjectAttributeLogic {
	return &DeleteSubjectAttributeLogic{ctx: ctx, svcCtx: svcCtx}
}

// DeleteSubjectAttribute 删除主体单条属性（幂等）。
func (l *DeleteSubjectAttributeLogic) DeleteSubjectAttribute(req *types.DeleteSubjectAttributeReq) error {
	owner, err := newAttrOwner(req.Subject)
	if err != nil {
		return err
	}
	if req.Key == "" {
		return kiterrors.BadRequest("AUTHZ_ATTR_INVALID", "key 不能为空")
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_subject_attr",
		"attr:"+owner+"#"+req.Key, req, func() error {
			if err := l.svcCtx.AttributeRepo.Delete(l.ctx, owner, req.Key); err != nil {
				logx.WithContext(l.ctx).Errorf("delete attribute %s/%s: %v", owner, req.Key, err)
				return kiterrors.New(503, "AUTHZ_ATTR_UNAVAILABLE", "属性存储删除失败")
			}
			return nil
		})
}

// ---------------------------------------------------------------------------
// 资源属性
// ---------------------------------------------------------------------------

// SetResourceAttributesLogic 设置资源属性。
type SetResourceAttributesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewSetResourceAttributesLogic 构造资源属性设置逻辑。
func NewSetResourceAttributesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetResourceAttributesLogic {
	return &SetResourceAttributesLogic{ctx: ctx, svcCtx: svcCtx}
}

// SetResourceAttributes 设置资源属性（owner 校验 + 覆盖式 upsert）。
func (l *SetResourceAttributesLogic) SetResourceAttributes(req *types.SetResourceAttributesReq) (*types.SetResourceAttributesResp, error) {
	owner, err := newAttrOwner(req.Resource)
	if err != nil {
		return nil, err
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "set_resource_attrs",
		"attr:"+owner, req, func() error {
			return setAttrs(l.ctx, l.svcCtx, owner, req.Attrs)
		})
	if err != nil {
		return nil, err
	}
	return &types.SetResourceAttributesResp{Resource: owner}, nil
}

// GetResourceAttributesLogic 查询资源属性。
type GetResourceAttributesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGetResourceAttributesLogic 构造资源属性查询逻辑。
func NewGetResourceAttributesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetResourceAttributesLogic {
	return &GetResourceAttributesLogic{ctx: ctx, svcCtx: svcCtx}
}

// GetResourceAttributes 查询资源属性（无属性返回空 map）。
func (l *GetResourceAttributesLogic) GetResourceAttributes(req *types.GetResourceAttributesReq) (*types.GetResourceAttributesResp, error) {
	owner, err := newAttrOwner(req.Resource)
	if err != nil {
		return nil, err
	}
	attrs, err := l.svcCtx.AttributeRepo.Get(l.ctx, owner)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("get attributes %s: %v", owner, err)
		return nil, kiterrors.New(503, "AUTHZ_ATTR_UNAVAILABLE", "属性存储读取失败")
	}
	return &types.GetResourceAttributesResp{Resource: owner, Attrs: attrs}, nil
}

// DeleteResourceAttributeLogic 删除资源单条属性。
type DeleteResourceAttributeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteResourceAttributeLogic 构造资源属性删除逻辑。
func NewDeleteResourceAttributeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteResourceAttributeLogic {
	return &DeleteResourceAttributeLogic{ctx: ctx, svcCtx: svcCtx}
}

// DeleteResourceAttribute 删除资源单条属性（幂等）。
func (l *DeleteResourceAttributeLogic) DeleteResourceAttribute(req *types.DeleteResourceAttributeReq) error {
	owner, err := newAttrOwner(req.Resource)
	if err != nil {
		return err
	}
	if req.Key == "" {
		return kiterrors.BadRequest("AUTHZ_ATTR_INVALID", "key 不能为空")
	}
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return err
	}
	return writeWithAudit(l.ctx, l.svcCtx, operator, "delete_resource_attr",
		"attr:"+owner+"#"+req.Key, req, func() error {
			if err := l.svcCtx.AttributeRepo.Delete(l.ctx, owner, req.Key); err != nil {
				logx.WithContext(l.ctx).Errorf("delete attribute %s/%s: %v", owner, req.Key, err)
				return kiterrors.New(503, "AUTHZ_ATTR_UNAVAILABLE", "属性存储删除失败")
			}
			return nil
		})
}
