// Package cas 实现 CAS 协议客户端：用 service ticket 向 CAS 服务端换取
// 登录用户信息（serviceValidate），供 callback 编排做用户映射。
// 协议口径（M6 联调探测修正）：XML 解析按 CAS 2.0/3.0 兼容的局部名——本地
// 测试容器实为 CAS 2.0 明文 /serviceValidate 端点；若启用 3.0 p3 路径
// （/cas/p3/serviceValidate），attributes/name/email 的回落策略需复核。
package casclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	kiterrors "nemo/kit/errors"
)

// defaultValidatePath 是 ValidatePath 未配置时的兜底（CAS 标准路径）。
const defaultValidatePath = "/serviceValidate"

// casCallTimeout 是单次票据校验调用的固定超时（CAS 不可达快速失败）。
const casCallTimeout = 5 * time.Second

// maxBodyBytes 是响应体读取上限（1MB）：防上游异常超大响应耗尽内存
// （authzclient 同约定）。
const maxBodyBytes = 1 << 20

// CasUser 是 CAS 换回的登录用户信息。
type CasUser struct {
	// Username CAS 用户名（身份主体，映射与建档的唯一依据）。
	Username string
	// Name 展示名（attributes 缺失时回落 Username）。
	Name string
	// Email 邮箱（attributes 缺失时回落 Username）。
	Email string
	// OrgID 用户所属组织（CAS 目录组织架构信息；登录后用于同步组织成员）。
	OrgID string
	// OrgName 组织展示名（与 OrgID 同源，展示/排查用）。
	OrgName string
}

// Client 是 CAS 协议客户端（版本口径见包注释：本地 2.0 端点，3.0 兼容解析）。
type Client struct {
	// baseURL 是 CAS 服务基地址（尾斜杠已剔除）。
	baseURL string
	// validatePath 是票据校验路径（如 /cas/p3/serviceValidate）。
	validatePath string
	// hc 承载固定超时的底层 HTTP 客户端（dev 自签场景跳过 TLS 校验）。
	hc *http.Client
}

// NewClient 构造 CAS 客户端。insecureSkipVerify 为 true 时跳过 TLS 证书校验，
// 仅用于 dev 环境 CAS https 自签证书场景（生产 CAS 必须用可信证书，置 false）。
func NewClient(baseURL, validatePath string, insecureSkipVerify bool) *Client {
	if validatePath == "" {
		validatePath = defaultValidatePath
	}
	hc := &http.Client{Timeout: casCallTimeout}
	if insecureSkipVerify {
		// 仅 dev：本地 CAS 容器自签证书，无法用系统信任链校验。
		hc.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev 自签场景，见函数注释
		}
	}
	return &Client{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		validatePath: validatePath,
		hc:           hc,
	}
}

// casServiceResponse 是 CAS 3.0 serviceValidate 响应的最小 XML 结构
// （encoding/xml 按局部名匹配，兼容 cas: 等命名空间前缀）。
type casServiceResponse struct {
	// Success 非 nil 表示 authenticationSuccess 分支。
	Success *struct {
		User       string `xml:"user"`
		Attributes struct {
			// Name/Email 用切片承接：CAS attributes 元素可能多值（部署差异），
			// 解析取首个（见 firstOf）。
			Name    []string `xml:"name"`
			Email   []string `xml:"email"`
			OrgID   []string `xml:"orgId"`
			OrgName []string `xml:"orgName"`
		} `xml:"attributes"`
	} `xml:"authenticationSuccess"`
	// Failure 非 nil 表示 authenticationFailure 分支（票据无效等）。
	Failure *struct {
		Code    string `xml:"code,attr"`
		Message string `xml:",chardata"`
	} `xml:"authenticationFailure"`
}

