// Package authz 提供业务服务的权限判定接入件（kit 侧）：Checker 接口、
// Remote 远端实现与 Require/RequireWith HTTP 中间件（形态为标准库
// http.HandlerFunc），权限码判定件（CodeGranted 等）与统一身份中间件。
//
// 职责边界（kit 纪律）：不含业务逻辑、不出现业务名词、不做运行时治理
// （熔断/重试由服务治理层或调用方负责）；错误统一经 kit/errors 约定体输出。
//
// 四语义（全局约束，逐字）：
//   - 无权 → 403 AUTHZ_DENIED；
//   - 网络/5xx/超时 → 503 AUTHZ_SERVICE_UNAVAILABLE（fail-closed）；
//   - 上游身份错误（401）原样透传（"未认证"与"无权"是两类事实）；
//   - RequireWith 提取器判对象 ID 非法 → 400 AUTHZ_OBJECT_INVALID。
package authz

import "context"

// Checker 是权限判定的最小接口：ctx 内含身份——实现方（如 Remote）从 ctx
// 取 JWT 透传给判定服务（见 remote.go 的 WithToken）。
type Checker interface {
	// Check 判定 object 上 relation 权限是否允许当前用户。
	// 第二返回值仅表示判定服务不可用/通信失败等故障；
	// "明确无权"返回 (false, nil)，不报错。
	Check(ctx context.Context, object, relation string) (bool, error)

	// ListObjects 枚举当前用户在 objType 类型对象上拥有 relation 权限的实例列表。
	ListObjects(ctx context.Context, objType, relation string) ([]string, error)

	// BatchCheckItem 是 BatchCheck 的单条判定请求。
	BatchCheck(ctx context.Context, items []BatchCheckItem) ([]bool, error)
}

// BatchCheckItem 是 BatchCheck 的单条判定项（Checker.BatchCheck 的入参元素）。
// user 由 ctx 统一承载（与 Check 一致），不单独指定。
type BatchCheckItem struct {
	// Object 对象标识（如 kafka_topic:1）。
	Object string `json:"object"`
	// Relation 关系/权限（如 can_view）。
	Relation string `json:"relation"`
}

// CheckObject 按类型与实例 ID 拼装对象标识后判定：
// objType "order"、id "o1"、relation "can_view" 等价于 Check(ctx, "order:o1", "can_view")。
// （注释中的 "order"/"can_view" 取自计划接口签名的逐字示例，仅作文档示例——
// 本包代码与测试不出现任何业务名词。）
func CheckObject(ctx context.Context, c Checker, objType, id, relation string) (bool, error) {
	return c.Check(ctx, objType+":"+id, relation)
}
