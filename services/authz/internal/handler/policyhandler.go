// policyhandler.go ABAC 策略路由处理器（方案 A）：仅做协议转换。
// 手写合并区（与 attributeshandler.go 同理由）：四个端点共享同一份样板。
package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"nemo/services/authz/internal/logic"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"
)

// CreatePolicyHandler 新建策略。
func CreatePolicyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreatePolicyReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewCreatePolicyLogic(r.Context(), svcCtx)
		resp, err := l.CreatePolicy(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ListPoliciesHandler 策略列表。
func ListPoliciesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewListPoliciesLogic(r.Context(), svcCtx)
		resp, err := l.ListPolicies()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// UpdatePolicyHandler 更新策略。
func UpdatePolicyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdatePolicyReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewUpdatePolicyLogic(r.Context(), svcCtx)
		resp, err := l.UpdatePolicy(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// DeletePolicyHandler 删除策略。
func DeletePolicyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PolicyIdReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewDeletePolicyLogic(r.Context(), svcCtx)
		if err := l.DeletePolicy(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, nil)
	}
}
