// features_test.go —— OpenFGA 常用能力探针(能力封测,非业务模型):
// 用独立测试模型逐项验证内嵌引擎支持的功能面,结论作为 Phase C 功能规划的依据。
// 覆盖:公开访问(user:*)、排除(but not)、条件(ABAC condition)、上下文元组
// (contextual tuples)。条件与上下文元组经底层官方 client 直探——Handle.Check
// 有意不透出这两类参数(平台当前无此需求,引擎能力留档,启用时零 surprises)。
package engine

import (
	"context"
	_ "embed"
	"fmt"
	"testing"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

//go:embed testdata/features_model.fga
var featuresModelDSL string

// startFeaturesEngine 启动独立引擎 + 建独立 store + 写探针模型,返回 Handle。
func startFeaturesEngine(t *testing.T) *Handle {
	t.Helper()
	h, err := Start(context.Background(), Options{
		HTTPAddr:  "127.0.0.1:0",
		StoreName: fmt.Sprintf("features-probe-%d", time.Now().UnixNano()),
		Datastore: "memory",
	})
	if err != nil {
		t.Fatalf("引擎启动失败: %v", err)
	}
	t.Cleanup(h.Stop)
	if _, _, err := h.EnsureModel(context.Background(), featuresModelDSL); err != nil {
		t.Fatalf("探针模型解析失败: %v", err)
	}
	return h
}

// rawClient 构造直连引擎的官方 SDK client(探针专用,绕过 Handle 的能力裁剪)。
func rawClient(t *testing.T, h *Handle) *client.OpenFgaClient {
	t.Helper()
	cc, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               h.APIURL(),
		StoreId:              h.StoreID(),
		AuthorizationModelId: h.ModelID(),
	})
	if err != nil {
		t.Fatalf("探针 client 构造失败: %v", err)
	}
	return cc
}

// TestFeaturePublicAccess 公开访问:资源对 user:* 开放后,任意用户可读。
func TestFeaturePublicAccess(t *testing.T) {
	h := startFeaturesEngine(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// reader: [user, user:*]——公开访问 = 显式写入 user:* 通配元组(实测语义:
	// 模型写 [user:*] 时仅接受通配元组、拒绝具体用户;两类都要需并列声明)。
	// private 用具体用户授权,验证通配元组不会外溢到其他对象。
	must(h.WriteTuples(ctx, []Tuple{
		{User: "user:*", Relation: "reader", Object: "doc:public"},
		{User: "user:bob", Relation: "reader", Object: "doc:private"},
	}))
	for _, u := range []string{"alice", "bob", "stranger"} {
		ok, err := h.Check(ctx, "user:"+u, "reader", "doc:public")
		must(err)
		if !ok {
			t.Fatalf("公开访问:user:%s 应可读 doc:public", u)
		}
	}
	if ok, _ := h.Check(ctx, "user:alice", "reader", "doc:private"); ok {
		t.Fatal("未授权用户不应读 doc:private")
	}
	if ok, _ := h.Check(ctx, "user:bob", "reader", "doc:private"); !ok {
		t.Fatal("显式授权的 bob 应可读 doc:private")
	}
}

// TestFeatureExclusion 排除(but not):黑名单成员即使持有 base 关系也被拒绝。
func TestFeatureExclusion(t *testing.T) {
	h := startFeaturesEngine(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// viewer = direct but not banned(各自对象独立判定)
	must(h.WriteTuples(ctx, []Tuple{
		{User: "user:alice", Relation: "banned", Object: "doc:x"},
		{User: "user:alice", Relation: "direct", Object: "doc:x"},
		{User: "user:bob", Relation: "direct", Object: "doc:x"},
	}))
	if ok, _ := h.Check(ctx, "user:alice", "viewer", "doc:x"); ok {
		t.Fatal("黑名单成员不应有 viewer(排除未生效)")
	}
	if ok, _ := h.Check(ctx, "user:bob", "viewer", "doc:x"); !ok {
		t.Fatal("非黑名单成员应有 viewer")
	}
}

// TestFeatureConditions 条件授权(ABAC):条件元组写入时不携带存储上下文,
// 求值以 Check 请求的 context 为准(实测:eng→授权,sales/未传→拒绝)。
// Handle.Check 有意不透出条件参数(平台当前无此需求),经底层官方 client 直探。
func TestFeatureConditions(t *testing.T) {
	h := startFeaturesEngine(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cc := rawClient(t, h)
	_, err := cc.Write(ctx).
		Body(client.ClientWriteRequest{
			Writes: []client.ClientTupleKey{{
				User: "user:alice", Relation: "cond_reader", Object: "doc:c",
				Condition: &openfga.RelationshipCondition{Name: "is_employee"},
			}},
		}).
		Execute()
	must(err)
	check := func(cctx map[string]any) bool {
		t.Helper()
		resp, err := cc.Check(ctx).
			Body(client.ClientCheckRequest{
				User: "user:alice", Relation: "cond_reader", Object: "doc:c", Context: &cctx,
			}).
			Execute()
		must(err)
		return resp.GetAllowed()
	}
	if !check(map[string]any{"department": "eng"}) {
		t.Fatal("条件满足(eng)时应授权")
	}
	if check(map[string]any{"department": "sales"}) {
		t.Fatal("条件不满足(sales)时应拒绝")
	}
}

// TestFeatureContextualTuples 上下文元组:check 时临时附加元组,不落存储。
func TestFeatureContextualTuples(t *testing.T) {
	h := startFeaturesEngine(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// 注意:本版本(引擎 v1.19)上下文元组与通配类型限制(reader:[user:*])组合
	// 会报 2027"type not allowed"——上下文元组请用在显式类型限制的关系上(实测)
	if ok, _ := h.Check(ctx, "user:temp", "direct", "doc:t"); ok {
		t.Fatal("上下文元组不应落库:附加前应拒绝")
	}
	cc := rawClient(t, h)
	resp, err := cc.Check(ctx).
		Body(client.ClientCheckRequest{
			User: "user:temp", Relation: "direct", Object: "doc:t",
			ContextualTuples: []openfga.TupleKey{{User: "user:temp", Relation: "direct", Object: "doc:t"}},
		}).
		Execute()
	must(err)
	if !resp.GetAllowed() {
		t.Fatal("上下文元组附加后本次 check 应授权")
	}
	if ok, _ := h.Check(ctx, "user:temp", "direct", "doc:t"); ok {
		t.Fatal("上下文元组不应落库:再次普通 check 应拒绝")
	}
}
