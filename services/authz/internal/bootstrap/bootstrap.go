// Package bootstrap 幂等引导授权引擎：装配授权模型（model.fga）、
// 写入根组织与超管初始元组。所有幂等性依托 engine.Handle 已有能力——
// EnsureModel 按名找 Store 无则建、WriteTuples 读后写跳过已存在元组——
// 本包只做编排，不重复实现幂等检查逻辑。
package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/services/authz/internal/engine"
)

// 初始主体/对象标识（默认值；超管声明已配置化，见 Options/DefaultAdminUser）。
const (
	// DefaultAdminUser 是初始超管默认登录名：配置未声明 AdminUsers 时
	// 使用，保证既有环境（yaml 无 Bootstrap 段）向后兼容——admin 仍为初始超管。
	DefaultAdminUser = "admin"
	// RootOrg 是根组织。
	RootOrg = "org:root"
	// AdminAllPerm 是超管全通配权限点对象（permission:*.*，码 = *:*）：
	// 引导时授超管的 can 元组，超管天然持有全部权限码——去角色化后权限码
	// 唯一来源是 permission.can 元组，超管不引导即无码（鸡蛋死锁）。
	// 注意：引擎对象 id 禁含冒号（permission 侧码内冒号写为点，permCode
	// 还原），纯 * 或 *:* 形态被 OpenFGA 判为 typed wildcard/非法格式直接
	// 拒绝；*.* 是普通 id（同 *:read 的存储形态 *.read），permCode 还原为
	// 码 *:*，PermGranted/hasPermission 的 *:* 分支匹配任意 <服务>:<动作>。
	AdminAllPerm = "permission:*.*"
)

// ModelFile 是模型 DSL 文件路径（相对进程工作目录）。
// 默认与服务主入口的 etc/authz-api.yaml 同约定：工作目录为 services/authz；
// 测试可覆写此变量指向仓库内其他位置。
var ModelFile = "model.fga"

// Options 是引导（超管自举）配置，由主入口从 etc 配置文件注入：
// 声明初始超管用户与超管组织（可多个）。引导只做初始装配，撤权走
// 控制台授权页面——配置移除不会自动回收已写元组（WriteTuples 只增不删）。
type Options struct {
	// AdminUsers 超管用户登录名；为空时默认 [DefaultAdminUser]。
	AdminUsers []string
	// AdminOrgs 超管组织编码；对该组织成员集合（org:{编码}#member）
	// 授全通配权限，组织下全部成员继承超管。
	AdminOrgs []string
}

// Run 幂等执行引导，任一步失败即返回错误：
//  1. EnsureModel 装配 model.fga（Store 按名幂等创建/查找）；
//  2. 写根组织初始元组（org:root 的 member 为各超管用户；WriteTuples 读后写幂等）；
//  3. 写超管全通配权限元组——权限码是去角色化后鉴权的唯一真源：
//     每个超管用户授 permission:*.*#can@user:{名}；每个超管组织授
//     permission:*.*#can@org:{编码}#member（组织成员集合继承超管）。
//     超管引导即全权（*:* 码经 PermGranted/hasPermission 匹配任意
//     <服务>:<动作>），避免"admin 无码进不了授权页、没人能给 admin 授权"
//     的鸡蛋死锁；
//  4. 日志输出 storeID/modelID（后续任务经 Handle.StoreID/ModelID 承接状态）。
func Run(ctx context.Context, h *engine.Handle, opts Options) error {
	if h == nil {
		return fmt.Errorf("engine Handle 为 nil")
	}

	// 读取模型 DSL 全文（入口与服务主程序同约定：工作目录为 services/authz）。
	dsl, err := os.ReadFile(ModelFile)
	if err != nil {
		return fmt.Errorf("读取模型 DSL(%s): %w", ModelFile, err)
	}

	// ① 装配模型：DSL 解析校验 + Store 按名幂等创建。
	storeID, modelID, err := h.EnsureModel(ctx, string(dsl))
	if err != nil {
		return fmt.Errorf("装配授权模型: %w", err)
	}

	// ②③ 初始元组（WriteTuples 读后写，重复执行不重复提交）：
	//   - 根组织成员（各超管用户 member org:root，组织继承语义原生生效）；
	//   - 超管全通配权限（超管用户 can permission:*.*；超管组织成员集合
	//     can permission:*.*，码 *:* 匹配任意 服务:动作）。
	users := opts.AdminUsers
	if len(users) == 0 {
		// 配置未声明 → 默认超管（向后兼容既有环境）。
		users = []string{DefaultAdminUser}
	}
	tuples := make([]engine.Tuple, 0, len(users)*2+len(opts.AdminOrgs)*2)
	for _, u := range users {
		if u == "" {
			continue
		}
		tuples = append(tuples,
			engine.Tuple{User: "user:" + u, Relation: "member", Object: RootOrg},
			engine.Tuple{User: "user:" + u, Relation: "can", Object: AdminAllPerm},
		)
	}
	for _, o := range opts.AdminOrgs {
		if o == "" {
			continue
		}
		// 组织维度超管：成员集合主体 org:{编码}#member（DSL can: [user, org#member]）。
		// 组织层级（parent/member）由 CAS 组织同步维护，引导不写组织挂接元组。
		tuples = append(tuples, engine.Tuple{User: "org:" + o + "#member", Relation: "can", Object: AdminAllPerm})
	}
	if err := h.WriteTuples(ctx, tuples); err != nil {
		return fmt.Errorf("写初始元组（根组织/超管权限）: %w", err)
	}

	// ④ 输出 store/model id，供后续任务状态承接与运维排查。
	logx.Infof("bootstrap 完成: storeID=%s modelID=%s, 超管用户=%v, 超管组织=%v, %s#can@%s",
		storeID, modelID, users, opts.AdminOrgs, AdminAllPerm, RootOrg)
	return nil
}
