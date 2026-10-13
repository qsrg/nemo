package repo

import "errors"

// ErrNotFound 表示目标记录不存在（或列表为空）；各实现对未命中统一返回该
// 哨兵错误，调用方必须用 errors.Is 判定而非字符串比较。
var ErrNotFound = errors.New("record not found")

// ErrInvalidOwner 表示属性归属标识不合法（非 user:/org: 主体，也非 类型:id
// 资源形式）；由 NormalizeOwner 返回、logic 层转 400。
var ErrInvalidOwner = errors.New("invalid attribute owner")
