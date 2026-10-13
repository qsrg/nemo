// modelgen_test.go 覆盖资源类型 DSL 模板生成器：
//   - 确定性：同参数两次 Render 深比较一致，动作顺序不影响输出；
//   - 白名单矩阵：can_* 出口选配开/关全组合（2^5）产出的块与基础模型合并后
//     均能经 engine.EnsureModel 真解析（DSL 合法）；
//   - 非法配置：空名/非法字符/保留类型名/未知动作/空动作集一律报错，
//     保证非法配置不可能产出越权结构。
package engine

import (
	"os"
	"strings"
	"testing"

	"nemo/services/authz/internal/modelmerge"
)

// renderGolden 是全量出口（五个 can_* 全选）时的精确输出（确定性金样）。
// 2026-09-23：editor 支持组织主体（组织可按资源批量授可编辑）。
// 2026-09-30：can_delete 收口 owner（删除是与编辑独立的授权边界）；
// can_manage_grants 新增出口并固定 owner（名单管理权独立边界）。
// 2026-10-09：owner 支持组织主体（部门可授"可管理"档，成员整体继承）。
const renderGolden = `type report
  relations
    define org: [org]
    define owner: [user, org#member]
    define sharer: [user] or owner or admin from org
    define editor: [user, org#member] or owner
    define viewer: [user, org#member] or editor
    define member_view: viewer or member from org
    define can_view: member_view or owner
    define can_edit: editor
    define can_delete: owner
    define can_share: sharer
    define can_manage_grants: owner`

// TestRenderDeterministic 同参数两次 Render 输出逐字节一致；
// 动作传入顺序不同（乱序/重复）输出仍一致（规范化后生成）。
func TestRenderDeterministic(t *testing.T) {
	full := []string{"can_view", "can_edit", "can_delete", "can_share", "can_manage_grants"}

	first, err := Render(ResourceType{Name: "report", Actions: full})
	if err != nil {
		t.Fatalf("Render 全量出口: %v", err)
	}
	second, err := Render(ResourceType{Name: "report", Actions: full})
	if err != nil {
		t.Fatalf("Render 全量出口(第二次): %v", err)
	}
	if first != second {
		t.Errorf("同参数两次 Render 输出不一致:\n--首次--\n%s\n--二次--\n%s", first, second)
	}
	if first != renderGolden {
		t.Errorf("全量出口输出与金样不符:\n--实际--\n%s\n--金样--\n%s", first, renderGolden)
	}

	// 乱序 + 重复动作：规范化（去重 + 固定顺序）后输出与有序输入一致。
	shuffled, err := Render(ResourceType{Name: "report", Actions: []string{
		"can_share", "can_view", "can_share", "can_delete", "can_edit", "can_manage_grants",
	}})
	if err != nil {
		t.Fatalf("Render 乱序动作: %v", err)
	}
	if shuffled != first {
		t.Errorf("动作顺序影响输出（违反确定性）:\n--乱序--\n%s\n--有序--\n%s", shuffled, first)
	}
}

// TestRenderWhitelistMatrix can_* 出口选配开/关全组合（2^5 = 32 种）：
// 每种组合 Render 出的块与基础模型（model.fga）合并后经 EnsureModel 真解析，
// 全部必须合法；同时断言块内只出现勾选的 can_* 出口。
func TestRenderWhitelistMatrix(t *testing.T) {
	base, err := os.ReadFile("../../model.fga")
	if err != nil {
		t.Fatalf("读取 model.fga: %v", err)
	}

	h, err := Start(t.Context(), Options{HTTPAddr: "127.0.0.1:0", StoreName: "modelgen-matrix"})
	if err != nil {
		t.Fatalf("启动引擎: %v", err)
	}
	t.Cleanup(h.Stop)

	exits := []string{"can_view", "can_edit", "can_delete", "can_share", "can_manage_grants"}
	for mask := range 1 << len(exits) {
		if mask == 0 {
			continue // 空动作集非法，已在 TestRenderInvalidConfigs 覆盖
		}
		var actions []string
		for i, e := range exits {
			if mask&(1<<i) != 0 {
				actions = append(actions, e)
			}
		}
		block, err := Render(ResourceType{Name: "report", Actions: actions})
		if err != nil {
			t.Fatalf("组合 %04b Render: %v", mask, err)
		}

		// 块内 can_* 出口必须与勾选集完全一致（不多不少）。
		for _, e := range exits {
			want := strings.Contains(block, "define can_"+strings.TrimPrefix(e, "can_")+":")
			got := strings.Contains(block, "define "+e+":")
			if want != got {
				t.Errorf("组合 %04b 块内出口 %s 出现=%v, 期望=%v\n%s", mask, e, got, want, block)
			}
		}

		// 真解析：合并 DSL 能写入引擎即合法（transformer 语法+语义校验）。
		// v3 起基准模型末尾带 condition 块（FGA DSL 要求 condition 在全部
		// type 之后），注册块须经 modelmerge.MergeBlocks 插到 condition 之前
		//（与生产注册/重放路径同一合并函数）。
		merged := modelmerge.MergeBlocks(string(base), []string{block})
		if _, _, err := h.EnsureModel(t.Context(), merged); err != nil {
			t.Errorf("组合 %04b 合并 DSL 解析失败: %v\n%s", mask, err, merged)
		}
	}
}

// TestRenderInvalidConfigs 非法配置一律报错（fail-closed）：
// 空名/大写或非法字符/保留类型名/未知动作/空动作集都不可能产出 DSL 块。
func TestRenderInvalidConfigs(t *testing.T) {
	cases := []struct {
		name    string
		cfg     ResourceType
		wantSub string // 错误信息应包含的关键字
	}{
		{"空类型名", ResourceType{Actions: []string{"can_view"}}, "类型名"},
		{"大写字母", ResourceType{Name: "Report", Actions: []string{"can_view"}}, "类型名"},
		{"数字开头", ResourceType{Name: "1report", Actions: []string{"can_view"}}, "类型名"},
		{"连字符", ResourceType{Name: "my-report", Actions: []string{"can_view"}}, "类型名"},
		{"保留名 user", ResourceType{Name: "user", Actions: []string{"can_view"}}, "保留"},
		{"保留名 org", ResourceType{Name: "org", Actions: []string{"can_view"}}, "保留"},
		{"保留名 platform_role", ResourceType{Name: "platform_role", Actions: []string{"can_view"}}, "保留"},
		{"保留名 order", ResourceType{Name: "order", Actions: []string{"can_view"}}, "保留"},
		{"未知动作", ResourceType{Name: "report", Actions: []string{"can_fly"}}, "动作"},
		{"空动作集", ResourceType{Name: "report", Actions: nil}, "动作"},
	}
	for _, tc := range cases {
		_, err := Render(tc.cfg)
		if err == nil {
			t.Errorf("%s: 应报错, 实际成功", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Errorf("%s: 错误信息 %q 应包含 %q", tc.name, err.Error(), tc.wantSub)
		}
	}
}

// TestExtractActions 从已渲染块反解动作码清单（列表接口回填 Actions 用）：
// 输出按模板固定顺序（与入参顺序无关）。
func TestExtractActions(t *testing.T) {
	block, err := Render(ResourceType{Name: "report", Actions: []string{
		"can_share", "can_view", "can_delete",
	}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := ExtractActions(block)
	want := []string{"can_view", "can_delete", "can_share"}
	if len(got) != len(want) {
		t.Fatalf("ExtractActions = %v, 期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ExtractActions[%d] = %q, 期望 %q", i, got[i], want[i])
		}
	}
}
