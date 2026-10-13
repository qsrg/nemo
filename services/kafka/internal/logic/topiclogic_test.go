// topiclogic_test.go 覆盖 Topic 列表分页优化（列谓词下推 + 引擎逐行复核）
// 与创建路径的归属挂接：
//   - repo.Page：All/归属列/直接授予 ID 三路谓词、分页边界、total；
//   - 列表：manage 视角全量翻页；普通用户谓词 = 部门归属 ∪ 直接授予；
//     引擎复核（can_view Check）过滤列漂移行；
//   - 创建：自动挂创建人部门（org_id 列 + owner/org 元组双写）、重名 409
//     （uk_name 点查预检，无全表扫）。
//
// 鉴权依赖经 httptest 桩服务承接（kit/authz Remote 的三个端点），身份用
// Authorization: Bearer token-<sub> 透传——与生产 Identity 中间件注入形态一致。
package logic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	authzkit "nemo/kit/authz"
	"nemo/services/kafka/internal/config"
	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	kiterrors "nemo/kit/errors"
)

// stubAuthz 是集中式 authz 的测试桩：按 (user, type, relation) 返回 list-objects
// 集合，按 (user, object, relation) 返回 check 结果（缺省 checkDefault），
// relations 写入全部记录到 written（含创建路径的 owner/org 元组断言）。
type stubAuthz struct {
	t *testing.T
	// lists：user|type|relation → 对象串列表。
	lists map[string][]string
	// checks：user|object|relation → allowed（未命中取 checkDefault）。
	checks    map[string]bool
	checkDeny map[string]bool // 命中即 false（显示拒绝，优先于 checkDefault）
	// written：relations 写入的全部元组。
	written []authzkit.Tuple
	server  *httptest.Server
}

// newStubAuthz 启动桩服务（t.Cleanup 自动关闭）。
func newStubAuthz(t *testing.T) *stubAuthz {
	t.Helper()
	s := &stubAuthz{
		t:         t,
		lists:     map[string][]string{},
		checks:    map[string]bool{},
		checkDeny: map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/authz/list-objects", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Type, Relation string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"objects": s.lists[userOf(r)+"|"+in.Type+"|"+in.Relation],
		})
	})
	mux.HandleFunc("/v1/authz/check", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Object, Relation string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		key := userOf(r) + "|" + in.Object + "|" + in.Relation
		allowed := true
		if s.checks[key] {
			allowed = true
		}
		if s.checkDeny[key] {
			allowed = false
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowed})
	})
	// 批量判定桩：逐条复用 check 语义（deny 命中即 false，缺省 allow），
	// results 与请求 items 下标一一对应（kit/authz Remote.BatchCheck 契约）。
	mux.HandleFunc("/v1/authz/batch-check", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items []struct{ Object, Relation string } `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		results := make([]bool, len(in.Items))
		for i, it := range in.Items {
			key := userOf(r) + "|" + it.Object + "|" + it.Relation
			results[i] = !s.checkDeny[key]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("/v1/authz/relations", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Tuples []authzkit.Tuple `json:"tuples"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		s.written = append(s.written, in.Tuples...)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"written": len(in.Tuples)})
	})
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

// userOf 从请求头解析桩约定的 token（token-<sub>）。
func userOf(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	return strings.TrimPrefix(auth, "Bearer token-")
}

// grantList 预置一个 list-objects 结果。
func (s *stubAuthz) grantList(user, objType, relation string, objects ...string) {
	s.lists[user+"|"+objType+"|"+relation] = objects
}

// newTestSvc 装配指向桩服务的 ServiceContext（内存 repo）。
func newTestSvc(t *testing.T, s *stubAuthz) *svc.ServiceContext {
	t.Helper()
	var c config.Config
	c.Authz.BaseURL = s.server.URL
	return svc.NewServiceContext(c)
}

// userCtx 构造带身份与 token 的 ctx（与生产 Identity 中间件注入形态一致）。
func userCtx(sub string) context.Context {
	return authzkit.WithToken(authzkit.WithIdentity(context.Background(), sub, nil), "token-"+sub)
}

