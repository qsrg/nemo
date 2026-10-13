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

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
)

// WriteRelationsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type WriteRelationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewWriteRelationsLogic 构造业务规则层实例。
func NewWriteRelationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WriteRelationsLogic {
	return &WriteRelationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// writableRelations 是 WriteRelations 的关系白名单：业务资源的共享语义关系
// （与 modelgen 模板渲染的基础关系一致）。can_* 出口关系由模型派生、管理面
// 关系（org 的 admin/member/parent）只能走既有管理 API——这是接口语义而非
// 权限问题，超管调用同样受此约束。
// org 关系（资源挂部门，user 形如 org:{id}）：归属挂接 = 部门成员可见性授予，
// 与给 org#member 授 viewer 等价——写权限由 manageGranted 按资源类型把关
// （注册类型 = 名单管理码/owner/editor），不额外收紧。
var writableRelations = map[string]bool{
	"owner":  true,
	"editor": true,
	"viewer": true,
	"sharer": true,
	"org":    true,
}

// reservedObjectTypes 是平台保留对象类型（modelgen reservedTypeNames 同源）：
// 一律禁止经判定面写接口触碰（防终端用户自授 platform_role assignee 等越权，
// 终审 C1 护栏）。org 关系挂接、角色授予分别由管理 API 与后续服务凭证机制承担。
var reservedObjectTypes = map[string]bool{
	"user":          true,
	"org":           true,
	"platform_role": true,
}

// writableObject 校验单个元组的对象类型可写：
//   - 保留类型（user/org/platform_role）→ false；
//   - 基础模型内建业务类型 order → true（model.fga 定义，非注册表管理）；
//   - 其余业务类型须已注册（ResourceTypeRepo 命中）→ 注册表查询失败即 false。
//     （kafka_topic 原内置 type topic，2026-09-16 迁注册表后走注册表查询。）
func (l *WriteRelationsLogic) writableObject(objType string) bool {
	if reservedObjectTypes[objType] {
		return false
	}
	if objType == "order" {
		return true
	}
	_, err := l.svcCtx.ResourceTypeRepo.Get(l.ctx, objType)
	return err == nil
}

// manageGranted 校验调用者对元组对象的管理权（2026-10-11 收敛：kafka_topic
// 专用分支并入通用注册策略，全注册类型同一套 owner-or-管理码口径——不含
// editor，编辑属性不放大为名单管理权，防自授 owner 提权）：
//   - 创建引导放行（owner/org + 空对象 + create/manage 码）先于类型分支；
//   - order → 原语义（基础模型业务类型，业务服务批量写，不校验资源管理权）；
//   - 其余（已注册类型，writableObject 已保证）→ 注册配置驱动的通用名单
//     策略（grantpolicy.go）：名单管理码 or owner + ABAC 叠加——新资源
//     类型接入零改码，注册时声明 ManageCode 即得管理权校验。
// manageGranted 校验调用者对某对象某关系的名单管理权。relation 参与创建
// 引导放行的判定（仅 owner/org 两类创建期元组）。
//
// 放行分支（创建路径自举，2026-10-03）：**空对象首写**——对象当前无任何
// owner 元组、且本次写 relation 为 owner 或 org（kafka 创建的部门挂接与
// owner 同批），调用者持有该类型注册的 create 码或 manage 码 → 放行。
// 语义："创建者即 owner"的业务事实由调用方服务的创建守卫（create/manage
// 码）背书，引擎侧仅认码；放行仍受三道白名单与审计约束。防预占由调用方
// 服务承担：创建闭包在元组写失败时回滚业务行，预占元组永不指向真实资源。
//
// 其余对象照旧：order → 放行；注册类型 → 名单管理码 or owner（不含 editor）。
func (l *WriteRelationsLogic) manageGranted(operator, object, relation string) error {
	objType, _, ok := strings.Cut(object, ":")
	if !ok || objType == "" {
		return errors.BadRequest("AUTHZ_RELATION_INVALID", "object 须为 type:id 形式")
	}
	// 创建引导放行（先于类型分支，kafka/redis/zk 的创建路径统一受益）：
	// owner/org + 空对象（**无任何元组**——若只查 owner，"有 viewer 无 owner"
	// 的窗口期对象（半写补对账前）会被 create 码持有者抢注 owner，越权）
	// + create/manage 码。
	if relation == "owner" || relation == "org" {
		existing, err := l.svcCtx.Engine.ReadTuples(l.ctx, object, "")
		if err != nil {
			logx.WithContext(l.ctx).Errorf("空对象判定失败 object=%s: %v", object, err)
			return topicEngineErr()
		}
		if len(existing) == 0 {
			policy, perr := grantPolicyFor(l.ctx, l.svcCtx, objType)
			if perr != nil {
				logx.WithContext(l.ctx).Errorf("读资源类型名单策略失败 %s: %v", objType, perr)
				return topicEngineErr()
			}
			if policy != nil {
				for _, code := range []string{policy.manageCode, policy.createCode} {
					if code == "" {
						continue
					}
					if ok, gerr := menuManageGranted(l.ctx, l.svcCtx.Engine, operator, code); gerr != nil {
						logx.WithContext(l.ctx).Errorf("创建引导码判定失败 %s: %v", code, gerr)
						return topicEngineErr()
					} else if ok {
						return nil
					}
				}
			}
		}
	}
	switch objType {
	case "order":
		return nil
	}
	policy, err := grantPolicyFor(l.ctx, l.svcCtx, objType)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("读资源类型名单策略失败 %s: %v", objType, err)
		return topicEngineErr()
	}
	// writableObject 已保证非 order/topic 必已注册；此处 nil 属异常路径，
	// fail-closed 拒绝（不静默放行）。
	if policy == nil {
		return topicDenied()
	}
	if ok, err := grantsManageable(l.ctx, policy, l.svcCtx, operator, object); err != nil {
		logx.WithContext(l.ctx).Errorf("名单管理权判定失败 object=%s: %v", object, err)
		return topicEngineErr()
	} else if !ok {
		return topicDenied()
	}
	return nil
}

