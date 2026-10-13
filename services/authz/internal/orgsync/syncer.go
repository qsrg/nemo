// Package orgsync 是组织架构同步对账：以 CAS 目录（internal/casdir）为
// 组织事实源，把全量组织树与用户归属持续同步到授权系统——
//   - orgs 注册表（repo.OrgStore）：id/name/parentId 展示元数据；
//   - 引擎 parent 元组（org:{parent}#parent@org:{child}）：组织层级，
//     是"上级组织成员经 parent 链继承下级组织身份"判定的物理载体；
//   - 引擎 member 元组（org:{id}#member@user:{uid}）：人员归属。
//
// 同步为全量对账语义：拉 CAS 目标状态 → 与本系统现状 diff → 增/删/改收敛。
// 选对账而非事件监听：CAS 侧无变更推送，且数据量小（数百节点），全量 diff
// 成本可忽略。撤权时效 = 同步周期（config.CasSyncConf.Interval）。
//
// 职责边界（不可破坏）：
//  1. 只写删 CAS 拥有的两类关系（parent/member），绝不触碰授权元组
//     （permission:{code}#can@org:{id}#member 等）——授权是本系统独有职责；
//  2. 删除组织前做引用安全检查：仍被授权指向或被子组织 parent 引用的
//     组织跳过删除并告警，防止悬空授权。
package orgsync

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/services/authz/internal/casdir"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
)

// Result 是单轮同步摘要（日志、审计快照、手动触发接口响应共用）。
type Result struct {
	// OrgsUpserted 注册表 upsert 条数（新建与更新合计）。
	OrgsUpserted int `json:"orgsUpserted"`
	// ParentsWritten 新增/修正的 parent 元组数。
	ParentsWritten int `json:"parentsWritten"`
	// ParentsDeleted 删除的 parent 元组数（换父/根化/组织删除）。
	ParentsDeleted int `json:"parentsDeleted"`
	// MembersWritten 新增的 member 元组数。
	MembersWritten int `json:"membersWritten"`
	// MembersDeleted 删除的 member 元组数（调岗/离职/组织删除）。
	MembersDeleted int `json:"membersDeleted"`
	// OrgsDeleted 删除的组织数（CAS 已撤销且无引用）。
	OrgsDeleted int `json:"orgsDeleted"`
	// OrgsSkipped 因仍有引用而跳过删除的组织数（明细见告警日志）。
	OrgsSkipped int `json:"orgsSkipped"`
	// DanglingUsers orgId 不在组织架构中而被跳过的用户数。
	DanglingUsers int `json:"danglingUsers"`
}

// orgState 是单个组织在引擎里的关系现状（diff 基准）。
type orgState struct {
	// parents parent 元组的 user 集合（形如 org:{parentID}，至多一条，
	// 多条即异常数据，diff 中删除多余）。
	parents map[string]bool
	// children child 元组的 user 集合（形如 org:{childID}）——与 parent 互逆
	//（org:子#child@org:父），判定面 member from child 聚合子树成员用（模型 v5）。
	children map[string]bool
	// members member 元组的 user 集合（形如 user:{uid}）。
	members map[string]bool
}

// auditAction 是同步审计的动作名；target 标识同步对象域。
const (
	auditAction = "sync_orgs"
	auditTarget = "cas:orgs"
	// systemOperator 是同步写路径的操作人标识（审计 create_by/update_by）。
	systemOperator = "system"
)

