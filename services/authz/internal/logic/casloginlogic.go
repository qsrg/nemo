// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
	"nemo/services/authz/internal/svc"
)

// CasLoginLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CasLoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCasLoginLogic 构造业务规则层实例。
func NewCasLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CasLoginLogic {
	return &CasLoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CasLogin 执行业务逻辑：拼 CAS 登录页跳转 URL 返回给 handler（302 由
// handler 层 http.Redirect 下发，契约上 login 无返回体）；service 取调用方
// query 参数（前端接票页地址），拼装见 caslogin.go。
func (l *CasLoginLogic) CasLogin(service string) (string, error) {
	return buildCasLoginURL(l.svcCtx.Config.Cas, service), nil
}