// WriteRelations 业务服务批量写关系元组（permission-design §6.2 写入语义）：
//   - operator=JWT claims.sub（身份缺失 401 fail-closed）；
//   - 调用方服务名取请求头 X-Service-Name（types 契约 header 绑定，生成物
//     httpx.Parse 注入 Source 字段），缺省 "unknown"，只落审计快照不参业务；
//   - 幂等：engine.WriteTuples 读后写过滤，重复提交不报错不重复落引擎；
//   - 校验（写路径前置，失败不落审计）：tuples 非空且每条三要素齐全 → 400；
//   - 写白名单（终审 C1 护栏，防终端用户自授管理面关系/平台角色）：逐条校验
//     对象类型与关系——保留类型（user/org/platform_role）或关系不在
//     owner/editor/viewer/sharer 白名单、或业务类型未注册 → 403
//     AUTHZ_RELATION_FORBIDDEN（对全体调用者生效，含超管：管理面关系只能走
//     既有管理 API，这是接口语义）；
//   - 审计先行：writeWithAudit 封装 CreatePending → WriteTuples →
//     MarkCommitted；引擎错误 → MarkFailed + 503 AUTHZ_SERVICE_UNAVAILABLE。
//   - Written 返回请求元组数（引擎侧幂等去重，重复条目不产生新数据）。
func (l *WriteRelationsLogic) WriteRelations(req *types.WriteRelationsReq) (*types.WriteRelationsResp, error) {
	operator, _, err := identityFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Tuples) == 0 {
		return nil, errors.BadRequest("AUTHZ_TUPLES_INVALID", "tuples 不能为空")
	}
	tuples := make([]engine.Tuple, 0, len(req.Tuples))
	for _, t := range req.Tuples {
		tuple := engine.Tuple{
			User:     strings.TrimSpace(t.User),
			Relation: strings.TrimSpace(t.Relation),
			Object:   strings.TrimSpace(t.Object),
		}
		if tuple.User == "" || tuple.Relation == "" || tuple.Object == "" {
			return nil, errors.BadRequest("AUTHZ_TUPLES_INVALID", "元组 user/relation/object 不能为空")
		}
		// 写白名单：对象类型取 object 冒号前缀，关系/类型双重校验；
		// 任一条不合规整批拒绝（写路径前置，不落审计）。
		objType, _, _ := strings.Cut(tuple.Object, ":")
		if objType == "" || !writableRelations[tuple.Relation] || !l.writableObject(objType) {
			return nil, errors.Forbidden("AUTHZ_RELATION_FORBIDDEN", "不允许写该关系")
		}
		// 资源管理权校验（通用 relations 收敛后）：topic 对象要求
		// owner/editor/manage 码，防任意登录用户自授名单越权。
		if err := l.manageGranted(operator, tuple.Object, tuple.Relation); err != nil {
			return nil, err
		}
		tuples = append(tuples, tuple)
	}

	// service 取 X-Service-Name 头（缺省 unknown），随快照落审计供溯源。
	service := strings.TrimSpace(req.Source)
	if service == "" {
		service = "unknown"
	}
	snapshot := map[string]any{"service": service, "tuples": req.Tuples}
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "write_relations", service, snapshot, func() error {
		return l.svcCtx.Engine.WriteTuples(l.ctx, tuples)
	})
	if err != nil {
		return nil, err
	}

	return &types.WriteRelationsResp{Written: len(tuples)}, nil
}
