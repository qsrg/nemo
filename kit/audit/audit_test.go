// audit_test.go 覆盖 kit/audit 审计先行写路径模板：成功链路（pending →
// write → committed）、写失败链路（pending → failed + 503 AUTHZ_SERVICE_
// UNAVAILABLE）、引擎校验拒绝（Invalid tuple → 400 AUTHZ_ENGINE_REJECTED）、
// 审计落库失败（500 AUTHZ_INTERNAL，不执行 write）。
package audit

import (
	"context"
	"errors"
	"net/http"
	"testing"

	kiterrors "nemo/kit/errors"
)

// fakeSink 是 Sink 的测试替身：内存记录三态调用，可注入故障。
type fakeSink struct {
	entries   []Entry
	committed []int64
	failed    map[int64]string
	createErr error
	markErr   error
}

// CreatePending 记录条目并返回序号 ID；createErr 非空时模拟落库故障。
func (f *fakeSink) CreatePending(_ context.Context, e Entry) (int64, error) {
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.entries = append(f.entries, e)
	return int64(len(f.entries)), nil
}

// MarkCommitted 记录提交；markErr 非空时模拟标记故障（不影响主流程）。
func (f *fakeSink) MarkCommitted(_ context.Context, id int64) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.committed = append(f.committed, id)
	return nil
}

// MarkFailed 记录失败原因。
func (f *fakeSink) MarkFailed(_ context.Context, id int64, reason string) error {
	if f.failed == nil {
		f.failed = make(map[int64]string)
	}
	f.failed[id] = reason
	return nil
}

// TestWriteWithAuditSuccess 成功链路：pending 先落、write 执行、committed
// 标记、返回 nil；快照为入参 JSON 文本。
func TestWriteWithAuditSuccess(t *testing.T) {
	sink := &fakeSink{}
	called := false
	err := WriteWithAudit(context.Background(), sink, "zhangsan", "create_topic", "topic:name:t1",
		map[string]any{"name": "t1"}, func() error {
			called = true
			return nil
		})
	if err != nil || !called {
		t.Fatalf("成功场景: err=%v called=%v", err, called)
	}
	if len(sink.entries) != 1 || len(sink.committed) != 1 {
		t.Fatalf("三态调用错误: entries=%d committed=%d", len(sink.entries), len(sink.committed))
	}
	e := sink.entries[0]
	if e.Operator != "zhangsan" || e.Action != "create_topic" || e.Target != "topic:name:t1" {
		t.Errorf("条目字段错误: %+v", e)
	}
	if e.Snapshot != `{"name":"t1"}` {
		t.Errorf("快照错误: %q", e.Snapshot)
	}
}

// TestWriteWithAuditWriteFail 写失败：条目标 failed（附原因），返回 503
// AUTHZ_SERVICE_UNAVAILABLE（fail-closed）。
func TestWriteWithAuditWriteFail(t *testing.T) {
	sink := &fakeSink{}
	err := WriteWithAudit(context.Background(), sink, "zhangsan", "update_topic", "kafka_topic:1",
		nil, func() error { return errors.New("connection refused") })
	var ke *kiterrors.Error
	if !errors.As(err, &ke) {
		t.Fatalf("应返回 *kiterrors.Error: %v", err)
	}
	if ke.Status != http.StatusServiceUnavailable || ke.Code != "AUTHZ_SERVICE_UNAVAILABLE" {
		t.Errorf("写失败应 503/AUTHZ_SERVICE_UNAVAILABLE: status=%d code=%q", ke.Status, ke.Code)
	}
	if len(sink.committed) != 0 || len(sink.failed) != 1 {
		t.Fatalf("失败链路三态错误: committed=%d failed=%d", len(sink.committed), len(sink.failed))
	}
	if reason := sink.failed[1]; reason != "connection refused" {
		t.Errorf("failed 原因应落库: %q", reason)
	}
}

// TestWriteWithAuditEngineRejected 引擎校验拒绝（Invalid tuple）→ 400
// AUTHZ_ENGINE_REJECTED 带真实原因（请求/配置问题，不按服务故障误报 503）。
func TestWriteWithAuditEngineRejected(t *testing.T) {
	sink := &fakeSink{}
	err := WriteWithAudit(context.Background(), sink, "zhangsan", "write_relations", "unknown",
		nil, func() error { return errors.New(`400 Bad Request: Invalid tuple 'user:x owner org:y'`) })
	var ke *kiterrors.Error
	if !errors.As(err, &ke) {
		t.Fatalf("应返回 *kiterrors.Error: %v", err)
	}
	if ke.Status != http.StatusBadRequest || ke.Code != "AUTHZ_ENGINE_REJECTED" {
		t.Errorf("引擎拒绝应 400/AUTHZ_ENGINE_REJECTED: status=%d code=%q", ke.Status, ke.Code)
	}
}

// TestWriteWithAuditPendingFail 审计落库失败 → 500 AUTHZ_INTERNAL，
// write 不执行（不留无痕变更）。
func TestWriteWithAuditPendingFail(t *testing.T) {
	sink := &fakeSink{createErr: errors.New("db down")}
	called := false
	err := WriteWithAudit(context.Background(), sink, "zhangsan", "create_topic", "topic:name:t1",
		nil, func() error {
			called = true
			return nil
		})
	var ke *kiterrors.Error
	if !errors.As(err, &ke) {
		t.Fatalf("应返回 *kiterrors.Error: %v", err)
	}
	if ke.Status != http.StatusInternalServerError || ke.Code != "AUTHZ_INTERNAL" {
		t.Errorf("审计失败应 500/AUTHZ_INTERNAL: status=%d code=%q", ke.Status, ke.Code)
	}
	if called {
		t.Errorf("审计落库失败时 write 不应执行")
	}
}

// TestWriteWithAuditMarkFailNonBlocking 状态标记故障不阻断主流程：
// write 成功、MarkCommitted 报错 → 仍返回 nil（标记只影响审计展示）。
func TestWriteWithAuditMarkFailNonBlocking(t *testing.T) {
	sink := &fakeSink{markErr: errors.New("mark down")}
	err := WriteWithAudit(context.Background(), sink, "zhangsan", "create_topic", "topic:name:t1",
		nil, func() error { return nil })
	if err != nil {
		t.Fatalf("标记故障不应阻断成功路径: %v", err)
	}
}
