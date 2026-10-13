// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 独立服务改造：判定与授权名单读写全部经 kit/authz 转发集中式 authz。

package logic

import (
	"context"
	stderrors "errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// orgIDPattern 是归属组织 ID 的格式白名单：org id 会拼进 "org:<id>" 引擎
// 元组串，白名单拦截空格/#/@ 等元组语法字符的注入（CAS 组织标识同风格，
// 如 tech-frontend）。
var orgIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)

// CreateKafkaTopicLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateKafkaTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateKafkaTopicLogic 构造业务规则层实例。
func NewCreateKafkaTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateKafkaTopicLogic {
	return &CreateKafkaTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateKafkaTopic 新建 Topic（新建权判定已上收路由守卫，业务代码只关注业务）：
// name 必填（400）、重名 409（DB 唯一键兜底，不做全表预检——几十万量级下
// List 预检即全表扫）。**新建入口须持管理码 kafka-topics:manage**——判定在
// 路由层完成（routes.go 挂 requireTopicManageCode），logic 不再内联。
//
// 归属挂接（列表分页优化）：归属部门 = 表单显式指定的 orgId（格式白名单
// 校验见 orgIDPattern），缺省回退创建人所在部门（list-objects (org, member)
// 的首个结果）。业务表 org_id 列与引擎 org 元组双写：列是列表过滤的谓词
// 来源（漂移方向安全：列多包含的行被列表页引擎复核拦截），元组是权限真源
// （member from org 可见性）。orgId 不做存在性校验：服务端无可用组织查询
// 端点，前端选择器只供真实组织；错误 orgId 的后果是该组织名下无人可见
// （元组挂空组织授予零人，列表谓词永不命中），方向安全不泄漏，创建人
// owner 元组保证自身可见。创建人无部门且未指定 orgId 时挂接跳过（org_id
// 空，行为与存量行一致）。
//
// 主键改造：topics.id 为 DB 自增主键，写库前 ID 未知，故：
//   - 审计 target 用业务唯一键 topic:name:<name>；
//   - write 闭包内先 TopicRepo.Create 拿到自增 ID，再用该 ID 写元组；
//     resp.Id 由自增 ID 转字符串。
func (l *CreateKafkaTopicLogic) CreateKafkaTopic(req *types.CreateKafkaTopicReq) (*types.KafkaTopicResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.BadRequest("AUTHZ_TOPIC_INVALID", "name 不能为空")
	}

	// 重名预检（uk_name 点查；并发竞态由唯一键兜底——竞态路径经审计 failed
	// 记录后按 503 返回，fail-closed 不产生半写状态）。
	if _, derr := l.svcCtx.TopicRepo.GetByName(l.ctx, name); derr == nil {
		return nil, errors.Conflict("AUTHZ_TOPIC_EXISTS", "Topic 已存在: "+name)
	} else if !stderrors.Is(derr, repo.ErrNotFound) {
		l.Logger.Errorf("重名预检失败 name=%s: %v", name, derr)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 数据读取失败")
	}

	// 归属部门：表单指定 orgId 优先，否则回退创建人所在部门（用户维度
	// 小集合，量级 = 用户自己的组织数）。
	orgID := strings.TrimSpace(req.OrgId)
	if orgID != "" {
		if !orgIDPattern.MatchString(orgID) {
			return nil, errors.BadRequest("AUTHZ_TOPIC_INVALID", "orgId 仅限字母、数字与 . _ -")
		}
	} else {
		var oerr error
		orgID, oerr = l.primaryOrgOf(operator)
		if oerr != nil {
			l.Logger.Errorf("创建人部门查询失败 sub=%s: %v", operator, oerr)
			return nil, topicEngineErr()
		}
	}

	in := repo.Topic{
		Name:        name,
		Env:         strings.TrimSpace(req.Env),
		Description: strings.TrimSpace(req.Description),
		OrgID:       orgID,
		CreatedBy:   operator,
		UpdateBy:    operator, // 审计字段（AGENTS.md 规范）：新建即最近更新人。
		CreatedAt:   time.Now(),
	}
	var saved repo.Topic
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_topic", "topic:name:"+name, req, func() error {
		// 先写库拿 DB 自增 ID（重名由唯一键报 ErrDuplicate，闭包外转 409——
		// 不做全表预检），再用该 ID写元组。
		created, werr := l.svcCtx.TopicRepo.Create(l.ctx, in)
		if werr != nil {
			return werr
		}
		saved = created
		// 元组双写：owner（创建人无条件可编辑/可删）+ org 归属挂接（部门成员
		// 可见性，member from org）。以调用者 token 写入：新 topic 首写
		// owner/org 由 authz 创建引导放行（空对象 + kafka-topics:create/
		// manage 码）。带退避重试覆盖瞬时不可用；**重试耗尽则回滚业务行**
		//（消灭半写；预占元组成孤儿由 reconcile 兜底）。
		tuples := []authzkit.Tuple{{
			User: "user:" + operator, Relation: "owner", Object: topicObject(created.ID),
		}}
		if orgID != "" {
			tuples = append(tuples, authzkit.Tuple{
				User: "org:" + orgID, Relation: "org", Object: topicObject(created.ID),
			})
		}
		_, werr = writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, tuples)
		if werr != nil {
			if derr := l.svcCtx.TopicRepo.Delete(l.ctx, created.ID); derr != nil {
				logx.Errorf("owner/org 元组写失败且回滚失败（脏行待清理）topic=%d: %v", created.ID, derr)
			} else {
				logx.Errorf("owner/org 元组写失败，已回滚业务行 topic=%d: %v", created.ID, werr)
			}
		}
		return werr
	})
	if err != nil {
		return nil, err
	}

	return &types.KafkaTopicResp{
		Id:          strconv.FormatInt(saved.ID, 10),
		Name:        saved.Name,
		Env:         saved.Env,
		Description: saved.Description,
		OrgId:       saved.OrgID,
		CreatedBy:   saved.CreatedBy,
		// 创建人即 owner，判定面已知。
		CanEdit: true, CanDelete: true, CanManageGrants: true, Owner: true,
	}, nil
}

// primaryOrgOf 取用户的主部门（list-objects(org, member) 的首个结果，含
// 子树继承的上级部门）；无部门返回空串。归属语义是"创建人所在部门"，
// 仅作为表单未指定 orgId 时的回退来源（表单显式指定优先）。
func (l *CreateKafkaTopicLogic) primaryOrgOf(sub string) (string, error) {
	orgs, err := l.svcCtx.Authz.ListObjects(l.ctx, "org", "member")
	if err != nil {
		return "", err
	}
	for _, o := range orgs {
		if id, ok := strings.CutPrefix(o, "org:"); ok && id != "" {
			return id, nil
		}
	}
	return "", nil
}