// RunOnce 执行单轮同步对账并记录审计：无论成败落一条 sync_orgs 审计
// （成功 committed，失败 failed+原因）。同步是幂等对账，失败下轮自动重放，
// 无需 pending 对账重放语义。cas 传入 nil 表示未配置 CAS（报错返回）。
// 任何拉取/写入故障整体中止并返回错误（调用方记日志，下轮重试）——
// 带残缺视图做 diff 比漏一轮更危险。幂等：无差异时零写入。
func RunOnce(ctx context.Context, sc *svc.ServiceContext, cas *casdir.Client) (Result, error) {
	if sc == nil || sc.Engine == nil || sc.Orgs == nil || sc.AuditRepo == nil {
		return Result{}, stderrors.New("orgsync: ServiceContext 未装配")
	}
	if cas == nil {
		return Result{}, stderrors.New("orgsync: CAS 目录客户端未配置")
	}

	res, err := run(ctx, sc, cas)
	// 审计 run 汇总：摘要落 snapshot，失败附原因（对账排障用）。
	snapJSON, _ := json.Marshal(res)
	auditID, auditErr := sc.AuditRepo.CreatePending(ctx, repo.AuditEntry{
		Operator: systemOperator,
		Action:   auditAction,
		Target:   auditTarget,
		Snapshot: string(snapJSON),
		CreateBy: systemOperator,
	})
	if auditErr != nil {
		return res, fmt.Errorf("写同步审计: %w（原错误: %v）", auditErr, err)
	}
	if err != nil {
		if markErr := sc.AuditRepo.MarkFailed(ctx, auditID, err.Error()); markErr != nil {
			return res, fmt.Errorf("标记同步审计 failed: %w（原错误: %v）", markErr, err)
		}
		return res, err
	}
	if err := sc.AuditRepo.MarkCommitted(ctx, auditID); err != nil {
		return res, fmt.Errorf("标记同步审计 committed: %w", err)
	}
	return res, nil
}

