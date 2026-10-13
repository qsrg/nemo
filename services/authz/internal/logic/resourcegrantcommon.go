// resourcegrantcommon.go 承载通用资源授权（/v1/authz/relations 读写名单）
// 路径的统一错误构造。
//
// 2026-09-23 拆分说明：kafka 演示域独立为 services/kafka 后，原 kafkacommon.go
// 的 kafka 专用部分（topicGrantSubject/topicGrantTuple/topicCanEdit 等）随服务
// 迁移；本文件曾保留 authz 通用名单在 kafka_topic 类型上的分支实现
// （topicCanManage/parseTopicID/topicObject/kafkaTopicsManageCode）。
// 2026-10-11 收敛：kafka_topic 专用分支并入注册配置驱动的通用名单策略
// （grantpolicy.go，管理码来自注册表 ManageCode 声明），上述专用函数删除，
// 本文件仅保留通用错误构造（topicDenied/topicEngineErr）。
package logic

import (
	"nemo/kit/errors"
)

// topicDenied 是名单判定不满足的统一 403（读/写/撤销接口共用）。
func topicDenied() error {
	return errors.Forbidden("AUTHZ_DENIED", "无操作权限")
}

// topicEngineErr 是判定/元组引擎故障的统一 503（fail-closed，同 CheckLogic
// 口径：不区分未注册类型与引擎故障）。
func topicEngineErr() error {
	return errors.New(503, "AUTHZ_ENGINE_ERROR", "权限判定服务异常")
}
