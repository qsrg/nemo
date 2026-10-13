// bootstrap.go 实现引擎的引导操作：授权模型装配（EnsureModel）与关系元组写入（WriteTuples）。
// 两者均按"读后写"实现幂等，供 Task 2 起的服务层与测试复用。
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	fgaSdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/openfga/language/pkg/go/transformer"
)

// defaultStoreName 是未配置 StoreName 时的兜底 Store 名称。
const defaultStoreName = "nemo-authz"

// maxWriteOps 是 OpenFGA 单次 Write 请求允许的最大操作数（服务端硬限制 100）。
const maxWriteOps = 100

// EnsureModel 幂等装配授权模型：
//  1. DSL 经 language transformer 解析校验并转为 JSON；
//  2. 按 StoreName 查找 Store，不存在则创建（幂等按名）；
//  3. 模型 JSON 经 WriteAuthorizationModel 写入（本任务总是新建，
//     与业务模型版本的比对逻辑由 Task 6 接管）。
//
// 返回 storeID 与 modelID，并绑定到 Handle 供 Check/ListObjects/WriteTuples 使用。
func (h *Handle) EnsureModel(ctx context.Context, modelDSL string) (string, string, error) {
	if h.client == nil {
		return "", "", errors.New("engine client 未初始化")
	}
	if strings.TrimSpace(modelDSL) == "" {
		return "", "", errors.New("modelDSL 为空")
	}

	// DSL → JSON：同时完成语法与语义校验；失败说明 DSL 不合法。
	modelJSON, err := transformer.TransformDSLToJSON(modelDSL)
	if err != nil {
		return "", "", fmt.Errorf("解析模型 DSL: %w", err)
	}

	// JSON 直接反序列化为 SDK 的写模型请求体（type_definitions/conditions 对齐）。
	var writeModelReq client.ClientWriteAuthorizationModelRequest
	if err := json.Unmarshal([]byte(modelJSON), &writeModelReq); err != nil {
		return "", "", fmt.Errorf("模型 JSON 反序列化: %w", err)
	}

	storeID, err := h.ensureStore(ctx)
	if err != nil {
		return "", "", err
	}

	modelResp, err := h.client.WriteAuthorizationModel(ctx).
		Options(client.ClientWriteAuthorizationModelOptions{StoreId: &storeID}).
		Body(writeModelReq).
		Execute()
	if err != nil {
		return "", "", fmt.Errorf("写入授权模型: %w", err)
	}
	modelID := modelResp.GetAuthorizationModelId()
	h.bindIDs(storeID, modelID)
	return storeID, modelID, nil
}

// ensureStore 按 StoreName 幂等查找/创建 Store：
// 先带 Name 过滤的 ListStores 分页查找同名 Store，找不到再 CreateStore。
func (h *Handle) ensureStore(ctx context.Context) (string, error) {
	name := h.storeName
	if strings.TrimSpace(name) == "" {
		name = defaultStoreName
	}

	// 分页遍历既有 Store，命中同名即返回（幂等）。
	pageSize := int32(100)
	continuationToken := ""
	for {
		resp, err := h.client.ListStores(ctx).
			Options(client.ClientListStoresOptions{
				PageSize:          &pageSize,
				ContinuationToken: &continuationToken,
			}).
			Execute()
		if err != nil {
			return "", fmt.Errorf("按名查找 Store(%s): %w", name, err)
		}
		for _, st := range resp.GetStores() {
			if st.GetName() == name {
				return st.GetId(), nil
			}
		}
		token := resp.GetContinuationToken()
		if token == "" {
			break
		}
		continuationToken = token
	}

	// 不存在则创建。
	createResp, err := h.client.CreateStore(ctx).
		Body(client.ClientCreateStoreRequest{Name: name}).
		Execute()
	if err != nil {
		return "", fmt.Errorf("创建 Store(%s): %w", name, err)
	}
	return createResp.GetId(), nil
}

