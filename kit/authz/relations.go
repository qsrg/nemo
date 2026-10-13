// relations.go 扩展 Remote 的通用资源授权管理能力：把授权元组的读/写/删
// 转发给集中式 authz 服务（GET/POST/DELETE /v1/authz/relations）。
// 与 Check/ListObjects 同一远端、同一身份透传（ctx 内 JWT + X-Request-ID）。
//
// 用途：业务服务（如 kafka）维护"谁对某资源有哪些权限"的授权名单时，
// 无需内嵌引擎，直接经本组件操作 authz 侧统一收敛的 relations 接口——
// 权限校验（名单管理权）由 authz 服务在接口内完成，调用方自带用户 JWT 即可。
package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	kiterrors "nemo/kit/errors"
)

// Tuple 是通用授权元组（与 authz /v1/authz/relations 契约逐字一致）：
// User 形如 user:zhangsan / org:tech-frontend#member，Relation 为
// owner/editor/viewer/sharer 等共享语义关系，Object 为 type:id 形式。
type Tuple struct {
	User     string `json:"user"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

// WriteRelations 批量写授权元组：POST /v1/authz/relations，返回写入条数。
// 幂等语义由 authz 服务保证（重复写同元组不报错）。
func (r *Remote) WriteRelations(ctx context.Context, tuples []Tuple) (int, error) {
	var out struct {
		Written int `json:"written"`
	}
	in := struct {
		Tuples []Tuple `json:"tuples"`
	}{Tuples: tuples}
	if err := r.post(ctx, "/v1/authz/relations", in, &out); err != nil {
		return 0, err
	}
	return out.Written, nil
}

// ListRelations 读对象的授权元组：GET /v1/authz/relations?object=&relation=。
// relation 传空串表示列该对象全部元组（authz 侧 relation 为 optional）。
func (r *Remote) ListRelations(ctx context.Context, object, relation string) ([]Tuple, error) {
	path := "/v1/authz/relations?object=" + url.QueryEscape(object)
	if relation != "" {
		path += "&relation=" + url.QueryEscape(relation)
	}
	var out struct {
		Items []Tuple `json:"items"`
	}
	if err := r.get(ctx, path, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		return []Tuple{}, nil
	}
	return out.Items, nil
}

// DeleteRelation 删单条授权元组：DELETE /v1/authz/relations?object=&user=&relation=。
// 幂等（authz 侧撤销对称语义：不存在同样成功）。
func (r *Remote) DeleteRelation(ctx context.Context, object, user, relation string) error {
	path := "/v1/authz/relations?object=" + url.QueryEscape(object) +
		"&user=" + url.QueryEscape(user) +
		"&relation=" + url.QueryEscape(relation)
	return r.delete(ctx, path)
}

// get 执行单次 GET 调用（身份/request id 注入与错误归一化同 post）。
func (r *Remote) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("authz: 构造请求 %s: %w", path, err)
	}
	if token := r.bearer(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if id := kiterrors.RequestIDFrom(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("authz: 请求上游 %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("authz: 读取上游响应 %s: %w", path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return upstreamError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("authz: 解码上游响应 %s: %w", path, err)
		}
	}
	return nil
}

// delete 执行单次 DELETE 调用（身份/request id 注入与错误归一化同 post）。
func (r *Remote) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, r.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("authz: 构造请求 %s: %w", path, err)
	}
	if token := r.bearer(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if id := kiterrors.RequestIDFrom(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("authz: 请求上游 %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("authz: 读取上游响应 %s: %w", path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return upstreamError(resp.StatusCode, data)
	}
	return nil
}
