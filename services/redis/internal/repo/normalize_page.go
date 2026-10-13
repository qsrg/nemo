// normalize_page.go 提供分页参数的统一钳制（repo 内存/MySQL 实现共用）：
// 页码 <1 视为 1，页大小 <1 或超过上限 200 视为默认 20——防恶意大页拖库。
package repo

// defaultPageSize 是页大小缺省值。
const defaultPageSize = 20

// maxPageSize 是页大小上限（防一次拉全表）。
const maxPageSize = 200

// normalizePage 钳制分页参数并返回（page, size）。
func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > maxPageSize {
		size = defaultPageSize
	}
	return page, size
}