// WriteTuples 幂等写入关系元组：写前逐条 Read 校验，已存在即跳过；
// 只提交不存在的部分，全部已存在时不发请求。
func (h *Handle) WriteTuples(ctx context.Context, tuples []Tuple) error {
	if h.client == nil {
		return errors.New("engine client 未初始化")
	}
	storeID, modelID := h.boundIDs()
	if storeID == "" || modelID == "" {
		return errors.New("Store/Model 未绑定，请先调用 EnsureModel")
	}

	// 读后写过滤：已存在的元组跳过，实现幂等。
	var pending []client.ClientTupleKey
	for _, t := range tuples {
		if err := t.validate(); err != nil {
			return err
		}
		exists, err := h.tupleExists(ctx, t)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		key := client.ClientTupleKey{
			User:     t.User,
			Relation: t.Relation,
			Object:   t.Object,
		}
		// 元组侧条件（ABAC）：条件名 + 持久化参数（如 required_level）。
		if t.Condition != nil {
			if strings.TrimSpace(t.Condition.Name) == "" {
				return errors.New("条件元组的 condition 名不得为空")
			}
			condCtx := t.Condition.Context
			key.Condition = &fgaSdk.RelationshipCondition{Name: t.Condition.Name, Context: &condCtx}
		}
		pending = append(pending, key)
	}
	if len(pending) == 0 {
		return nil
	}

	// OpenFGA 单次 Write 上限 100 条操作，超限分批提交。
	for start := 0; start < len(pending); start += maxWriteOps {
		end := start + maxWriteOps
		if end > len(pending) {
			end = len(pending)
		}
		_, err := h.client.Write(ctx).
			Options(client.ClientWriteOptions{
				StoreId:              &storeID,
				AuthorizationModelId: &modelID,
			}).
			Body(client.ClientWriteRequest{Writes: pending[start:end]}).
			Execute()
		if err != nil {
			return fmt.Errorf("写入 %d 条元组: %w", len(pending), err)
		}
	}
	return nil
}

// tupleExists 读前校验单条元组是否已存在。
func (h *Handle) tupleExists(ctx context.Context, t Tuple) (bool, error) {
	storeID, _ := h.boundIDs()
	resp, err := h.client.Read(ctx).
		Options(client.ClientReadOptions{
			StoreId: &storeID,
		}).
		Body(client.ClientReadRequest{
			User:     &t.User,
			Relation: &t.Relation,
			Object:   &t.Object,
		}).
		Execute()
	if err != nil {
		return false, fmt.Errorf("读元组(%s,%s,%s): %w", t.User, t.Relation, t.Object, err)
	}
	return len(resp.GetTuples()) > 0, nil
}

// TupleCondition 是元组侧条件（ABAC，Phase C 改造③④）：Name 为模型中已定义
// 的 condition 名，Context 为持久化在元组上的条件参数（如 level_ok 的
// required_level）。nil 条件的 Tuple 即普通元组，WriteTuples 行为不变。
type TupleCondition struct {
	// Name 条件名（须在授权模型中已定义，如 level_ok）。
	Name string
	// Context 条件参数（键须与 condition 参数一致，值类型须匹配）。
	Context map[string]any
}

// Tuple 是一条关系元组（user-relation-object 三元组 + 可选条件）。
type Tuple struct {
	// User 是主体标识，形如 "user:u1" 或 "org:root#member"。
	User string
	// Relation 是关系名，需在模型中已定义。
	Relation string
	// Object 是对象标识，形如 "order:o1"。
	Object string
	// Condition 元组侧条件（nil = 普通元组）。
	Condition *TupleCondition
}

// validate 校验元组三要素非空。
func (t Tuple) validate() error {
	if t.User == "" || t.Relation == "" || t.Object == "" {
		return fmt.Errorf("非法元组：user/relation/object 不得为空（got %q,%q,%q）", t.User, t.Relation, t.Object)
	}
	return nil
}
