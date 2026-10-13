// reconcilekafkatopicslogic.go 承载 Topic 授权对账（补偿）逻辑：扫描 Topic
// 业务表，对"库中存在但引擎缺失 owner 元组"的孤儿资源（创建时写 owner
// 重试耗尽、或历史脏数据）按 created_by 批量补写 owner，恢复资源可管理性。
//
// 定位：跨服务写无法做本地事务，本接口是创建路径 owner 写入失败的最终兜底
// （重试 → 审计 failed → 对账补偿）。仅运维操作，需持有页面管理码
// kafka-topics:manage；引擎故障时 fail-closed（不做半吊子对账）。
package logic

import (
	"context"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ReconcileKafkaTopicsLogic 承载对账接口的业务规则。
type ReconcileKafkaTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReconcileKafkaTopicsLogic 构造对账业务规则层实例。
func NewReconcileKafkaTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReconcileKafkaTopicsLogic {
	return &ReconcileKafkaTopicsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ReconcileKafkaTopics 扫描全部 Topic 并补写缺失的 owner 元组（管理码判定
// 已上收路由守卫 requireTopicManageCode，logic 只关注对账业务）。
//
// 流程：
//  1. TopicRepo.List 取全量业务记录；
//  2. 逐个 ListRelations(obj,"owner")：无 owner 且 created_by 非空 →
//     记入待补清单；created_by 为空（无法推断 owner）→ 记录错误并跳过；
//  3. 批量 WriteRelations 补写 owner，返回扫描数与补写数。
//
// 返回：
//   - ReconcileResp：Scanned 扫描总数、Repaired 补写数；
//   - error：引擎读取/补写故障返回 503（fail-closed）。
func (l *ReconcileKafkaTopicsLogic) ReconcileKafkaTopics() (*types.ReconcileResp, error) {
	operator, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}

	topics, lerr := l.svcCtx.TopicRepo.List(l.ctx)
	if lerr != nil {
		l.Logger.Errorf("reconcile: Topic 表读取失败: %v", lerr)
		return nil, topicEngineErr()
	}

	resp := &types.ReconcileResp{Scanned: len(topics)}
	pending := make([]authzkit.Tuple, 0)
	// 创建失败的 target 集合（审计 failed）：业务行是"回滚失败的脏行"，
	// 对账不得补 owner（否则脏行被洗白成有效资源）——跳过交人工清理。
	failedTargets, ferr := l.svcCtx.AuditRepo.ListFailedTargets(l.ctx, []string{"create_topic"})
	if ferr != nil {
		l.Logger.Errorf("reconcile: 失败审计读取失败: %v", ferr)
		return nil, topicEngineErr()
	}
	skipped := 0
	for _, t := range topics {
		obj := topicObject(t.ID)
		if strings.TrimSpace(t.CreatedBy) == "" {
			// 无创建人信息无法推断 owner，不能凭空指定——记录后交人工处理。
			l.Logger.Errorf("reconcile: topic 无 created_by，无法补 owner，请人工处理 %s", obj)
			continue
		}
		owners, rerr := listRelationsWithRetry(l.ctx, l.svcCtx.Authz, obj, "owner")
		if rerr != nil {
			l.Logger.Errorf("reconcile: owner 元组读取失败 %s: %v", obj, rerr)
			return nil, topicEngineErr()
		}
		if len(owners) == 0 {
			if failedTargets["topic:name:"+t.Name] {
				skipped++ // 创建审计 failed = 回滚失败的脏行
				continue
			}
			pending = append(pending, authzkit.Tuple{
				User: "user:" + t.CreatedBy, Relation: "owner", Object: obj,
			})
		}
	}

	if len(pending) > 0 {
		if _, werr := writeRelationsWithRetry(l.ctx, l.svcCtx.Authz, pending); werr != nil {
			l.Logger.Errorf("reconcile: owner 批量补写失败 count=%d: %v", len(pending), werr)
			return nil, topicEngineErr()
		}
		resp.Repaired = len(pending)
	}
	l.Logger.Infof("reconcile 完成 sub=%s scanned=%d repaired=%d", operator, resp.Scanned, resp.Repaired)
	return resp, nil
}
