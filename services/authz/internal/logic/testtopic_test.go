// testtopic_test.go 提供 ABAC/名单测试共用的 kafka_topic 测试资源脚手架。
//
// 2026-09-23 拆分说明：kafka 业务逻辑（CreateKafkaTopicLogic 等）已随
// services/kafka 拆出独立服务，authz 侧不再有 TopicRepo/创建逻辑；本助手
// 直接构造"已创建 Topic"的引擎状态（分配唯一 ID + 写 owner 元组），供
// ABAC 叠加与通用名单（/v1/authz/relations）判定测试使用，语义与生产
// 创建动作等价（创建即 owner）。
package logic

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"

	"nemo/services/authz/internal/engine"
	"nemo/services/authz/internal/svc"
)

// testTopicRef 是测试资源的最小引用（仅暴露引擎对象 id 字符串）。
type testTopicRef struct {
	Id string
}

// testTopicSeq 分配唯一的测试资源 ID（跨用例递增，避免引擎元组冲突）。
var testTopicSeq atomic.Int64

// createTopicViaLogic 构造一个已创建的 kafka_topic 测试资源：分配唯一 ID
// 并写入 owner 元组（admin 为创建人）。返回引用供名单/ABAC 断言使用。
func createTopicViaLogic(t *testing.T, sc *svc.ServiceContext, name string) *testTopicRef {
	t.Helper()
	id := testTopicSeq.Add(1)
	object := "kafka_topic:" + strconv.FormatInt(id, 10)
	if err := sc.Engine.WriteTuples(context.Background(), []engine.Tuple{{
		User:     "user:admin",
		Relation: "owner",
		Object:   object,
	}}); err != nil {
		t.Fatalf("写 owner 元组 %s: %v", object, err)
	}
	return &testTopicRef{Id: strconv.FormatInt(id, 10)}
}
