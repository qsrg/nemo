// createrocketmqtopiclogic.go 实现新建 Topic：参数校验（必填 name +
// clusterId）+ 集群存在与可见性校验 + 写入。权限判定在路由守卫（create
// 码或管理码），logic 层再校验目标集群对当前用户可见（防越权建到不可见
// 集群——集群不可见则建出的 topic 也无人可见，属无效操作）。
//
// 授权模型（2026-10-12 定稿）：集群级授权，topic 不再单独写引擎元组
// （可见性与操作权 = 所属集群的 rmq_cluster can_view），故创建路径无
// 元组双写。
package logic

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"nemo/kit/errors"
	"nemo/services/rocketmq/internal/repo"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// CreateRocketmqTopicLogic 承载新建接口业务规则。
type CreateRocketmqTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewCreateRocketmqTopicLogic 构造新建业务规则实例。
func NewCreateRocketmqTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateRocketmqTopicLogic {
	return &CreateRocketmqTopicLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// CreateRocketmqTopic 新建 Topic：name 必填非空白、clusterId 必传且集群
// 存在；重名 409；集群对当前用户不可见（非管理码）按 403 拒绝。成功返回
// 新建记录（ID 由仓库自增）。创建人写入 CreatedBy（审计口径）。
func (l *CreateRocketmqTopicLogic) CreateRocketmqTopic(req *types.CreateRocketmqTopicReq) (*types.RocketmqTopicResp, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New(http.StatusBadRequest, "AUTHZ_PARAM_INVALID", "Topic 名称必填")
	}
	if req.ClusterId <= 0 {
		return nil, errors.New(http.StatusBadRequest, "AUTHZ_PARAM_INVALID", "clusterId 必填")
	}
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	// 集群必须真实存在。
	if _, cerr := l.svcCtx.ClusterRepo.GetById(l.ctx, req.ClusterId); cerr != nil {
		return nil, errors.New(http.StatusBadRequest, "CLUSTER_NOT_FOUND", "集群不存在")
	}
	// 集群可见性校验：非管理码用户必须能看目标集群才能在其中建 topic。
	manageOK, merr := codeGranted(l.ctx, l.svcCtx, RocketMQTopicsManageCode)
	if merr != nil {
		l.Logger.Errorf("manage 码判定失败 sub=%s: %v", sub, merr)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
	}
	if !manageOK {
		viewSet, verr := clusterRelationSet(l.ctx, l.svcCtx, "can_view")
		if verr != nil {
			l.Logger.Errorf("集群 can_view 集合加载失败 sub=%s: %v", sub, verr)
			return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
		}
		if !viewSet[rmqClusterObject(req.ClusterId)] {
			return nil, errors.New(http.StatusForbidden, "AUTHZ_DENIED", "无权在该集群创建 Topic")
		}
	}

	created, werr := l.svcCtx.TopicRepo.Create(l.ctx, types.RocketmqTopic{
		Name:        name,
		Env:         strings.TrimSpace(req.Env),
		Description: strings.TrimSpace(req.Description),
		ClusterId:   req.ClusterId,
		CreatedBy:   sub,
	})
	if werr != nil {
		if stderrors.Is(werr, repo.ErrDuplicate) {
			return nil, errors.New(http.StatusConflict, "AUTHZ_DUPLICATE", "Topic 名称已存在")
		}
		l.Logger.Errorf("Topic 创建失败: %v", werr)
		return nil, errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 创建失败")
	}

	return &types.RocketmqTopicResp{
		Id:          strconv.FormatInt(created.Id, 10),
		Name:        created.Name,
		Env:         created.Env,
		Description: created.Description,
		ClusterId:   strconv.FormatInt(created.ClusterId, 10),
		CreatedBy:   created.CreatedBy,
	}, nil
}
