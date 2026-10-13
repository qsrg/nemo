// tuples.go 提供 Handle 的元组读取与删除能力（写路径 WriteTuples 见 bootstrap.go）：
// ReadTuples 按 object/relation 过滤（空 = 不过滤）分页读全量；
// ReadTuplesByUser 按 user 过滤（主体反查）；ReadTuplesByType 按 object 类型
// 过滤（某类型对象全量扫描，组织同步对账用）；DeleteTuples 幂等删除——
// 读后删，不存在的元组跳过，语义与 WriteTuples 对称。
package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/openfga/go-sdk/client"
)

// readPageSize 是 ReadTuples 单页大小上限（OpenFGA 服务端上限 100，翻页取全）。
const readPageSize = int32(100)

// ReadTuples 按 object/relation 过滤读取元组（两参均可为空 = 全量读取），
// 内部按 continuation token 翻页直至取完，返回 Tuple 切片（无匹配返回空切片）。
// Store/Model 未绑定（未 EnsureModel）时报错，防止对错误模型静默读空。
func (h *Handle) ReadTuples(ctx context.Context, object, relation string) ([]Tuple, error) {
	req := client.ClientReadRequest{}
	if object != "" {
		req.Object = &object
	}
	if relation != "" {
		req.Relation = &relation
	}
	return h.readTuples(ctx, req, fmt.Sprintf("object=%q relation=%q", object, relation))
}

// ReadTuplesByType 已移除：OpenFGA v1.19 的 Read 要求 object 为完整 type:id
//（type-only 与 user-only 过滤均被拒绝，实测报 object type required），不存在
// 按类型扫描元组的官方路径。类型扫描走 ListObjectIDs。

// ListObjectIDs 返回指定 object type 的全部对象 id（去重升序），供组织同步
// 对账等全量扫描场景使用——OpenFGA Read 不支持按类型扫描（见上），分后端实现：
//   - mysql：直查引擎库 tuple 表（SELECT DISTINCT object_id，走存储主键索引）；
//   - 其余（memory 演示/测试后端）：Read 全量分页在应用层过滤，规模小可接受。
func (h *Handle) ListObjectIDs(ctx context.Context, objectType string) ([]string, error) {
	if objectType == "" {
		return nil, errors.New("ListObjectIDs: objectType 不能为空")
	}
	storeID, _ := h.boundIDs()
	if storeID == "" {
		return nil, errors.New("Store/Model 未绑定，请先调用 EnsureModel")
	}
	if h.datastore == DatastoreMySQL && h.mysqlDSN != "" {
		return h.listObjectIDsMySQL(ctx, storeID, objectType)
	}
	tuples, err := h.readTuples(ctx, client.ClientReadRequest{}, "全量(内存后端)")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, t := range tuples {
		if rest, ok := strings.CutPrefix(t.Object, objectType+":"); ok {
			seen[rest] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// listObjectIDsMySQL 是 ListObjectIDs 的 MySQL 分支：直查引擎库 tuple 表。
// 每次调用开独立短连接（对账为分钟级低频操作，连接管理成本大于复用收益）。
func (h *Handle) listObjectIDsMySQL(ctx context.Context, storeID, objectType string) ([]string, error) {
	db, err := sql.Open("mysql", h.mysqlDSN)
	if err != nil {
		return nil, fmt.Errorf("打开引擎库连接: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT object_id FROM tuple WHERE store = ? AND object_type = ? ORDER BY object_id`,
		storeID, objectType)
	if err != nil {
		return nil, fmt.Errorf("查引擎对象清单(type=%s): %w", objectType, err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan object_id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate object_id: %w", err)
	}
	return ids, nil
}

// ReadTuplesByUser 按 user/relation 过滤读取元组（OpenFGA Read 的 user 过滤
// 形态：user 与 object 至少提供其一，按主体反查用）。relation 可为空。
func (h *Handle) ReadTuplesByUser(ctx context.Context, user, relation string) ([]Tuple, error) {
	req := client.ClientReadRequest{User: &user}
	if relation != "" {
		req.Relation = &relation
	}
	return h.readTuples(ctx, req, fmt.Sprintf("user=%q relation=%q", user, relation))
}

// readTuples 是元组读取的公共实现：按 continuation token 翻页直至取完，
// 回填条件元组（ABAC，条件名/参数透传给 grants 列表等消费方）。
// Store/Model 未绑定（未 EnsureModel）时报错，防止对错误模型静默读空。
func (h *Handle) readTuples(ctx context.Context, req client.ClientReadRequest, desc string) ([]Tuple, error) {
	if h.client == nil {
		return nil, errors.New("engine client 未初始化")
	}
	storeID, modelID := h.boundIDs()
	if storeID == "" || modelID == "" {
		return nil, errors.New("Store/Model 未绑定，请先调用 EnsureModel")
	}

	out := make([]Tuple, 0)
	continuationToken := ""
	pageSize := readPageSize
	for {
		resp, err := h.client.Read(ctx).
			Options(client.ClientReadOptions{
				StoreId:           &storeID,
				PageSize:          &pageSize,
				ContinuationToken: &continuationToken,
			}).
			Body(req).
			Execute()
		if err != nil {
			return nil, fmt.Errorf("读元组(%s): %w", desc, err)
		}
		for _, t := range resp.GetTuples() {
			key := t.GetKey()
			tuple := Tuple{
				User:     key.GetUser(),
				Relation: key.GetRelation(),
				Object:   key.GetObject(),
			}
			// 条件元组（ABAC）：回填条件名与持久化参数（grants 列表消费）。
			// SDK 的 GetCondition 返回值类型（零值 = 无条件），经 ConditionOk 判存。
			if cond, ok := key.GetConditionOk(); ok && cond.GetName() != "" {
				tuple.Condition = &TupleCondition{Name: cond.GetName(), Context: cond.GetContext()}
			}
			out = append(out, tuple)
		}
		token := resp.GetContinuationToken()
		if token == "" {
			return out, nil
		}
		continuationToken = token
	}
}

// DeleteTuples 幂等删除元组：读后删——逐条确认存在才提交删除，不存在的跳过
// （全部不存在则不发请求），重复调用安全。与 WriteTuples 的读后写对称。
func (h *Handle) DeleteTuples(ctx context.Context, tuples []Tuple) error {
	if h.client == nil {
		return errors.New("engine client 未初始化")
	}
	storeID, modelID := h.boundIDs()
	if storeID == "" || modelID == "" {
		return errors.New("Store/Model 未绑定，请先调用 EnsureModel")
	}

	// 读后删过滤：只删确实存在的元组，实现幂等。
	var pending []client.ClientTupleKeyWithoutCondition
	for _, t := range tuples {
		if err := t.validate(); err != nil {
			return err
		}
		exists, err := h.tupleExists(ctx, t)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		pending = append(pending, client.ClientTupleKeyWithoutCondition{
			User:     t.User,
			Relation: t.Relation,
			Object:   t.Object,
		})
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
			Body(client.ClientWriteRequest{Deletes: pending[start:end]}).
			Execute()
		if err != nil {
			return fmt.Errorf("删除 %d 条元组: %w", end-start, err)
		}
	}
	return nil
}
