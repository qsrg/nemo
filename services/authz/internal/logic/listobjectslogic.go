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

// ListObjectsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListObjectsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListObjectsLogic 构造业务规则层实例。
func NewListObjectsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListObjectsLogic {
	return &ListObjectsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListObjects 列出 user 拥有指定关系的对象 id 集合（permission-design §5.3：
// 列表过滤用它反向展开，禁止逐行 Check）：
//   - user 一律取 JWT claims.sub（身份缺失 401 fail-closed）；
//   - Type/Relation 为全 optional 契约下的必填自校验（空 → 400）；
//   - 与 Check 同判定语义：无可见对象 = 200 空数组（非 null），不作为错误；
//   - 引擎 error（不可达、未注册资源类型等）→ 503 fail-closed：
//     AUTHZ_ENGINE_ERROR（"权限判定服务异常"）。状态码说明与未注册类型
//     决策同 CheckLogic.Check（kiterrors.Internal 实际为 500，按全局约束
//     显式构造 503；未注册类型经实测引擎返回 error，不与真实故障区分，
//     一律拒绝 fail-closed）。
func (l *ListObjectsLogic) ListObjects(req *types.ListObjectsReq) (*types.ListObjectsResp, error) {
	sub, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	objType := strings.TrimSpace(req.Type)
	relation := strings.TrimSpace(req.Relation)
	if objType == "" || relation == "" {
		return nil, errors.BadRequest("AUTHZ_LIST_OBJECTS_INVALID", "type 与 relation 不能为空")
	}

	objects, err := l.svcCtx.Engine.ListObjects(l.ctx, "user:"+sub, relation, objType)
	if err != nil {
		l.Logger.Errorf("引擎对象列表失败 type=%s relation=%s: %v", objType, relation, err)
		return nil, errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
	}
	if objects == nil {
		objects = []string{}
	}
	// ABAC 叠加（方案 A，通用判定面，与 CheckLogic 同口径）：deny 命中或
	// allow 门槛零命中的对象从列表剔除——列表可见性与逐条判定一致
	//（resourceType 留空的策略对任意对象生效，配置时注意作用域；
	// 存储故障 fail-open：剔除失败的对象保留在列表中，安全兜底依赖
	// 逐条 Check 的 fail-closed 判定）。对象按 "type:id" 取 type 段。
	filtered := make([]string, 0, len(objects))
	for _, o := range objects {
		if i := strings.IndexByte(o, ':'); i > 0 {
			if abacBlocked(l.ctx, l.svcCtx, o[:i], o[i+1:], sub) {
				continue
			}
		}
		filtered = append(filtered, o)
	}
	return &types.ListObjectsResp{Objects: filtered}, nil
}
