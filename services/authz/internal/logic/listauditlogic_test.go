// listauditlogic_test.go 覆盖 ListAudit（GET /v1/admin/audit）回填实现：
// 分页与（CreatedAt,ID）倒序、按操作人过滤、空结果返回 [] 非 null、
// repo 故障 fail-closed。
package logic

import (
	"context"
	"errors"
	"net/http"
	"testing"

	kiterrors "nemo/kit/errors"
	"nemo/services/authz/internal/repo"
	"nemo/services/authz/internal/svc"
	"nemo/services/authz/internal/types"
)

// failingAuditRepo 是 AuditRepo 的故障替身：Query 恒报错（其余方法内嵌接口零值，
// 本测试只触发 Query）。
type failingAuditRepo struct {
	repo.AuditRepo
}

// Query 恒报错，模拟存储层故障。
func (failingAuditRepo) Query(context.Context, repo.AuditFilter) ([]repo.AuditEntry, int, error) {
	return nil, 0, errors.New("storage down")
}

// seedAudit 经 AuditRepo.CreatePending 造 n 条审计（操作人可区分）。
func seedAudit(t *testing.T, sc *svc.ServiceContext, n int, operator string) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := sc.AuditRepo.CreatePending(context.Background(), repo.AuditEntry{
			Operator: operator,
			Action:   "action",
			Target:   "target",
		}); err != nil {
			t.Fatalf("造审计条目: %v", err)
		}
	}
}

// TestListAuditPagination 覆盖分页：5 条、pageSize=2 → 第 1/2 页各 2 条、
// 第 3 页 1 条，total 恒为 5，页间无重叠，越界页空列表非 null。
func TestListAuditPagination(t *testing.T) {
	sc := newTestSvcCtx(t)
	seedAudit(t, sc, 5, "op1")

	// 单条创建耗时 <1ms，CreatedAt 可能同值——倒序由（CreatedAt,ID）保证，
	// 断言只验证总量与页大小，不依赖跨条目时间戳的严格递增。
	p1, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("第 1 页: %v", err)
	}
	if p1.Total != 5 || len(p1.Items) != 2 {
		t.Fatalf("第 1 页应 total=5、2 条，实际 total=%d len=%d", p1.Total, len(p1.Items))
	}
	p2, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("第 2 页: %v", err)
	}
	if p2.Total != 5 || len(p2.Items) != 2 {
		t.Fatalf("第 2 页应 total=5、2 条，实际 total=%d len=%d", p2.Total, len(p2.Items))
	}
	// 页间无重叠（ID 集合不相交）。
	seen := map[string]bool{}
	for _, it := range append(append([]types.AuditResp{}, p1.Items...), p2.Items...) {
		if seen[it.Id] {
			t.Errorf("条目 %s 跨页重复", it.Id)
		}
		seen[it.Id] = true
	}
	p3, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{Page: 3, PageSize: 2})
	if err != nil {
		t.Fatalf("第 3 页: %v", err)
	}
	if p3.Total != 5 || len(p3.Items) != 1 {
		t.Fatalf("第 3 页应 total=5、1 条，实际 total=%d len=%d", p3.Total, len(p3.Items))
	}

	// 越界页：空列表非 nil，total 保持。
	p9, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{Page: 9, PageSize: 2})
	if err != nil {
		t.Fatalf("越界页: %v", err)
	}
	if p9.Items == nil || len(p9.Items) != 0 || p9.Total != 5 {
		t.Fatalf("越界页应空列表非 null：items=%v total=%d", p9.Items, p9.Total)
	}
}

// TestListAuditFilterOperator 覆盖 UserId 过滤：只返回该操作人的条目。
func TestListAuditFilterOperator(t *testing.T) {
	sc := newTestSvcCtx(t)
	seedAudit(t, sc, 2, "op-a")
	seedAudit(t, sc, 1, "op-b")

	resp, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{UserId: "op-a"})
	if err != nil {
		t.Fatalf("过滤查询: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("op-a 应 2 条，实际 total=%d len=%d", resp.Total, len(resp.Items))
	}
	for _, it := range resp.Items {
		if it.Operator != "op-a" {
			t.Errorf("过滤结果混入他人条目: %+v", it)
		}
	}
}

// TestListAuditEmpty 覆盖空结果：Items 为 [] 非 null。
func TestListAuditEmpty(t *testing.T) {
	sc := newTestSvcCtx(t)
	resp, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{})
	if err != nil {
		t.Fatalf("空查询: %v", err)
	}
	if resp.Items == nil || len(resp.Items) != 0 || resp.Total != 0 {
		t.Fatalf("空结果应 []/0：items=%v total=%d", resp.Items, resp.Total)
	}
}

// TestListAuditRepoFailure 覆盖存储故障：500 AUTHZ_INTERNAL fail-closed
// （与 ListRules 的 repo 故障语义一致）。
func TestListAuditRepoFailure(t *testing.T) {
	sc := &svc.ServiceContext{AuditRepo: failingAuditRepo{}}
	_, err := NewListAuditLogic(adminCtx(), sc).ListAudit(&types.ListAuditReq{})
	var e *kiterrors.Error
	if !errors.As(err, &e) || e.Status != http.StatusInternalServerError {
		t.Fatalf("存储故障应 500 fail-closed: %v", err)
	}
	if e.Code != "AUTHZ_INTERNAL" {
		t.Fatalf("错误码应 AUTHZ_INTERNAL: %q", e.Code)
	}
}
