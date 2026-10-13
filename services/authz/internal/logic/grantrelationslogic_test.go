package logic

import (
	"reflect"
	"testing"

	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/types"
)

// TestExportToRelations 验证注册类型 can_* 出口 → 可授基础关系档位的派生：
// 固定顺序 viewer/editor/owner/sharer；can_delete 与 can_manage_grants 均收口
// owner；can_share 独立为 sharer 档。
func TestExportToRelations(t *testing.T) {
	cases := []struct {
		name    string
		actions []string
		want    []string
	}{
		{"仅查看（只读资源/集群）", []string{"can_view"}, []string{"viewer"}},
		{"查看+编辑（无删除出口）", []string{"can_view", "can_edit"}, []string{"viewer", "editor"}},
		{"kafka_topic：查看+编辑+删除", []string{"can_view", "can_edit", "can_delete"}, []string{"viewer", "editor", "owner"}},
		{"名单管理出口同样派生 owner 档", []string{"can_view", "can_manage_grants"}, []string{"viewer", "owner"}},
		{"全出口", []string{"can_view", "can_edit", "can_delete", "can_share", "can_manage_grants"}, []string{"viewer", "editor", "owner", "sharer"}},
		{"无出口不崩（空切片而非 nil，JSON 落 []）", nil, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := exportToRelations(c.actions)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("actions=%v → got %v, want %v", c.actions, got, c.want)
			}
		})
	}
}

// TestExportToLevels 验证每个档位展开为实际包含的 can_* 出口（继承语义）：
// viewer=can_view；editor=can_view+can_edit；owner=按注册展开主操作全集；
// sharer=仅 can_share（正交）。前端据此展示"档位=哪些按钮"。
func TestExportToLevels(t *testing.T) {
	cases := []struct {
		name    string
		actions []string
		want    []types.GrantLevel
	}{
		{"仅查看", []string{"can_view"}, []types.GrantLevel{
			{Relation: "viewer", Actions: []string{"can_view"}},
		}},
		{"redis_instance：查看+编辑+删除+配名单", []string{"can_view", "can_edit", "can_delete", "can_manage_grants"}, []types.GrantLevel{
			{Relation: "viewer", Actions: []string{"can_view"}},
			{Relation: "editor", Actions: []string{"can_view", "can_edit"}},
			{Relation: "owner", Actions: []string{"can_view", "can_edit", "can_delete", "can_manage_grants"}},
		}},
		{"kafka_topic：无 manage_grants 出口，owner 只含删除", []string{"can_view", "can_edit", "can_delete"}, []types.GrantLevel{
			{Relation: "viewer", Actions: []string{"can_view"}},
			{Relation: "editor", Actions: []string{"can_view", "can_edit"}},
			{Relation: "owner", Actions: []string{"can_view", "can_edit", "can_delete"}},
		}},
		{"带共享档：sharer 仅 can_share", []string{"can_view", "can_share"}, []types.GrantLevel{
			{Relation: "viewer", Actions: []string{"can_view"}},
			{Relation: "sharer", Actions: []string{"can_share"}},
		}},
		{"空 actions", nil, []types.GrantLevel{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := exportToLevels(c.actions, nil)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("actions=%v → got %+v, want %+v", c.actions, got, c.want)
			}
		})
	}
}

// TestBuildActionTitles 验证从菜单声明反查每个 can_* 出口对应的真实按钮名：
// menu 行计入 can_view；button 行按 relation 分组；同名按钮去重；无关类型过滤。
func TestBuildActionTitles(t *testing.T) {
	menus := []repo.Menu{
		{Type: "menu", Title: "Topic 管理", ResourceType: "kafka_topic", Sort: 1},
		{Type: "button", Title: "查看消费者", Relation: "can_view", ResourceType: "kafka_topic", Sort: 2},
		{Type: "button", Title: "编辑 Topic", Relation: "can_edit", ResourceType: "kafka_topic", Sort: 3},
		{Type: "button", Title: "冻结 Topic", Relation: "can_edit", ResourceType: "kafka_topic", Sort: 4},
		{Type: "button", Title: "重置位点", Relation: "can_edit", ResourceType: "kafka_topic", Sort: 5},
		{Type: "button", Title: "删除 Topic", Relation: "can_delete", ResourceType: "kafka_topic", Sort: 6},
		{Type: "button", Title: "配置权限", Relation: "can_manage_grants", ResourceType: "kafka_topic", Sort: 7},
		// 无关类型，应被过滤
		{Type: "button", Title: "查看实例", Relation: "can_view", ResourceType: "redis_instance", Sort: 1},
		// 同名按钮，应去重
		{Type: "button", Title: "编辑 Topic", Relation: "can_edit", ResourceType: "kafka_topic,alias_topic", Sort: 9},
	}
	got := buildActionTitles(menus, "kafka_topic")
	want := map[string][]string{
		"can_view":          {"Topic 管理", "查看消费者"},
		"can_edit":          {"编辑 Topic", "冻结 Topic", "重置位点"},
		"can_delete":        {"删除 Topic"},
		"can_manage_grants": {"配置权限"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
