// Code scaffolded by goctl. Safe to edit.
// 约定：logic 是业务规则层、单测重点；错误用 kit/errors 语义断言。

package logic

import (
	"testing"

	"nemo/services/authz/internal/types"

	kiterrors "nemo/kit/errors"
)

// TestUpdateResourceType 管理码配置接口：未注册 404、非法码 400、
// 合法更新生效（List 可见）并幂等（重复更新成功）。
func TestUpdateResourceType(t *testing.T) {
	sc := newTestSvcCtx(t)
	registerKafkaTopic(t, sc) // 注册 kafka_topic（manageCode=kafka-topics:manage）

	ul := NewUpdateResourceTypeLogic(adminCtx(), sc)

	// 1. 未注册类型 → 404。
	if _, err := ul.UpdateResourceType(&types.UpdateResourceTypeReq{Name: "no_such_type", ManageCode: "x:manage"}); err == nil {
		t.Fatal("未注册类型应 404")
	} else if e := kiterrors.From(err); e == nil || e.Status != 404 {
		t.Fatalf("应 404: %v", err)
	}

	// 2. 非法管理码形态 → 400。
	if _, err := ul.UpdateResourceType(&types.UpdateResourceTypeReq{Name: "kafka_topic", ManageCode: "非法码"}); err == nil {
		t.Fatal("非法管理码应 400")
	} else if e := kiterrors.From(err); e == nil || e.Status != 400 {
		t.Fatalf("应 400: %v", err)
	}

	// 3. 更新为新码 → 成功且 List 可见。
	resp, err := ul.UpdateResourceType(&types.UpdateResourceTypeReq{Name: "kafka_topic", ManageCode: "kafka-topics:ops"})
	if err != nil {
		t.Fatalf("更新管理码应成功: %v", err)
	}
	if resp.ManageCode != "kafka-topics:ops" {
		t.Fatalf("返回管理码不符: %s", resp.ManageCode)
	}
	listResp, err := NewListResourceTypesLogic(adminCtx(), sc).ListResourceTypes()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, it := range listResp.Items {
		if it.Name == "kafka_topic" && it.ManageCode != "kafka-topics:ops" {
			t.Fatalf("List 管理码未更新: %+v", it)
		}
	}

	// 4. 幂等：重复更新（同值）成功。
	if _, err := ul.UpdateResourceType(&types.UpdateResourceTypeReq{Name: "kafka_topic", ManageCode: "kafka-topics:ops"}); err != nil {
		t.Fatalf("重复更新应幂等成功: %v", err)
	}

	// 5. 清空管理码 → 成功。
	if resp, err := ul.UpdateResourceType(&types.UpdateResourceTypeReq{Name: "kafka_topic", ManageCode: ""}); err != nil {
		t.Fatalf("清空管理码应成功: %v", err)
	} else if resp.ManageCode != "" {
		t.Fatalf("清空后管理码应为空: %s", resp.ManageCode)
	}
}
