// Package casdir 是 CAS 目录查询客户端：拉取组织架构全量（GET /orgs）与
// 用户目录全量（GET /users 翻页），供组织同步对账（internal/orgsync）消费。
//
// 生产语义：组织架构由 CAS 系统统一维护并对外提供查询（本系统不做组织
// 事实源，只做同步副本落授权引擎）。接口契约与本地 mock CAS 容器一致
// （cas-test，见 mock 数据目录 users.json / cas_server.py）：
//   - GET /orgs                → {"items":[{id,name,parentId,sort}], "total":N}
//   - GET /users?page&page_size → {"items":[{username,...,orgId,orgName}], "total":N}
package casdir

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// orgsPageSize 是 ListAllUsers 翻页大小（mock CAS 服务端上限 100）。
const usersPageSize = 100

// maxUserPages 是翻页轮数上限：100 页 × 100 条 = 1 万用户封顶，防御服务端
// total 异常（如恒不变）导致的无界翻页。
const maxUserPages = 100

// maxBodyBytes 是响应体读取上限（1MB）：防上游异常超大响应耗尽内存
// （与 auth 服务 cas 客户端同约定）。
const maxBodyBytes = 1 << 20

// Org 是 CAS 组织架构节点：id 为稳定语义编码（对应生产"部门编码"属性，
// 部门改名不变），parentId 为父节点 id（根为空串）。
type Org struct {
	// ID 组织 ID。
	ID string `json:"id"`
	// Name 组织展示名。
	Name string `json:"name"`
	// ParentID 父组织 ID，根组织为空。
	ParentID string `json:"parentId"`
	// Sort 同级展示序（同步透传，本系统暂不消费）。
	Sort int `json:"sort"`
}

// DirUser 是 CAS 目录用户中与组织同步相关的投影：username 即授权引擎里
// user:{username} 的主体，orgId 是其组织归属。
type DirUser struct {
	// Username 用户名（idp_subject 口径）。
	Username string `json:"username"`
	// DisplayName 展示名（告警日志可读性用）。
	DisplayName string `json:"displayName"`
	// OrgID 所属组织 ID（同步写 org:{orgID}#member@user:{username}）。
	OrgID string `json:"orgId"`
}

// Client 是 CAS 目录查询客户端（无状态，可并发复用）。
type Client struct {
	// baseURL 是 CAS 服务基地址（尾斜杠已剔除）。
	baseURL string
	// hc 承载固定超时的底层 HTTP 客户端。
	hc *http.Client
}

// NewClient 构造目录客户端。insecureSkipVerify 为 true 时跳过 TLS 证书校验
// （生产 CAS https 自签场景；mock 为明文 HTTP 时该值无影响）。
func NewClient(baseURL string, insecureSkipVerify bool, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	transport := http.DefaultTransport
	if insecureSkipVerify {
		transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	return &Client{
		baseURL: trimTrailingSlash(baseURL),
		hc:      &http.Client{Timeout: timeout, Transport: transport},
	}
}

// ListOrgs 拉取组织架构全量（mock 接口无分页，一次返回全部节点）。
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	var resp struct {
		Items []Org `json:"items"`
		Total int   `json:"total"`
	}
	if err := c.getJSON(ctx, "/orgs", nil, &resp); err != nil {
		return nil, fmt.Errorf("拉取组织架构: %w", err)
	}
	return resp.Items, nil
}

// ListAllUsers 翻页拉取用户目录全量（/users 服务端按 username 排序，翻页稳定）；
// 返回总条数与 total 不一致时以实际取到的为准（防御 total 异常）。
func (c *Client) ListAllUsers(ctx context.Context) ([]DirUser, error) {
	var all []DirUser
	for page := 1; page <= maxUserPages; page++ {
		var resp struct {
			Items []DirUser `json:"items"`
			Total int       `json:"total"`
		}
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(usersPageSize))
		if err := c.getJSON(ctx, "/users", q, &resp); err != nil {
			return nil, fmt.Errorf("拉取用户目录(page=%d): %w", page, err)
		}
		all = append(all, resp.Items...)
		if len(all) >= resp.Total || len(resp.Items) == 0 {
			return all, nil
		}
	}
	return all, fmt.Errorf("用户目录翻页超过上限 %d 页（total 异常？）", maxUserPages)
}

// getJSON 发起 GET 并把 JSON 响应解码进 out；非 2xx 读出错误体转为 error。
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("构造请求: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("请求 %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("读取响应: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s 返回 %d: %s", path, resp.StatusCode, truncate(body, 200))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析响应: %w", err)
	}
	return nil
}

// trimTrailingSlash 剔除尾斜杠（空串原样返回）。
func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// truncate 截断字节串用于错误信息展示。
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
