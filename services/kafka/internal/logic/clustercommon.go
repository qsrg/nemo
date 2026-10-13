// clustercommon.go 集群模块公共小函数：错误归一（ErrNotFound 判定）、
// 记录 → 响应转换、int64 → string。
package logic

import (
	"errors"
	"strconv"
	"time"

	"nemo/services/kafka/internal/repo"
	"nemo/services/kafka/internal/types"
)

// isNotFound 判定存储层未命中哨兵（errors.Is 判等）。
func isNotFound(err error) bool { return errors.Is(err, repo.ErrNotFound) }

// itoa 是 int64 → string 的短名封装（路径/审计 target 拼串用）。
func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// clusterResp 把存储层 Cluster 记录转为 API 响应（时间格式化与契约对齐）。
func clusterResp(c repo.Cluster) *types.ClusterResp {
	return &types.ClusterResp{
		Id:        c.ID,
		Code:      c.Code,
		Dc:        c.Dc,
		Address:   c.Address,
		Remark:    c.Remark,
		CreatedBy: c.CreatedBy,
		UpdatedAt: c.UpdatedAt.Format(time.DateTime),
	}
}
