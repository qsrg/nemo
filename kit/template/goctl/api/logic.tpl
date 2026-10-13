// Code scaffolded by goctl. Safe to edit.
// goctl {{.version}}
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package {{.pkgName}}

import (
	{{.imports}}
)

// {{.logic}} 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type {{.logic}} struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

{{if .hasDoc}}{{.doc}}{{end}}
// New{{.logic}} 构造业务规则层实例。
func New{{.logic}}(ctx context.Context, svcCtx *svc.ServiceContext) *{{.logic}} {
	return &{{.logic}}{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// {{.function}} 执行业务逻辑：单测重点；错误用 kit/errors 语义构造器返回。
func (l *{{.logic}}) {{.function}}({{.request}}) {{.responseType}} {
	// todo: add your logic here and delete this line

	{{.returnString}}
}
