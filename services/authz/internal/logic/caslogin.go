// caslogin.go 承载 CAS 登录跳转 URL 的拼装（逻辑主体；生成的 casloginlogic.go
// 仅薄委托到此，handler 以 302 重写下发——契约上 login 无返回体）。
package logic

import (
	"net/url"
	"strings"

	"nemo/services/authz/internal/config"
)

// buildCasLoginURL 拼 CAS 登录页跳转 URL：{BaseURL}/login?service={service}。
// service 由调用方指定（前端接票页地址，如 https://console.example.com/cas-ticket），
// 须与 callback 请求携带的 service 一致（CAS 校验票据与 service 匹配）；
// 仅接受 http/https 合法 URL，空/非法时回退配置默认值（Cas.Service，本服务
// callback 地址——兼容未传 service 的旧调用方）。query 经标准编码。
func buildCasLoginURL(c config.CasConf, service string) string {
	if !validService(service) {
		service = c.Service
	}
	base := strings.TrimSuffix(c.BaseURL, "/")
	return base + "/login?" + url.Values{"service": {service}}.Encode()
}

// validService 校验调用方指定的 service：非空且为 http/https 绝对 URL。
// 仅做协议白名单（防 javascript: 等注入到重定向地址）；域名允许范围由
// CAS 服务端的 service 白名单把守，本层不重复维护。
func validService(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