// ValidateTicket 用 ST 票据向 CAS 换取用户信息：GET
// {BaseURL}{ValidatePath}?ticket=&service=，解析 CAS 3.0 XML。
// 错误语义二分（callback 编排依赖）：
//   - 网络错误 / 上游非 200 / 报文不可解析 → 503 AUTH_CAS_UNAVAILABLE（服务故障）；
//   - authenticationFailure / 无 success 分支 → 401 AUTH_CAS_TICKET_INVALID（票据无效）。
func (c *Client) ValidateTicket(ctx context.Context, ticket, service string) (CasUser, error) {
	u := c.baseURL + c.validatePath + "?" + url.Values{
		"ticket":  {ticket},
		"service": {service},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return CasUser{}, unavailable(fmt.Errorf("构造校验请求: %w", err))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return CasUser{}, unavailable(fmt.Errorf("请求 CAS: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CasUser{}, unavailable(fmt.Errorf("CAS 响应 %d", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return CasUser{}, unavailable(fmt.Errorf("读取 CAS 响应: %w", err))
	}

	var sr casServiceResponse
	if err := xml.Unmarshal(data, &sr); err != nil {
		// 报文不可解析属服务端异常，与票据无效区分（503）。
		return CasUser{}, unavailable(fmt.Errorf("解析 CAS 响应: %w", err))
	}
	// 无 success 分支（含 failure 分支）一律视为票据校验失败（401）。
	if sr.Success == nil || sr.Success.User == "" {
		return CasUser{}, kiterrors.Unauthorized("AUTH_CAS_TICKET_INVALID", "CAS ticket 校验失败")
	}
	return CasUser{
		Username: sr.Success.User,
		Name:     firstOf(sr.Success.Attributes.Name, sr.Success.User),
		Email:    firstOf(sr.Success.Attributes.Email, sr.Success.User),
		OrgID:    firstOf(sr.Success.Attributes.OrgID, ""),
		OrgName:  firstOf(sr.Success.Attributes.OrgName, ""),
	}, nil
}

// CasDirectoryUser 是 CAS 用户目录 /users 接口返回的单条用户信息。
// 生产语义：CAS 是用户信息与组织归属的权威源，响应永不含 password。
type CasDirectoryUser struct {
	// Username 用户名（= auth 侧 IdpSubject 口径）。
	Username string `json:"username"`
	// DisplayName 展示名（CAS 权威；登录换票的 name 同源）。
	DisplayName string `json:"displayName"`
	// Mail 邮箱。
	Mail string `json:"mail"`
	// Department 部门展示串（目录展示用，非授权组织结构）。
	Department string `json:"department"`
	// Phone 电话。
	Phone string `json:"phone"`
	// OrgID 用户所属组织（authz 组织 id，组织成员同步/继承判定的事实源）。
	OrgID string `json:"orgId"`
	// OrgName 组织展示名。
	OrgName string `json:"orgName"`
}

// CasDirectoryResp 是 CAS /users 的响应结构。
type CasDirectoryResp struct {
	// Items 命中用户（精确批量按请求顺序；分页场景按 username 升序）。
	Items []CasDirectoryUser `json:"items"`
	// Total 命中总数（分页场景为过滤后全量）。
	Total int `json:"total"`
}

// QueryUsers 实时查询 CAS 用户目录（GET {BaseURL}/users）：
//   - usernames 非空：精确批量（逗号分隔，保持请求顺序输出）；
//   - 否则 keyword 模糊（username/displayName/mail 子串）+ page/pageSize 分页。
//
// 供用户目录查询（管理面选人/姓名反查）以 CAS 为事实源。错误语义与
// ValidateTicket 一致：网络错误/非 200/报文不可解析 → 503 AUTH_CAS_UNAVAILABLE。
func (c *Client) QueryUsers(ctx context.Context, usernames []string, keyword string, page, pageSize int) (CasDirectoryResp, error) {
	q := url.Values{}
	if len(usernames) > 0 {
		q.Set("usernames", strings.Join(usernames, ","))
	}
	if keyword != "" {
		q.Set("keyword", keyword)
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	u := c.baseURL + "/users"
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return CasDirectoryResp{}, unavailable(fmt.Errorf("构造用户目录请求: %w", err))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return CasDirectoryResp{}, unavailable(fmt.Errorf("请求 CAS 用户目录: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CasDirectoryResp{}, unavailable(fmt.Errorf("CAS 用户目录响应 %d", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return CasDirectoryResp{}, unavailable(fmt.Errorf("读取 CAS 用户目录响应: %w", err))
	}
	var out CasDirectoryResp
	if err := json.Unmarshal(data, &out); err != nil {
		return CasDirectoryResp{}, unavailable(fmt.Errorf("解析 CAS 用户目录响应: %w", err))
	}
	return out, nil
}

// unavailable 归一化 CAS 服务故障为 503 语义错误：原始 err 记入日志（保留
// 排障线索），不进响应体（上游内部细节不外泄）。
func unavailable(err error) error {
	logx.Errorf("CAS 票据校验调用失败: %v", err)
	return kiterrors.New(503, "AUTH_CAS_UNAVAILABLE", "CAS 服务不可用")
}

// firstOf 取多值属性的首个值（CAS attributes 多值元素场景）；空则回落 def。
func firstOf(values []string, def string) string {
	if len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}
	return def
}
