// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2
//
// 约定：logic 是业务规则层、单测重点；错误一律用 kit/errors 语义构造器
// （BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）返回，
// 由全局错误出口统一转为约定的 JSON 错误体。

package logic

import (
	"context"
	"strings"

	"nemo/kit/errors"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CheckLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CheckLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCheckLogic 构造业务规则层实例。
func NewCheckLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckLogic {
	return &CheckLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Check 资源级权限判定（permission-design §5.3）：
//   - user 一律取 JWT claims.sub（authzkit.IdentityFrom；身份缺失 401 fail-closed）；
//   - Object/Relation 为全 optional 契约下的必填自校验（空 → 400）；
//   - 判定语义（简报逐字）：明确无权 = 200 {allowed:false}，不作为错误返回——
//     业务层 403 由 kit client 或前端转译，服务端不把"无权"当错误；
//   - 引擎 error（含不可达、未注册资源类型等）→ 503 fail-closed：
//     AUTHZ_ENGINE_ERROR（"权限判定服务异常"）。状态码说明：简报写的是
//     kiterrors.Internal，但经核对 kit/errors/constructors.go 该构造器实际
//     输出 500 而非 503；按全局约束"引擎不可达/内部错 503 fail-closed"，
//     此处用 errors.New 显式构造 503 + AUTHZ_ENGINE_ERROR，语义不变。
//     不区分"未注册类型"与"引擎故障"：经实测 OpenFGA 对模型中不存在的类型
//     返回 error（而非明确 false），可靠区分需对错误文案做脆弱的字符串匹配，
//     误判风险大于收益——两类都按"无法给出权威判定 → 拒绝"处理，均 fail-closed。
func (l *CheckLogic) Check(req *types.CheckReq) (*types.CheckResp, error) {
	sub, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	object := strings.TrimSpace(req.Object)
	relation := strings.TrimSpace(req.Relation)
	if object == "" || relation == "" {
		return nil, errors.BadRequest("AUTHZ_CHECK_INVALID", "object 与 relation 不能为空")
	}

	allowed, err := l.svcCtx.Engine.Check(l.ctx, "user:"+sub, relation, object)
	if err != nil {
		l.Logger.Errorf("引擎判定失败 object=%s relation=%s: %v", object, relation, err)
		return nil, errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
	}
	// ABAC 叠加（方案 A，通用判定面）：ReBAC 判定通过后过属性策略——
	// deny 命中或 allow 门槛零命中即 allowed=false（存储故障 fail-open）。
	// 对象按 "type:id" 取 type 段为资源类型；后续业务域经本接口判定即
	// 自动获得 ABAC 叠加，无需逐处接线（deny/allow 语义见 abac.go）。
	// 注意：resourceType 留空的策略对任意对象生效，配置时注意作用域。
	if allowed {
		if i := strings.IndexByte(object, ':'); i > 0 {
			allowed = !abacBlocked(l.ctx, l.svcCtx, object[:i], object[i+1:], sub)
		}
	}
	// 明确 false 同样 200 返回，不报错。
	return &types.CheckResp{Allowed: allowed}, nil
}
