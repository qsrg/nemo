// guard_test.go 覆盖 kafka 路由守卫的对象 ID 提取器：pathvar 注入的 :id
// 合法/非法/缺失三态（pathvar 注入时机由 go-zero patrouter 保证——
// ServeHTTP 命中路由后、调用 handler 链前 WithVars，见 patrouter.go）。
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

// TestTopicIDFromPath pathvar 注入 :id 三态：合法正整数放行提取、非法格式
// 与缺失（空 map）均判 !ok（中间件 400 fail-closed，不进判定）。
func TestTopicIDFromPath(t *testing.T) {
	cases := []struct {
		name   string
		vars   map[string]string
		wantID string
		wantOK bool
	}{
		{"合法 ID", map[string]string{"id": "42"}, "42", true},
		{"非数字", map[string]string{"id": "abc"}, "", false},
		{"零与负数", map[string]string{"id": "0"}, "", false},
		{"缺失参数", map[string]string{}, "", false},
		{"空串", map[string]string{"id": ""}, "", false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodPut, "/v1/kafka/topics/42", nil)
		r = pathvar.WithVars(r, tc.vars)
		id, ok := topicIDFromPath(r)
		if id != tc.wantID || ok != tc.wantOK {
			t.Errorf("%s: topicIDFromPath = (%q,%v)，期望 (%q,%v)", tc.name, id, ok, tc.wantID, tc.wantOK)
		}
	}
}
