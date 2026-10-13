// Code scaffolded by goctl. Safe to edit.
//
// 约定：logic 是业务规则层；错误一律用 kit/errors 语义构造器返回。
// 动作码演示（按钮级功能权限）：导出清单 = kafka-topics:export 码 +
// 资源级可见性（只导出调用者可见的行——功能码管"能不能导出"，资源级
// 管"能导出哪些"，两层 AND）。

package logic

import (
	"context"
	"fmt"
	"strings"

	"nemo/services/kafka/internal/svc"
	"nemo/services/kafka/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ExportKafkaTopicsLogic 承载「导出 Topic 清单」的业务规则。
type ExportKafkaTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewExportKafkaTopicsLogic 构造业务规则层实例。
func NewExportKafkaTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExportKafkaTopicsLogic {
	return &ExportKafkaTopicsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ExportKafkaTopics 导出当前用户可见的 Topic 清单（CSV 文本）。
// 功能码 kafka-topics:export 或管理码 kafka-topics:manage 在路由守卫
// （requireCodeOrManage）判定，本层只做业务：复用列表 logic 的可见性口径
// （列谓词分页 + 引擎复核），导出内容与列表所见完全一致——不因导出而
// 扩大可见范围。演示阶段导出前 100 条（与列表单页上限一致）。
func (l *ExportKafkaTopicsLogic) ExportKafkaTopics(_ *types.ExportTopicsReq) (string, error) {
	resp, err := NewListKafkaTopicsLogic(l.ctx, l.svcCtx).ListKafkaTopics(&types.ListTopicsReq{Page: 1, PageSize: 100})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("id,name,env,org_id,created_by\n")
	for _, t := range resp.Items {
		// CSV 字段含逗号风险低（name/env/org 均为受控字符集），描述列不导出。
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s\n", t.Id, t.Name, t.Env, t.OrgId, t.CreatedBy)
	}
	return b.String(), nil
}
