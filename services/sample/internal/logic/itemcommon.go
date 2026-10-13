// itemcommon.go 是 sample 服务 logic 层公共小函数：API 层（types.Item.Id 为
// 字符串路径参数/JSON 字段，goctl 契约固定）与存储层（repo.Item.ID 为 int64
// 自增主键，AGENTS.md 数据库规范）之间的 ID 互转。
package logic

import (
	"strconv"
	"strings"

	"nemo/services/sample/internal/repo"
)

// parseItemID 把 API 路径参数（十进制字符串）解析为存储层 int64 主键。
// 非法/非正数按未命中处理（返回 repo.ErrNotFound，调用方统一转 404）——
// 与原字符串主键"查不到即 404"的行为保持一致，避免引入新的 400 语义。
func parseItemID(id string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 {
		return 0, repo.ErrNotFound
	}
	return n, nil
}

// formatItemID 把存储层 int64 主键转回 API 字符串形态（goctl 契约 Id 为 string）。
func formatItemID(id int64) string {
	return strconv.FormatInt(id, 10)
}
