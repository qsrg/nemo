// replay.go 实现启动模型重放（Phase C 改造②，修 Shares 页 503）：
//
// 现状 bug：资源类型注册表（biz 库）有已注册类型时，服务重启会让引擎
// 回落基础 model.fga（或内存引擎从零开始），模型中不存在注册类型，
// 对该类型对象的 check 报 503 AUTHZ_ENGINE_ERROR。
//
// 修复：启动装配时（bootstrap 之后，main 调用）列资源类型注册表 →
// 逐块取 DslBlock → 与基础模型合并（modelmerge，与 RegisterResourceType
// 同一公共函数）→ EnsureModel；无注册类型则跳过（基础模型由 bootstrap
// 负责）。重建式合并与注册路径同源自愈：biz 库与引擎状态漂移也在
// 重启时归一。
package svc

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/modelmerge"
	"nemo/services/authz/internal/repo"
)

// ReplayRegisteredModel 启动模型重放：注册表有已注册类型时，以"基础模型 +
// 全部注册块"重建完整 DSL 并 EnsureModel 绑定为当前判定模型；模型版本同步
// 落 ModelVersionRepo（与最新版本一致时跳过，避免每次启动累积版本行）。
// 无注册类型时跳过（返回 nil，基础模型由 bootstrap 装配）。
// 任一步失败返回错误——启动期依赖不可用必须快速失败（main 侧退出）。
func ReplayRegisteredModel(ctx context.Context, sc *ServiceContext) error {
	rts, err := sc.ResourceTypeRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("列资源类型注册表: %w", err)
	}
	if len(rts) == 0 {
		logx.Info("启动模型重放：无已注册资源类型，跳过（基础模型由 bootstrap 装配）")
		return nil
	}

	// 模板迁移：按当前 Render 模板对存量块幂等重渲染——模板升级（如
	// owner 放开 [user, org#member]）时库里 DslBlock 是历史输出，不重渲染
	// 新模型不生效。同参数 Render 输出确定，无差异即原样沿用。
	blocks := make([]string, 0, len(rts))
	for _, rt := range rts {
		block, err := engine.Render(engine.ResourceType{
			Name:    rt.Name,
			Actions: engine.ExtractActions(rt.DslBlock),
		})
		if err != nil {
			return fmt.Errorf("重渲染资源类型模型块(%s): %w", rt.Name, err)
		}
		if block != rt.DslBlock {
			rt.DslBlock = block
			rt.UpdateBy = "system-replay"
			if uerr := sc.ResourceTypeRepo.Update(ctx, rt); uerr != nil {
				return fmt.Errorf("迁移资源类型 DslBlock(%s): %w", rt.Name, uerr)
			}
			logx.Infof("启动模型迁移：资源类型 %s 的 DslBlock 已按最新模板重渲染", rt.Name)
		}
		blocks = append(blocks, block)
	}
	base, err := modelmerge.ReadBase(sc.Config.Engine.ModelFile)
	if err != nil {
		return fmt.Errorf("读取基础模型: %w", err)
	}
	dsl := modelmerge.MergeBlocks(base, blocks)

	_, modelID, err := sc.Engine.EnsureModel(ctx, dsl)
	if err != nil {
		return fmt.Errorf("重放装配授权模型(%d 个注册类型): %w", len(rts), err)
	}

	// 版本落库：与最新记录一致则跳过；Latest 无记录/读失败按写入处理
	//（版本表是比对辅助，失败不阻断重放，只记日志）。
	latest, lerr := sc.ModelVersionRepo.Latest(ctx)
	if lerr != nil || latest.Dsl != dsl {
		if serr := sc.ModelVersionRepo.Save(ctx, repo.ModelVersion{EnginemodelID: modelID, Dsl: dsl}); serr != nil {
			logx.Errorf("重放模型版本落库失败（不阻断启动）: %v", serr)
		}
	}

	logx.Infof("启动模型重放完成: 注册类型 %d 个, modelID=%s", len(rts), modelID)
	return nil
}
