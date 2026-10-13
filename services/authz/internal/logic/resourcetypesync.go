// resourcetypesync.go 实现资源类型由菜单声明派生（按钮声明驱动，替代独立注册）。
//
// 背景：资源类型不再需要独立注册页面/接口——类型就是"资源级按钮声明的
// 资源归类"。派生规则：
//
//   - type=menu 且 resourceType 非空：页面入口 ⇒ 隐含 can_view（管理该类型
//     资源至少可看列表，行级可见性仍由 can_view 授权控制）；
//   - type=button 且 relation 非空：资源级操作 ⇒ 贡献该 relation（引擎出口
//     白名单 can_view/can_edit/can_delete/can_share，菜单写路径已校验）；
//   - type=button 且 relation 空（页面级按钮）：不贡献（与具体资源无关）。
//
// 同一 resourceType 被多行引用时 actions 取并集；类型存在即"被引用"，无需
// 单独注册。sync 与注册表 diff：仅被引用类型 upsert（Render 确定性输出，
// 同 actions 同 DSL 块），未被引用的存量类型不自动删除（声明优先，下线需
// 显式处理，避免误删存量授权元组）。有变更时重建"基础模型 + 全部注册块"
// 并 EnsureModel（与 RegisterResourceType 同源合并，见 modelmerge）。
package logic

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"nemo/kit/errors"
	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/modelmerge"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// SyncResourceTypesFromMenus 扫描全部菜单行派生资源类型并同步注册表/引擎模型。
// 由菜单写路径（create/update/delete menu）成功后调用；无引用变化时无副作用
// （不重建引擎模型，幂等）。任一失败返回错误——调用方按 503 fail-closed 处理
// （菜单本身已生效，重试该 sync 即可归一，与注册路径的自愈语义一致）。
func SyncResourceTypesFromMenus(ctx context.Context, sc *svc.ServiceContext) error {
	menus, err := sc.MenuRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("列菜单派生资源类型: %w", err)
	}

	// 派生：类型名 → actions 集合（menu 行隐含 can_view，资源级按钮贡献 relation）。
	// resource_type 支持逗号分隔多值（页面含多种业务资源），逐项拆分派生。
	derived := map[string]map[string]bool{}
	for _, m := range menus {
		for _, rt := range splitResourceTypes(m.ResourceType) {
			set := derived[rt]
			if set == nil {
				set = map[string]bool{}
				derived[rt] = set
			}
			switch m.Type {
			case "menu":
				set["can_view"] = true // 页面入口 = 该类型列表可见性维度
			case "button":
				if m.Relation != "" {
					set[m.Relation] = true
				}
			}
		}
	}

	// 白名单固定顺序渲染（Render 内部规范化，同参数同输出）。
	order := []string{"can_view", "can_edit", "can_delete", "can_share", "can_manage_grants"}
	names := make([]string, 0, len(derived))
	for name, set := range derived {
		if len(set) == 0 {
			// 仅被页面级按钮（relation 空）引用：无操作维度，无法渲染类型块，
			// 跳过并告警——创建侧已禁止该形态（页面级按钮不得声明 resourceType）。
			logx.WithContext(ctx).Infof("资源类型 %s 无操作维度（无菜单行/资源级按钮声明），跳过派生", name)
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	// 与注册表 diff：被引用类型 upsert（比对确定性 DSL 块）。
	existing, err := sc.ResourceTypeRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("列资源类型注册表(派生同步): %w", err)
	}
	byName := map[string]repo.ResourceType{}
	for _, rt := range existing {
		byName[rt.Name] = rt
	}
	blocks := make([]string, 0, len(names))
	changed := false
	for _, name := range names {
		actions := make([]string, 0, len(derived[name]))
		for _, a := range order {
			if derived[name][a] {
				actions = append(actions, a)
			}
		}
		block, rerr := engine.Render(engine.ResourceType{Name: name, Actions: actions})
		if rerr != nil {
			return fmt.Errorf("渲染资源类型块(%s): %w", name, rerr)
		}
		blocks = append(blocks, block)
		cur, ok := byName[name]
		if ok && cur.DslBlock == block {
			continue // 无变化
		}
		changed = true
		if ok {
			// 更新：保留存量名单管理码（派生不覆盖管理面既有策略声明）。
			if uerr := sc.ResourceTypeRepo.Update(ctx, repo.ResourceType{
				Name:       name,
				DslBlock:   block,
				ManageCode: cur.ManageCode,
				UpdateBy:   "system-sync", // 审计字段（AGENTS.md 规范）：派生同步更新人。
			}); uerr != nil {
				return fmt.Errorf("更新资源类型(%s): %w", name, uerr)
			}
			logx.WithContext(ctx).Infof("资源类型派生同步更新: %s (actions=%v)", name, actions)
		} else {
			if _, cerr := sc.ResourceTypeRepo.Create(ctx, repo.ResourceType{
				Name:       name,
				DslBlock:   block,
				ManageCode: "",
				CreateBy:   "system-sync", // 审计字段（AGENTS.md 规范）：派生同步创建人。
				UpdateBy:   "system-sync",
			}); cerr != nil {
				return fmt.Errorf("注册资源类型(派生同步)(%s): %w", name, cerr)
			}
			logx.WithContext(ctx).Infof("资源类型派生同步注册: %s (actions=%v)", name, actions)
		}
	}
	if !changed {
		return nil
	}

	// 有变更：重建完整 DSL（基础 + 全部注册块）并 EnsureModel + 版本落库。
	// 引擎先行，注册表已在上方落库——若引擎失败，下次 sync/启动重放自愈。
	rts, lerr := sc.ResourceTypeRepo.List(ctx)
	if lerr != nil {
		return fmt.Errorf("列资源类型注册表(重建模型): %w", lerr)
	}
	allBlocks := make([]string, 0, len(rts))
	for _, rt := range rts {
		allBlocks = append(allBlocks, rt.DslBlock)
	}
	base, berr := modelmerge.ReadBase(sc.Config.Engine.ModelFile)
	if berr != nil {
		return fmt.Errorf("读取基础模型(派生同步): %w", berr)
	}
	dsl := modelmerge.MergeBlocks(base, allBlocks)
	_, modelID, eerr := sc.Engine.EnsureModel(ctx, dsl)
	if eerr != nil {
		return fmt.Errorf("重建授权模型(派生同步): %w", eerr)
	}
	if serr := sc.ModelVersionRepo.Save(ctx, repo.ModelVersion{
		EnginemodelID: modelID,
		Dsl:           dsl,
		CreateBy:      "system-sync", // 审计字段（AGENTS.md 规范）：派生同步版本创建人。
		UpdateBy:      "system-sync",
	}); serr != nil {
		// 版本落库失败不阻断（版本表是比对辅助，启动重放可归一）。
		logx.WithContext(ctx).Errorf("派生同步版本落库失败（不阻断）: %v", serr)
	}
	logx.WithContext(ctx).Infof("资源类型派生同步完成: %d 个类型, modelID=%s", len(allBlocks), modelID)
	return nil
}

// resourceTypeSyncErrTo503 将派生同步错误统一转译为 503（fail-closed）：
// 菜单已落库但资源类型未同步，授权判定将不可用，必须显式失败而非静默裸奔。
func resourceTypeSyncErrTo503(err error) error {
	if err == nil {
		return nil
	}
	logx.Errorf("资源类型派生同步失败: %v", err)
	return errors.New(http.StatusServiceUnavailable, "AUTHZ_SERVICE_UNAVAILABLE", "资源类型同步失败（菜单已生效，重试该操作可归一）")
}