// mustCreate 直接经 repo 建 Topic（测试脚手架，绕过 logic 的判定链路）。
func mustCreate(t *testing.T, sc *svc.ServiceContext, name, orgID, creator string) repo.Topic {
	t.Helper()
	created, err := sc.TopicRepo.Create(context.Background(), repo.Topic{
		Name: name, Env: "prod", OrgID: orgID, CreatedBy: creator, UpdateBy: creator, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("建 Topic %s: %v", name, err)
	}
	return created
}

// TestTopicPageRepoFilter repo.Page 三路谓词与分页边界（内存实现）。
func TestTopicPageRepoFilter(t *testing.T) {
	sc := newTestSvc(t, newStubAuthz(t))
	// 5 条：3 条归 tech、1 条归 pay、1 条无归属。
	mustCreate(t, sc, "t1", "tech", "a")
	mustCreate(t, sc, "t2", "tech", "a")
	mustCreate(t, sc, "t3", "tech", "b")
	pay := mustCreate(t, sc, "t4", "pay", "b")
	mustCreate(t, sc, "t5", "", "c")

	// All 视角：全量 5 条，分页翻页。
	items, total, err := sc.TopicRepo.Page(context.Background(), repo.TopicPageFilter{All: true, Page: 2, PageSize: 2})
	if err != nil || total != 5 || len(items) != 2 {
		t.Fatalf("All 第 2 页应 2 条/共 5: %d %d %v", len(items), total, err)
	}

	// 归属谓词：tech 部门 → 3 条。
	_, total, err = sc.TopicRepo.Page(context.Background(), repo.TopicPageFilter{OrgIDs: []string{"tech"}, PageSize: 10})
	if err != nil || total != 3 {
		t.Fatalf("tech 归属应 3 条: %d %v", total, err)
	}

	// 直接授予谓词：仅 t4 的 ID。
	items, total, err = sc.TopicRepo.Page(context.Background(), repo.TopicPageFilter{IDs: []int64{pay.ID}, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].Name != "t4" {
		t.Fatalf("ID 谓词应仅 t4: %+v %d %v", items, total, err)
	}

	// OR 组合：tech 归属 ∪ t4 ID → 4 条。
	_, total, err = sc.TopicRepo.Page(context.Background(), repo.TopicPageFilter{
		OrgIDs: []string{"tech"}, IDs: []int64{pay.ID}, PageSize: 10,
	})
	if err != nil || total != 4 {
		t.Fatalf("组合谓词应 4 条: %d %v", total, err)
	}

	// 两路皆空（非 All）：无可见 → 0 条（不退化为全量）。
	_, total, err = sc.TopicRepo.Page(context.Background(), repo.TopicPageFilter{PageSize: 10})
	if err != nil || total != 0 {
		t.Fatalf("空谓词应 0 条: %d %v", total, err)
	}

	// 越界页：空 items、total 仍准确。
	items, total, err = sc.TopicRepo.Page(context.Background(), repo.TopicPageFilter{All: true, Page: 99, PageSize: 10})
	if err != nil || total != 5 || len(items) != 0 {
		t.Fatalf("越界页应空 items: %d %d %v", len(items), total, err)
	}
}

// TestListTopicsPagedPredicate 列表分页 + 谓词 + 徽标：
//   - manage 持有者全量翻页（无谓词）；
//   - 普通用户谓词 = 部门归属 ∪ 直接授予（owner/viewer），只看到自己的行。
func TestListTopicsPagedPredicate(t *testing.T) {
	stub := newStubAuthz(t)
	sc := newTestSvc(t, stub)
	mustCreate(t, sc, "t1", "tech", "mgr")
	mustCreate(t, sc, "t2", "tech", "mgr")
	mustCreate(t, sc, "t3", "pay", "mgr")
	t4 := mustCreate(t, sc, "t4", "", "mgr") // 无归属，仅直接授予可见

	// mgr：manage 码 + 全部 owner（创建人）。
	stub.grantList("mgr", "permission", "can", "permission:kafka-topics.manage")
	stub.grantList("mgr", "kafka_topic", "owner", "kafka_topic:1", "kafka_topic:2", "kafka_topic:3", "kafka_topic:4")

	// u1：tech 部门成员 + t4 的 viewer。
	stub.grantList("u1", "org", "member", "org:tech")
	stub.grantList("u1", "kafka_topic", "owner")
	stub.grantList("u1", "kafka_topic", "editor")
	stub.grantList("u1", "kafka_topic", "viewer", "kafka_topic:"+strconv.FormatInt(t4.ID, 10))

	// mgr 第 1 页（pageSize 3）：全量 4 条中的前 3 条。
	resp, err := NewListKafkaTopicsLogic(userCtx("mgr"), sc).ListKafkaTopics(&types.ListTopicsReq{Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("mgr 列表: %v", err)
	}
	if resp.Total != 4 || len(resp.Items) != 3 {
		t.Fatalf("mgr 全量第 1 页应 3/4: %d %d", len(resp.Items), resp.Total)
	}
	if !resp.Items[0].CanEdit || !resp.Items[0].Owner {
		t.Errorf("mgr（owner+manage）徽标应可编辑可删")
	}

	// u1：tech 归属（t1/t2）∪ viewer（t4）= 3 条，pay 的 t3 不可见。
	resp, err = NewListKafkaTopicsLogic(userCtx("u1"), sc).ListKafkaTopics(&types.ListTopicsReq{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("u1 列表: %v", err)
	}
	seen := map[string]bool{}
	for _, it := range resp.Items {
		seen[it.Name] = true
		if it.Name == "t3" {
			t.Errorf("u1 不应看到 pay 归属的 t3")
		}
	}
	if !seen["t1"] || !seen["t2"] || !seen["t4"] {
		t.Errorf("u1 应见 t1/t2(归属)/t4(viewer): %v", seen)
	}
	if resp.Total != 3 {
		t.Errorf("u1 total 应 3: %d", resp.Total)
	}
	// 徽标：t1 非 owner 非 editor → 只读；t4 是 viewer 非 editor → 只读。
	for _, it := range resp.Items {
		if it.CanEdit || it.Owner {
			t.Errorf("u1 对 %s 不应可编辑/owner", it.Name)
		}
	}
}

// TestListTopicsEngineVerify 引擎复核：列谓词带出的行若 Check(can_view)
// 拒绝（列漂移或 ABAC 门槛），页内滤除——引擎是出口真源。
func TestListTopicsEngineVerify(t *testing.T) {
	stub := newStubAuthz(t)
	sc := newTestSvc(t, stub)
	mustCreate(t, sc, "t1", "tech", "mgr")
	mustCreate(t, sc, "t2", "tech", "mgr")

	stub.grantList("u1", "org", "member", "org:tech")
	// 列说可见（tech 归属），引擎说 t2 不可见（模拟 ABAC deny / 列漂移）。
	stub.checkDeny["u1|kafka_topic:2|can_view"] = true

	resp, err := NewListKafkaTopicsLogic(userCtx("u1"), sc).ListKafkaTopics(&types.ListTopicsReq{PageSize: 10})
	if err != nil {
		t.Fatalf("u1 列表: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Name != "t1" {
		t.Fatalf("引擎复核应滤除 t2: %+v", resp.Items)
	}
	// total 为谓词口径近似值（复核不回减——注释钉死的折衷）。
	if resp.Total != 2 {
		t.Errorf("total 应为谓词口径 2（近似值）: %d", resp.Total)
	}
}

// TestCreateTopicOrgAttach 创建归属挂接：org_id 列取创建人部门，
// owner + org 元组双写；重名 409（点查预检）。
func TestCreateTopicOrgAttach(t *testing.T) {
	stub := newStubAuthz(t)
	sc := newTestSvc(t, stub)
	// 中间件成员 mgro 所属部门 middleware。
	stub.grantList("mgro", "org", "member", "org:middleware", "org:platform")

	resp, err := NewCreateKafkaTopicLogic(userCtx("mgro"), sc).CreateKafkaTopic(&types.CreateKafkaTopicReq{
		Name: "order-events", Env: "prod", Description: "d",
	})
	if err != nil {
		t.Fatalf("创建: %v", err)
	}
	if resp.OrgId != "middleware" {
		t.Errorf("归属应取创建人首个部门 middleware: %q", resp.OrgId)
	}
	// 元组双写：owner（创建人）+ org（归属挂接）。
	hasOwner, hasOrg := false, false
	for _, tp := range stub.written {
		if tp.Relation == "owner" && tp.User == "user:mgro" && tp.Object == "kafka_topic:1" {
			hasOwner = true
		}
		if tp.Relation == "org" && tp.User == "org:middleware" && tp.Object == "kafka_topic:1" {
			hasOrg = true
		}
	}
	if !hasOwner || !hasOrg {
		t.Errorf("元组双写缺失（owner=%v org=%v）: %+v", hasOwner, hasOrg, stub.written)
	}

	// 重名 → 409（GetByName 点查预检，无全表扫）。
	_, err = NewCreateKafkaTopicLogic(userCtx("mgro"), sc).CreateKafkaTopic(&types.CreateKafkaTopicReq{Name: "order-events"})
	if e := kiterrors.From(err); e == nil || e.Status != 409 {
		t.Errorf("重名应 409: %v", err)
	}
}

// TestExportTopicsCSV 导出清单：内容 = 调用者可见行（与列表同口径），
// 功能码守卫在路由层（kit RequireCode 已有覆盖），本测试验证业务口径。
func TestExportTopicsCSV(t *testing.T) {
	stub := newStubAuthz(t)
	sc := newTestSvc(t, stub)
	mustCreate(t, sc, "t1", "tech", "mgr")
	mustCreate(t, sc, "t2", "pay", "mgr")

	// u1：tech 部门 → 只见 t1。
	stub.grantList("u1", "org", "member", "org:tech")
	csv, err := NewExportKafkaTopicsLogic(userCtx("u1"), sc).ExportKafkaTopics(&types.ExportTopicsReq{})
	if err != nil {
		t.Fatalf("导出: %v", err)
	}
	if !strings.Contains(csv, "t1,prod,tech,mgr") || strings.Contains(csv, "t2") {
		t.Errorf("导出应仅含可见行 t1: %q", csv)
	}
	if !strings.HasPrefix(csv, "id,name,env,org_id,created_by") {
		t.Errorf("CSV 应有表头: %q", csv)
	}
}

// TestResetOffsetTwoLayerGate 重置位点两层判定：动作码（路由层）之外的
// 对象权 can_edit——无编辑权 403、有编辑权通过（demo 受理）。
func TestResetOffsetTwoLayerGate(t *testing.T) {
	stub := newStubAuthz(t)
	sc := newTestSvc(t, stub)
	tp := mustCreate(t, sc, "t1", "tech", "mgr")
	id := strconv.FormatInt(tp.ID, 10)

	// u1：可见（tech 部门）但不可编辑（can_edit 拒绝）→ 403。
	stub.grantList("u1", "org", "member", "org:tech")
	stub.checkDeny["u1|kafka_topic:"+id+"|can_edit"] = true
	_, err := NewResetKafkaTopicOffsetLogic(userCtx("u1"), sc).ResetKafkaTopicOffset(&types.TopicIdReq{Id: id})
	if e := kiterrors.From(err); e == nil || e.Status != 403 {
		t.Fatalf("无编辑权应 403: %v", err)
	}

	// u2：editor → 受理成功。
	stub.grantList("u2", "kafka_topic", "editor", "kafka_topic:"+id)
	resp, err := NewResetKafkaTopicOffsetLogic(userCtx("u2"), sc).ResetKafkaTopicOffset(&types.TopicIdReq{Id: id})
	if err != nil || !resp.Reset {
		t.Fatalf("editor 重置应受理: %v %+v", err, resp)
	}
}
