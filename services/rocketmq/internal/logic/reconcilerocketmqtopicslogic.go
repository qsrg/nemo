// reconcilerocketmqtopicslogic.go 承载集群授权对账（补偿）逻辑：扫描
// clusters 业务表，对"库中存在但引擎元组缺失"的集群补写 owner 元组——
// 创建集群时 owner 元组写失败（重试耗尽）的最终兜底，恢复集群授权名单
// 可管理性（集群可见性仍由页面授权配置，不在此处补写）。
//
// 定位：跨服务写无法做本地事务，本接口是创建路径元组写入失败的最终兜底；
// 仅运维操作，需持有页面管理码 rocketmq-topics:manage（路由守卫上收）；
// 引擎故障时 fail-closed（503，不做半吊子对账）。
//
// 注：topic 在集群级授权模型（2026-10-12 定稿）下不再写引擎元组，
// 对账只覆盖集群维度；存量 rmq_topic 孤儿元组由删除路径清理覆盖。
package logic

import (
	"context"
	"net/http"
	"strings"

	authzkit "nemo/kit/authz"
	kiterrors "nemo/kit/errors"
	"nemo/services/rocketmq/internal/svc"
	"nemo/services/rocketmq/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ReconcileRocketmqTopicsLogic 承载对账接口的业务规则。
type ReconcileRocketmqTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReconcileRocketmqTopicsLogic 构造对账业务规则层实例。
func NewReconcileRocketmqTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReconcileRocketmqTopicsLogic {
	return &ReconcileRocketmqTopicsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ReconcileRocketmqTopics 扫描全部集群并补写缺失的 owner 元组：
//  1. 业务行无 owner 元组且 created_by 非空 → 补 user:{created_by} owner
//     （集群创建人可管理该集群授权名单；created_by 为空无法推断，记日志跳过）；
//  2. 批量 WriteRelations 一次提交，返回扫描数与补写数。
func (l *ReconcileRocketmqTopicsLogic) ReconcileRocketmqTopics() (*types.ReconcileRocketmqTopicsResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	clusters, lerr := l.svcCtx.ClusterRepo.List(l.ctx)
	if lerr != nil {
		l.Logger.Errorf("reconcile: clusters 表读取失败: %v", lerr)
		return nil, kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "集群数据读取失败")
	}

	resp := &types.ReconcileRocketmqTopicsResp{Scanned: len(clusters)}
	pending := make([]authzkit.Tuple, 0)
	for _, c := range clusters {
		obj := rmqClusterObject(c.ID)

		// owner 元组补写：缺失且能从 created_by 推断归属人。
		if strings.TrimSpace(c.CreatedBy) == "" {
			// 无创建人信息无法推断 owner，不能凭空指定——记录后交人工处理。
			l.Logger.Errorf("reconcile: cluster 无 created_by，无法补 owner，请人工处理 %s", obj)
			continue
		}
		owners, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, obj, "owner")
		if rerr != nil {
			l.Logger.Errorf("reconcile: owner 元组读取失败 %s: %v", obj, rerr)
			return nil, kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
		}
		if len(owners) == 0 {
			pending = append(pending, authzkit.Tuple{
				User: "user:" + c.CreatedBy, Relation: "owner", Object: obj,
			})
		}
	}

	if len(pending) > 0 {
		if _, werr := writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, pending); werr != nil {
			l.Logger.Errorf("reconcile: 元组批量补写失败 count=%d: %v", len(pending), werr)
			return nil, kiterrors.New(http.StatusServiceUnavailable, "AUTHZ_ENGINE_ERROR", "权限引擎不可用")
		}
		resp.Repaired = len(pending)
	}
	l.Logger.Infof("reconcile 完成 sub=%s scanned=%d repaired=%d", operator, resp.Scanned, resp.Repaired)
	return resp, nil
}
