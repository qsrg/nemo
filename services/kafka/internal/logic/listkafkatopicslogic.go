// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 列表分页优化（几十万量级）：可见性谓词下推 SQL（归属列 + 直接授予 ID），
// 不再"list-objects 全量展开 + IN 下推"——可见集合随 Topic 总数膨胀，
// 该链路在集合上万个 ID 后失效；列谓词只随用户自己的关系数增长。

package logic

import (
	"context"
	"strconv"
	"strings"

	authzkit "nemo/kit/authz"
	"nemo/kit/errors"
	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListKafkaTopicsLogic 承载该接口的业务规则，内嵌 logx.Logger 并持有 svc.ServiceContext。
type ListKafkaTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListKafkaTopicsLogic 构造业务规则层实例。
func NewListKafkaTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListKafkaTopicsLogic {
	return &ListKafkaTopicsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListKafkaTopics Topic 列表（列谓词分页 + 引擎逐行复核，两段式）：
//
// 第一段·取页（SQL，量级与 Topic 总数解耦）：
//   - 视角：kafka-topics:manage 持有者全量翻页（无谓词）；
//   - 普通用户谓词 = org_id IN (用户部门) OR id IN (直接持有关系的对象)——
//     与模型 can_view = member_view(viewer/editor) or member from org or owner
//     对齐：部门归属走归属列（org 元组的列投影），显式授予（owner/viewer/
//     editor 元组，含名单对话框的部门授予展开）走 ID 集合——两者都是
//     用户维度的小集合（list-objects 各一次，量级 = 用户自己的关系数）；
//   - 徽标集合（owner/can_edit/can_delete）同样经 list-objects 批量取回，
//     不逐行 Check；can_delete 与 can_edit 是独立出口（2026-09-30 拆分：
//     can_delete=owner，can_edit=editor），删除徽标 = owner 集合投影。
//
// 第二段·复核（引擎，每页至多 pageSize 次判定）：页内行逐条 Check(can_view)
// ——引擎是权限真源 + ABAC deny/allow 自动叠加，把列投影的漂移拦在出口
// （列多包含的行在此被滤掉；列漏包含 = 少显示一行，无泄漏）。复核后
// total 为近似值（COUNT 是谓词口径），ABAC 门槛启用时页内可能少于
// pageSize——可接受的折衷，量级到游标分页时一并处理。
//
// 授权服务故障 → 503 fail-closed（判定集合加载与逐行复核任一失败整页拒绝）。
func (l *ListKafkaTopicsLogic) ListKafkaTopics(req *types.ListTopicsReq) (*types.KafkaTopicListResp, error) {
	sub, err := operatorFrom(l.ctx)
	if err != nil {
		return nil, err
	}
	manageOK, err := menuManageGranted(l.ctx, l.svcCtx.Authz, KafkaTopicsManageCode)
	if err != nil {
		l.Logger.Errorf("manage 判定失败 sub=%s: %v", sub, err)
		return nil, topicEngineErr()
	}

	// owner/can_edit 徽标集合：owner 两种视角都需要（管理员未必是每条的
	// owner）；editor 仅普通用户需要（manage 持有者 can_edit 恒真）。
	ownerSet, err := topicRelationSet(l.ctx, l.svcCtx, "owner")
	if err != nil {
		l.Logger.Errorf("owner 集合加载失败 sub=%s: %v", sub, err)
		return nil, topicEngineErr()
	}
	var editSet map[string]bool
	if !manageOK {
		editSet, err = topicRelationSet(l.ctx, l.svcCtx, "editor")
		if err != nil {
			l.Logger.Errorf("editor 集合加载失败 sub=%s: %v", sub, err)
			return nil, topicEngineErr()
		}
	}

	// 可见性谓词原料（用户维度小集合）。
	filter := repo.TopicPageFilter{All: manageOK, Page: req.Page, PageSize: req.PageSize}
	if !manageOK {
		orgs, oerr := l.svcCtx.Authz.ListObjects(l.ctx, "org", "member")
		if oerr != nil {
			l.Logger.Errorf("部门集合加载失败 sub=%s: %v", sub, oerr)
			return nil, topicEngineErr()
		}
		filter.OrgIDs = parseOrgIDs(orgs)
		// 直接授予对象并集：viewer（含名单对话框的部门授予展开）∪ editor ∪ owner。
		filter.IDs = append(filter.IDs, parseTopicObjectIDs(keysOf(ownerSet))...)
		filter.IDs = append(filter.IDs, parseTopicObjectIDs(keysOf(editSet))...)
		if viewerIDs, verr := topicRelationSet(l.ctx, l.svcCtx, "viewer"); verr != nil {
			l.Logger.Errorf("viewer 集合加载失败 sub=%s: %v", sub, verr)
			return nil, topicEngineErr()
		} else {
			filter.IDs = append(filter.IDs, parseTopicObjectIDs(keysOf(viewerIDs))...)
		}
	}

	topics, total, err := l.svcCtx.TopicRepo.Page(l.ctx, filter)
	if err != nil {
		l.Logger.Errorf("Topic 分页查询失败: %v", err)
		return nil, errors.New(503, "AUTHZ_SERVICE_UNAVAILABLE", "Topic 数据读取失败")
	}

	// 引擎批量复核（真源 + ABAC）：普通用户 can_view 批量 Check 不过的行不展示；
	// manage 持有者全权（含可见），跳过复核——与第一段 All=true 取页口径一致。
	// 用 BatchCheck 一次 RPC 替代 N 次逐行 Check，减少引擎压力与网络开销。
	var viewResults []bool
	if !manageOK && len(topics) > 0 {
		batchItems := make([]authzkit.BatchCheckItem, 0, len(topics))
		for _, t := range topics {
			batchItems = append(batchItems, authzkit.BatchCheckItem{
				Object:   topicObject(t.ID),
				Relation: "can_view",
			})
		}
		var berr error
		viewResults, berr = l.svcCtx.Authz.BatchCheck(l.ctx, batchItems)
		if berr != nil {
			l.Logger.Errorf("can_view 批量复核失败 topics=%d: %v", len(topics), berr)
			return nil, topicEngineErr()
		}
	}

	items := make([]types.KafkaTopicResp, 0, len(topics))
	for i, t := range topics {
		obj := topicObject(t.ID)
		if !manageOK && !viewResults[i] {
			continue
		}
		canEdit := manageOK || ownerSet[obj] || editSet[obj]
		// CanManageGrants 与路由守卫 requireTopicManagePerm 同源：模型出口
		// can_manage_grants = owner（2026-09-30 入 DSL，名单管理权独立边界）
		// + 管理码回退；ownerSet 即其投影（导出即 owner 集合），不含 editor。
		canManageGrants := manageOK || ownerSet[obj]
		// CanDelete 与删除路由守卫同源：模型出口 can_delete = owner（2026-09-30
		// 起与 can_edit 拆分，可编辑者不再自动可删）+ 管理码回退。
		canDelete := manageOK || ownerSet[obj]
		// CanView 行级只读出口：manage 码全权；普通用户以引擎批量复核结果
		// 为准（viewResults 与 topics 等长，仅 !manageOK 时参与计算，短路安全）。
		canView := manageOK || viewResults[i]
		items = append(items, types.KafkaTopicResp{
			Id:              strconv.FormatInt(t.ID, 10),
			Name:            t.Name,
			Env:             t.Env,
			Description:     t.Description,
			OrgId:           t.OrgID,
			CreatedBy:       t.CreatedBy,
			CanEdit:         canEdit,
			CanDelete:       canDelete,
			CanManageGrants: canManageGrants,
			CanView:         canView,
			Owner:           ownerSet[obj],
		})
	}
	return &types.KafkaTopicListResp{Items: items, Total: total}, nil
}

// topicRelationSet 经 list-objects 批量取回当前用户在 kafka_topic 上持有
// relation（owner/viewer/editor 等）的对象集合，返回以完整对象串为键的集合。
func topicRelationSet(ctx context.Context, sc *svc.ServiceContext, relation string) (map[string]bool, error) {
	objects, err := sc.Authz.ListObjects(ctx, "kafka_topic", relation)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(objects))
	for _, o := range objects {
		set[o] = true
	}
	return set, nil
}

// keysOf 取集合的全部键。
func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// parseTopicObjectIDs 把 list-objects 返回的完整对象串（kafka_topic:13）
// 解析为 int64 ID；前缀不符、非法或非正数的条目跳过。
func parseTopicObjectIDs(objects []string) []int64 {
	ids := make([]int64, 0, len(objects))
	for _, o := range objects {
		raw, ok := strings.CutPrefix(o, "kafka_topic:")
		if !ok {
			continue
		}
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// parseOrgIDs 把 list-objects 返回的完整对象串（org:tech）解析为部门 ID。
func parseOrgIDs(objects []string) []string {
	ids := make([]string, 0, len(objects))
	for _, o := range objects {
		if id, ok := strings.CutPrefix(o, "org:"); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
