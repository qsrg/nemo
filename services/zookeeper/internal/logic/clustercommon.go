// clustercommon.go 是集群 logic 的公共小函数：repo 记录 → API 响应转换。
// 集群已进权限引擎（zk_cluster）：行级徽标（owner/canManageGrants）由
// listclusterslogic 批量取回后填充，本函数只做字段映射。
package logic

import (
	"strconv"

	"nemo/services/zookeeper/internal/repo"
	"nemo/services/zookeeper/internal/types"
)

// clusterResp 把 repo 集群记录转为 API 响应（Id 字符串化，前端 JSON 数字
// 精度兼容）。
func clusterResp(c repo.Cluster) types.ClusterResp {
	return types.ClusterResp{
		Id:        strconv.FormatInt(c.ID, 10),
		Code:      c.Code,
		Hosts:     c.Hosts,
		Remark:    c.Remark,
		CreatedBy: c.CreatedBy,
		UpdatedAt: c.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
