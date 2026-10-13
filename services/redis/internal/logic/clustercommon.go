// clustercommon.go 是集群 logic 的公共小函数：repo 记录 → API 响应转换。
// 集群为基础配置数据（照 kafka clustercommon 口径）：无引擎判定、无行级徽标。
package logic

import (
	"strconv"

	"nemo/services/redis/internal/repo"
	"nemo/services/redis/internal/types"
)

// clusterResp 把 repo 集群记录转为 API 响应（Id 字符串化，前端 JSON 数字
// 精度兼容）。
func clusterResp(c repo.Cluster) types.ClusterResp {
	return types.ClusterResp{
		Id:        strconv.FormatInt(c.ID, 10),
		Code:      c.Code,
		Remark:    c.Remark,
		CreatedBy: c.CreatedBy,
		UpdatedAt: c.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