// run 执行对账主体（拉取 → diff → 写），错误上抛由 RunOnce 统一记审计。
func run(ctx context.Context, sc *svc.ServiceContext, cas *casdir.Client) (Result, error) {
	var res Result

	// 1. 拉取 CAS 目标状态（全量组织树 + 全量用户归属）。
	casOrgs, err := cas.ListOrgs(ctx)
	if err != nil {
		return res, err
	}
	casUsers, err := cas.ListAllUsers(ctx)
	if err != nil {
		return res, err
	}

	// 2. 目标状态整理：组织 id 集合与组织→成员归属映射；orgId 悬空的用户
	// 跳过并计数（悬空归属同步进引擎会静默失去判定路径，宁可缺席+告警）。
	orgIDs := make(map[string]bool, len(casOrgs))
	for _, o := range casOrgs {
		if o.ID == "" {
			return res, fmt.Errorf("CAS 返回存在空 id 的组织节点")
		}
		orgIDs[o.ID] = true
	}
	// 用户显示名索引（username → displayName，CAS 权威）：members 等接口
	// 展示用；仅同步成功（run 无错返回）时随结果落 svcCtx（末尾 SetUserNames）。
	userNames := make(map[string]string, len(casUsers))
	targetMembers := make(map[string][]string, len(casOrgs))
	for _, u := range casUsers {
		if u.DisplayName != "" {
			userNames[u.Username] = u.DisplayName
		}
		if u.OrgID == "" {
			res.DanglingUsers++
			logx.Errorf("[orgsync] 用户 %s(%s) 无组织归属，跳过", u.Username, u.DisplayName)
			continue
		}
		if !orgIDs[u.OrgID] {
			res.DanglingUsers++
			logx.Errorf("[orgsync] 用户 %s(%s) orgId=%q 不在组织架构中，跳过", u.Username, u.DisplayName, u.OrgID)
			continue
		}
		targetMembers[u.OrgID] = append(targetMembers[u.OrgID], "user:"+u.Username)
	}

	// 3. 读取现状：注册表全量 + 并集组织在引擎里的关系元组。
	localOrgs, err := sc.Orgs.List(ctx)
	if err != nil {
		return res, fmt.Errorf("读组织注册表: %w", err)
	}
	localByName := make(map[string]repo.OrgInfo, len(localOrgs))
	union := make(map[string]bool, len(casOrgs)+len(localOrgs))
	for id := range orgIDs {
		union[id] = true
	}
	for _, o := range localOrgs {
		localByName[o.ID] = o
		union[o.ID] = true
	}
	// 引擎侧孤儿扫描：org 类型的全部对象 id 并入并集——过渡期手工建的组织
	// 在注册表内存期重启后行丢失、引擎元组（MySQL 持久）残留，若不扫出则
	// 删除分支永远看不到它们，元组成永久孤儿。
	engineOrgIDs, err := sc.Engine.ListObjectIDs(ctx, "org")
	if err != nil {
		return res, fmt.Errorf("扫描引擎组织对象: %w", err)
	}
	orphanCount := 0
	for _, id := range engineOrgIDs {
		if union[id] {
			continue
		}
		union[id] = true
		orphanCount++
	}
	if orphanCount > 0 {
		logx.Infof("[orgsync] 发现 %d 个引擎有元组但注册表无行的组织，纳入对账", orphanCount)
	}
	current := make(map[string]orgState, len(union))
	referencedAsParent := make(map[string]bool) // 被子组织 parent 元组引用的组织
	for id := range union {
		tuples, err := sc.Engine.ReadTuples(ctx, "org:"+id, "")
		if err != nil {
			return res, fmt.Errorf("读引擎元组(org=%s): %w", id, err)
		}
		st := orgState{parents: map[string]bool{}, children: map[string]bool{}, members: map[string]bool{}}
		for _, t := range tuples {
			switch t.Relation {
			case "parent":
				st.parents[t.User] = true
				referencedAsParent[t.User] = true
			case "child":
				st.children[t.User] = true
			case "member":
				st.members[t.User] = true
			}
		}
		current[id] = st
	}

	// 4. 注册表 upsert（CAS 全量，按 org_id 幂等；审计字段 system）。
	for _, o := range casOrgs {
		if _, err := sc.Orgs.Upsert(ctx, repo.OrgInfo{
			ID:       o.ID,
			Name:     o.Name,
			ParentID: o.ParentID,
			CreateBy: systemOperator,
			UpdateBy: systemOperator,
		}); err != nil {
			return res, fmt.Errorf("upsert 组织注册表(org=%s): %w", o.ID, err)
		}
		res.OrgsUpserted++
	}

	// 5. 引擎元组 diff（仅 CAS 组织）：parent/child 单值收敛 + member 集合收敛。
	// child 元组挂在父组织对象上（org:子#child@org:父），故按"该组织的孩子们"
	// diff：目标 = CAS 中 parentId 指向该组织的集合。
	targetChildren := make(map[string]map[string]bool, len(casOrgs))
	for _, o := range casOrgs {
		if o.ParentID == "" {
			continue
		}
		if targetChildren[o.ParentID] == nil {
			targetChildren[o.ParentID] = map[string]bool{}
		}
		targetChildren[o.ParentID]["org:"+o.ID] = true
	}
	for _, o := range casOrgs {
		st := current[o.ID]
		var writes []engine.Tuple
		var deletes []engine.Tuple

		// parent：目标一条（根组织无），现状多余/指向不同的一律删。
		wantParent := ""
		if o.ParentID != "" {
			if !orgIDs[o.ParentID] {
				return res, fmt.Errorf("组织 %s 的 parentId=%q 不在组织架构中（CAS 数据不一致）", o.ID, o.ParentID)
			}
			wantParent = "org:" + o.ParentID
		}
		if wantParent != "" && !st.parents[wantParent] {
			writes = append(writes, engine.Tuple{User: wantParent, Relation: "parent", Object: "org:" + o.ID})
			res.ParentsWritten++
		}
		for p := range st.parents {
			if p != wantParent {
				deletes = append(deletes, engine.Tuple{User: p, Relation: "parent", Object: "org:" + o.ID})
				res.ParentsDeleted++
			}
		}

		// child：目标 = CAS 中以该组织为父的孩子集合；现状多余删、缺失写。
		// 与 parent 同源（互逆元组），供判定面 member from child 聚合（模型 v5）。
		wantKids := targetChildren[o.ID]
		if wantKids == nil {
			wantKids = map[string]bool{}
		}
		for c := range st.children {
			if !wantKids[c] {
				deletes = append(deletes, engine.Tuple{User: c, Relation: "child", Object: "org:" + o.ID})
				res.ParentsDeleted++
			}
		}
		for c := range wantKids {
			if !st.children[c] {
				writes = append(writes, engine.Tuple{User: c, Relation: "child", Object: "org:" + o.ID})
				res.ParentsWritten++
			}
		}

		// member：目标 = CAS 归属集合；现状多余删、缺失写。
		wantMembers := make(map[string]bool, len(targetMembers[o.ID]))
		for _, u := range targetMembers[o.ID] {
			wantMembers[u] = true
			if !st.members[u] {
				writes = append(writes, engine.Tuple{User: u, Relation: "member", Object: "org:" + o.ID})
				res.MembersWritten++
			}
		}
		for m := range st.members {
			if !wantMembers[m] {
				deletes = append(deletes, engine.Tuple{User: m, Relation: "member", Object: "org:" + o.ID})
				res.MembersDeleted++
			}
		}

		if err := applyTupleOps(ctx, sc, writes, deletes); err != nil {
			return res, fmt.Errorf("同步组织 %s 元组: %w", o.ID, err)
		}
	}

	// 6. 删除（本系统有、CAS 无的组织，含注册表行与引擎孤儿两种形态）：
	// 引用安全检查通过才删。授权引用反查走 ListObjects（引擎 v1.19 Read
	// 不支持 user 维度过滤，实测报 object type required——与 logic 层
	// permCodesOf 同口径）：检查该组织 userset 直接持有的权限授予（can）
	// 与平台角色承担（assignee）。
	casIDSet := orgIDs
	for id := range union {
		if casIDSet[id] {
			continue
		}
		st := current[id]
		name := id
		if o, ok := localByName[id]; ok && o.Name != "" {
			name = fmt.Sprintf("%s(%s)", o.Name, o.ID)
		}
		subject := "org:" + id + "#member"
		grants, err := sc.Engine.ListObjects(ctx, subject, "can", "permission")
		if err != nil {
			return res, fmt.Errorf("查组织 %s 权限授予引用: %w", id, err)
		}
		roles, err := sc.Engine.ListObjects(ctx, subject, "assignee", "platform_role")
		if err != nil {
			return res, fmt.Errorf("查组织 %s 角色承担引用: %w", id, err)
		}
		switch {
		case len(grants) > 0 || len(roles) > 0:
			res.OrgsSkipped++
			logx.Errorf("[orgsync] 组织 %s 已从 CAS 移除但仍被引用（权限授予 %d 条、角色承担 %d 条），跳过删除，请先处理授权",
				name, len(grants), len(roles))
		case referencedAsParent["org:"+id]:
			res.OrgsSkipped++
			logx.Errorf("[orgsync] 组织 %s 已从 CAS 移除但仍被子组织 parent 引用，跳过删除，请检查 CAS 数据", name)
		default:
			// 无引用：清理残余元组（parent/child/member）+ 注册表行（孤儿无行，
			// Delete 返回 ErrNotFound 属预期，容错跳过）。
			var deletes []engine.Tuple
			for p := range st.parents {
				deletes = append(deletes, engine.Tuple{User: p, Relation: "parent", Object: "org:" + id})
			}
			for c := range st.children {
				deletes = append(deletes, engine.Tuple{User: c, Relation: "child", Object: "org:" + id})
			}
			for m := range st.members {
				deletes = append(deletes, engine.Tuple{User: m, Relation: "member", Object: "org:" + id})
			}
			if err := applyTupleOps(ctx, sc, nil, deletes); err != nil {
				return res, fmt.Errorf("清理组织 %s 元组: %w", id, err)
			}
			if err := sc.Orgs.Delete(ctx, id); err != nil && !stderrors.Is(err, repo.ErrNotFound) {
				return res, fmt.Errorf("删除组织注册表(org=%s): %w", id, err)
			}
			res.OrgsDeleted++
		}
	}

	// 同步成功：整体替换用户显示名索引（失败路径不更新，保留旧索引，
	// 避免同步失败导致展示名全量丢失）。
	sc.SetUserNames(userNames)

	return res, nil
}

// applyTupleOps 提交一批元组写/删操作（批量幂等，空批次直接跳过）。
func applyTupleOps(ctx context.Context, sc *svc.ServiceContext, writes, deletes []engine.Tuple) error {
	if len(writes) > 0 {
		if err := sc.Engine.WriteTuples(ctx, writes); err != nil {
			return fmt.Errorf("写元组: %w", err)
		}
	}
	if len(deletes) > 0 {
		if err := sc.Engine.DeleteTuples(ctx, deletes); err != nil {
			return fmt.Errorf("删元组: %w", err)
		}
	}
	return nil
}
