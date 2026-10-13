// attributeshandler.go 属性管理路由处理器（ABAC 方案 A）：
// 仅做协议转换（参数绑定、调 logic、写响应），业务规则在 logic 层。
// 手写合并区：六个端点（主体/资源 × 设置/查询/删除）共享同一份样板，
// 按 goctl 一 handler 一文件的惯例拆开收益低，故合并于此文件。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/authz/internal/logic"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"
)

// SetSubjectAttributesHandler 设置主体属性。
func SetSubjectAttributesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SetSubjectAttributesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewSetSubjectAttributesLogic(r.Context(), svcCtx)
		resp, err := l.SetSubjectAttributes(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// GetSubjectAttributesHandler 查询主体属性。
func GetSubjectAttributesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetSubjectAttributesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewGetSubjectAttributesLogic(r.Context(), svcCtx)
		resp, err := l.GetSubjectAttributes(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// DeleteSubjectAttributeHandler 删除主体单条属性。
func DeleteSubjectAttributeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteSubjectAttributeReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeleteSubjectAttributeLogic(r.Context(), svcCtx)
		if err := l.DeleteSubjectAttribute(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, nil)
	}
}

// SetResourceAttributesHandler 设置资源属性。
func SetResourceAttributesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SetResourceAttributesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewSetResourceAttributesLogic(r.Context(), svcCtx)
		resp, err := l.SetResourceAttributes(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// GetResourceAttributesHandler 查询资源属性。
func GetResourceAttributesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetResourceAttributesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewGetResourceAttributesLogic(r.Context(), svcCtx)
		resp, err := l.GetResourceAttributes(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// DeleteResourceAttributeHandler 删除资源单条属性。
func DeleteResourceAttributeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteResourceAttributeReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeleteResourceAttributeLogic(r.Context(), svcCtx)
		if err := l.DeleteResourceAttribute(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, nil)
	}
}
