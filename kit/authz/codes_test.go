// codes_test.go 覆盖 kit/authz 权限码判定件：PermGranted 三级匹配语义
// （表驱动，与前端 permission.test.ts 判定矩阵对齐——尤其钉死"跨服务通配
// 不授予"：持有 redis:* 不得通过 kafka-topics:manage，kafka 旧实现曾在
// 此处越权放行）、EffectiveCodes 对象→码映射与升序去重、CodeGranted 组合。
package authz

import (
	"context"
	"errors"
	"testing"
)

// fakeCodeChecker 是 Checker 的测试替身：按配置返回权限点对象集合。
type fakeCodeChecker struct {
	objects []string
	err     error
}

// Check 测试替身不使用。
func (f *fakeCodeChecker) Check(context.Context, string, string) (bool, error) {
	return false, nil
}

// ListObjects 返回预设的权限点对象集合（或故障）。
func (f *fakeCodeChecker) ListObjects(context.Context, string, string) ([]string, error) {
	return f.objects, f.err
}

// BatchCheck 测试替身不使用。
func (f *fakeCodeChecker) BatchCheck(context.Context, []BatchCheckItem) ([]bool, error) {
	return nil, nil
}

// TestPermGranted 三级匹配语义表驱动：授予/不授予逐例钉死。
func TestPermGranted(t *testing.T) {
	cases := []struct {
		name  string
		code  string
		perms []string
		want  bool
	}{
		// 精确码。
		{"精确码授予", "kafka-topics:manage", []string{"kafka-topics:manage", "redis:read"}, true},
		{"精确码不授予", "kafka-topics:manage", []string{"redis:read"}, false},
		// 段内通配码。
		{"跨服务同动作 *:manage", "kafka-topics:manage", []string{"*:manage"}, true},
		{"本服务跨动作 服务:*", "kafka-topics:manage", []string{"kafka-topics:*"}, true},
		{"全部 *", "kafka-topics:manage", []string{"*"}, true},
		{"全部 *:*", "kafka-topics:manage", []string{"*:*"}, true},
		// 跨服务通配不授予（本语义钉死：与 kafka 旧实现的差异点）。
		{"跨服务 服务:* 不授予", "kafka-topics:manage", []string{"redis:*"}, false},
		{"跨服务 *:其他动作 不授予", "kafka-topics:manage", []string{"*:read"}, false},
		// 纯服务码。
		{"纯服务码授予", "kafka-topics:manage", []string{"kafka-topics"}, true},
		{"其他纯服务码不授予", "kafka-topics:manage", []string{"redis"}, false},
		// code 非法形态（无冒号）：无精确码时不授予（通配/纯服务段均无从
		// 切分）；精确匹配仍授予（与 judgmentcommon 一致——形态校验属
		// 授予入口的 validPermissionCode，不在本函数）。
		{"code 无冒号且无精确码不授予", "manage", []string{"redis", "*:read"}, false},
		{"code 无冒号精确匹配授予", "manage", []string{"manage"}, true},
		// 空集合。
		{"空集合不授予", "kafka-topics:manage", nil, false},
	}
	for _, tc := range cases {
		if got := PermGranted(tc.code, tc.perms); got != tc.want {
			t.Errorf("%s: PermGranted(%q, %v) = %v，期望 %v", tc.name, tc.code, tc.perms, got, tc.want)
		}
	}
}

// TestEffectiveCodes 验证对象→码映射（permission:svc.action → svc:action）、
// 升序去重、无权限空切片、非 permission 前缀对象跳过。
func TestEffectiveCodes(t *testing.T) {
	f := &fakeCodeChecker{objects: []string{
		"permission:redis.read",          // → redis:read
		"permission:kafka-topics.manage", // → kafka-topics:manage
		"permission:redis.read",          // 重复，去重
		"permission:*.read",              // 通配对象 → *:read
		"order:o1",                       // 非 permission 前缀，跳过
	}}
	codes, err := EffectiveCodes(context.Background(), f)
	if err != nil {
		t.Fatalf("EffectiveCodes: %v", err)
	}
	want := []string{"*:read", "kafka-topics:manage", "redis:read"}
	if len(codes) != len(want) {
		t.Fatalf("结果数量错误: %v，期望 %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Errorf("第 %d 个码错误: %q，期望 %q（应升序）", i, codes[i], want[i])
		}
	}

	// 无权限：空切片（非 nil）不报错。
	f2 := &fakeCodeChecker{objects: []string{}}
	codes, err = EffectiveCodes(context.Background(), f2)
	if err != nil || codes == nil || len(codes) != 0 {
		t.Fatalf("空场景: codes=%v err=%v（应空切片非 nil）", codes, err)
	}

	// 判定服务故障透传。
	f3 := &fakeCodeChecker{err: errors.New("connection refused")}
	if _, err = EffectiveCodes(context.Background(), f3); err == nil {
		t.Fatalf("故障场景应返回 error")
	}
}

// TestCodeGranted 组合语义：集合命中即 true；集合不含即 false；故障透传。
func TestCodeGranted(t *testing.T) {
	f := &fakeCodeChecker{objects: []string{"permission:kafka-topics.manage"}}
	ok, err := CodeGranted(context.Background(), f, "kafka-topics:manage")
	if err != nil || !ok {
		t.Fatalf("持有场景: ok=%v err=%v", ok, err)
	}
	ok, err = CodeGranted(context.Background(), f, "redis:read")
	if err != nil || ok {
		t.Fatalf("不持有场景: ok=%v err=%v", ok, err)
	}
	f.err = errors.New("timeout")
	if _, err = CodeGranted(context.Background(), f, "kafka-topics:manage"); err == nil {
		t.Fatalf("故障场景应返回 error")
	}
}
