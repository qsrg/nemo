// createinstancelogic.go 承载新建实例的业务规则：写库 + 写 owner 元组
// （审计先行），新建权判定已上收路由守卫（routes.go 挂 requireCodeOrManage），
// 业务代码只关注业务。
//
// 与 kafka 的差异：**不写 org 元组、不落归属列**——redis 实例量级小，可见性
// 走纯引擎口径（owner + 显式 viewer/editor 授予），创建后由创建人（owner）
// 经授权对话框按需授予他人。
package logic

import (
	"context"
	stderrors "errors"
	"strconv"
	"strings"
	"time"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/redis/internal/repo"
	"nemo/services/redis/internal/svc"
	"nemo/services/redis/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CreateInstanceLogic 承载新建实例接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type CreateInstanceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateInstanceLogic 构造业务规则层实例。
func NewCreateInstanceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateInstanceLogic {
	return &CreateInstanceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateInstance 新建实例（新建权判定已上收路由守卫：redis:create 动作码或
// redis:manage 管理码）：clusterId 必填且集群须存在（400/404）、name 必填
// （400）、重名 409（DB 唯一键兜底，不做全表预检）。
//
// 主键与元组顺序：instances.id 为 DB 自增主键，写库前 ID 未知，故：
//   - 审计 target 用业务唯一键 instance:name:<name>；
//   - write 闭包内先 InstanceRepo.Create 拿到自增 ID，再用该 ID 写 owner
//     元组（创建人无条件可见/可编辑/可删/可配名单）；
//   - resp.Id 由自增 ID 转字符串。
func (l *CreateInstanceLogic) CreateInstance(req *types.CreateInstanceReq) (*types.InstanceResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	// 集群归属校验（先选集群再出实例——创建必须落在具体集群下；集群为
	// 基础配置数据，无权限维度，存在性即可）。ClusterId 为字符串形态契约，
	// 经 parseClusterID 归一非法格式（400）。
	clusterId, err := parseClusterID(req.ClusterId)
	if err != nil {
		return nil, err
	}
	if _, gerr := l.svcCtx.ClusterRepo.GetById(l.ctx, clusterId); gerr != nil {
		l.Logger.Errorf("集群不存在 clusterId=%d: %v", clusterId, gerr)
		return nil, errors.NotFound("REDIS_CLUSTER_NOT_FOUND", "集群不存在")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.BadRequest("REDIS_INSTANCE_INVALID", "name 不能为空")
	}

	// 重名预检（uk_name 点查；并发竞态由唯一键兜底——竞态路径经审计 failed
	// 记录后按 503 返回，fail-closed 不产生半写状态）。
	if _, derr := l.svcCtx.InstanceRepo.GetByName(l.ctx, name); derr == nil {
		return nil, errors.Conflict("REDIS_INSTANCE_EXISTS", "实例已存在: "+name)
	} else if !stderrors.Is(derr, repo.ErrNotFound) {
		l.Logger.Errorf("重名预检失败 name=%s: %v", name, derr)
		return nil, errors.New(503, "REDIS_SERVICE_UNAVAILABLE", "实例数据读取失败")
	}

	in := repo.Instance{
		Name:        name,
		Addr:        strings.TrimSpace(req.Addr),
		Description: strings.TrimSpace(req.Description),
		ClusterID:   clusterId,
		CreatedBy:   operator,
		UpdateBy:    operator, // 审计字段（AGENTS.md 规范）：新建即最近更新人。
		CreatedAt:   time.Now(),
	}
	var saved repo.Instance
	err = writeWithAudit(l.ctx, l.svcCtx, operator, "create_instance", "instance:name:"+name, req, func() error {
		// 先写库拿 DB 自增 ID（重名由唯一键报 ErrDuplicate，闭包外转 409——
		// 不做全表预检），再用该 ID 写元组。
		created, werr := l.svcCtx.InstanceRepo.Create(l.ctx, in)
		if werr != nil {
			return werr
		}
		saved = created
		// owner 元组（创建人无条件可见/可编辑/可删/可配名单）。以调用者
		// token 写入：新对象首写 owner 由 authz 的创建引导放行（空对象 +
		// 持 redis:create/redis:manage 码，writerelationslogic 空对象分支）。
		// 带退避重试覆盖瞬时不可用；**重试耗尽则回滚业务行**——半写状态
		// （资源存在却无主）被预占攻击利用过，失败即失败，不留脏行；预占
		// 元组成孤儿，由对账端点/运维清理。
		_, werr = writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, []authzkit.Tuple{{
			User: "user:" + operator, Relation: "owner", Object: instanceObject(created.ID),
		}})
		if werr != nil {
			if derr := l.svcCtx.InstanceRepo.Delete(l.ctx, created.ID); derr != nil {
				l.Logger.Errorf("owner 元组写失败且回滚失败（脏行待清理）id=%d: %v", created.ID, derr)
			} else {
				l.Logger.Errorf("owner 元组写失败，已回滚业务行 id=%d: %v", created.ID, werr)
			}
		}
		return werr
	})
	if err != nil {
		return nil, err
	}

	return &types.InstanceResp{
		Id:          strconv.FormatInt(saved.ID, 10),
		Name:        saved.Name,
		Addr:        saved.Addr,
		Description: saved.Description,
		CreatedBy:   saved.CreatedBy,
		// 创建人即 owner，判定面已知。
		CanEdit: true, CanDelete: true, CanManageGrants: true, Owner: true,
	}, nil
}
