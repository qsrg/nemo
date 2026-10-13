// Package engine 将 OpenFGA 引擎以进程内嵌方式启动（loopback HTTP），
// 为上层服务提供授权决策能力（Check/ListObjects）与引导能力（EnsureModel/WriteTuples）。
//
// 数据存储支持 memory（PoC/测试）与 mysql（持久化）两种后端；
// 引擎对 OpenFGA 家族模块的依赖仅限 openfga、go-sdk、language 三者
// 及其传递依赖 api（proto/gateway 注册）；MySQL 后端的 schema 迁移
// 复用 openfga 模块内的 pkg/storage/migrate（goose 驱动，Task 8 起）。
package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/openfga/go-sdk/client"
	"github.com/openfga/openfga/pkg/gateway"
	"github.com/openfga/openfga/pkg/logger"
	"github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage"
	"github.com/openfga/openfga/pkg/storage/memory"
	"github.com/openfga/openfga/pkg/storage/migrate"
	mysqlds "github.com/openfga/openfga/pkg/storage/mysql"
	"github.com/openfga/openfga/pkg/storage/sqlcommon"

	grpc_runtime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
)

// 数据存储后端类型常量，供 Options.Datastore 使用。
const (
	// DatastoreMemory 内存后端，用于 PoC 与测试。
	DatastoreMemory = "memory"
	// DatastoreMySQL MySQL 持久化后端，Task 8 填实。
	DatastoreMySQL = "mysql"
)

// portReadyTimeout 是等待 HTTP 端口就绪的最长时间。
const portReadyTimeout = 5 * time.Second

// pingTimeout 是 Ping 单次拨号探活的超时时间（就绪探针要求快速返回）。
const pingTimeout = time.Second

// Options 是引擎启动参数。
type Options struct {
	// HTTPAddr 是引擎 HTTP 监听地址（回环），例如 "127.0.0.1:0"（随机端口）。
	HTTPAddr string
	// StoreName 是默认授权 Store 的名称（EnsureModel 按名幂等创建）。
	StoreName string
	// Datastore 是数据存储后端：memory 或 mysql。
	Datastore string
	// MySQLDSN 是 MySQL 连接串（Datastore 为 mysql 时使用）。
	MySQLDSN string
}

// Handle 是引擎启动后对外暴露的操作句柄，封装 go-sdk client 与进程内 HTTP server。
type Handle struct {
	// apiURL 是引擎 HTTP 服务的实际地址（含随机端口解析结果）。
	apiURL string
	// client 是 OpenFGA go-sdk 客户端，指向进程内 HTTP server。
	client *client.OpenFgaClient
	// storeName 是 Options 传入的默认 Store 名称（ensureStore 幂等依据）。
	storeName string
	// storeID 是 EnsureModel 幂等创建/查找到的 Store ID。
	storeID string
	// modelID 是 EnsureModel 写入的授权模型 ID。
	modelID string
	// idsMu 保护 storeID/modelID：EnsureModel（RegisterResourceType 模型合并路径）
	// 写 vs Check/ListObjects/WriteTuples 等读并发（终审 I1），RWMutex 保护。
	idsMu sync.RWMutex
	// httpSrv 是进程内 HTTP server，Stop 时关闭。
	httpSrv *http.Server
	// dsClose 关闭 datastore 持有的资源（MySQL 连接池；memory 无资源为 nil），
	// Stop 时统一释放。
	dsClose func()
	// datastore 是启动时的后端类型（memory/mysql），ListObjectIDs 分支选择用。
	datastore string
	// mysqlDSN 是启动时的引擎库连接串（Datastore=mysql 时非空），同上。
	mysqlDSN string
}

// APIURL 返回引擎 HTTP 服务的实际地址。
func (h *Handle) APIURL() string {
	return h.apiURL
}

// Stop 关闭进程内 HTTP server 与 datastore 资源（MySQL 连接池），重复调用安全（幂等）。
func (h *Handle) Stop() {
	if h.httpSrv != nil {
		_ = h.httpSrv.Close()
		h.httpSrv = nil
	}
	if h.dsClose != nil {
		h.dsClose()
		h.dsClose = nil
	}
}

// StoreID 返回 EnsureModel 之后绑定的 Store ID（未绑定返回空串）。
func (h *Handle) StoreID() string {
	storeID, _ := h.boundIDs()
	return storeID
}

// ModelID 返回 EnsureModel 之后绑定的授权模型 ID（未绑定返回空串）。
func (h *Handle) ModelID() string {
	_, modelID := h.boundIDs()
	return modelID
}

// bindIDs 绑定 EnsureModel 的结果（写锁）。
func (h *Handle) bindIDs(storeID, modelID string) {
	h.idsMu.Lock()
	defer h.idsMu.Unlock()
	h.storeID, h.modelID = storeID, modelID
}

// boundIDs 取当前绑定的 Store/Model ID（读锁）。
func (h *Handle) boundIDs() (string, string) {
	h.idsMu.RLock()
	defer h.idsMu.RUnlock()
	return h.storeID, h.modelID
}

// Ping 探测引擎 HTTP 服务是否可用：对 APIURL 做一次短超时 TCP 拨号，
// 成功返回 nil，失败返回带上下文的错误（供服务层 /readyz 就绪探针使用）。
// Stop 之后端口已关闭，Ping 应返回错误。
func (h *Handle) Ping() error {
	// 从 apiURL 剥离 scheme，得到拨号探测用的 host:port（与 waitPortReady 同约定）。
	target := h.apiURL
	if len(target) > len("http://") && target[:len("http://")] == "http://" {
		target = target[len("http://"):]
	}
	conn, err := net.DialTimeout("tcp", target, pingTimeout)
	if err != nil {
		return fmt.Errorf("ping 引擎端口 %s: %w", target, err)
	}
	_ = conn.Close()
	return nil
}

// Start 进程内启动 OpenFGA 引擎：装配 server（按 Datastore 选择后端）→
// grpc-gateway 直接把服务实现映射为 HTTP JSON API（无独立 gRPC 端口）→
// goroutine 监听 → 等待端口就绪 → 构造 go-sdk client。
// ctx 用于 gateway 装配与就绪探测，不控制引擎生命周期（引擎由 Stop 控制）。
func Start(ctx context.Context, opt Options) (*Handle, error) {
	// 未指定后端时默认 memory，便于 PoC 与测试最小装配。
	if opt.Datastore == "" {
		opt.Datastore = DatastoreMemory
	}

	// 装配数据存储后端：memory（PoC/测试）或 mysql（持久化，Task 8 填实）。
	var ds storage.OpenFGADatastore
	started := false
	defer func() {
		// MySQL 连接池正常由 Stop 释放；构造中途失败的路径在此兜底释放
		//（started 置位后 defer 不重复关闭）。
		if !started && ds != nil {
			ds.Close()
		}
	}()
	switch opt.Datastore {
	case DatastoreMemory:
		ds = memory.New()
	case DatastoreMySQL:
		var err error
		if ds, err = newMySQLDatastore(opt.MySQLDSN); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("未知 datastore %q：支持 %s/%s", opt.Datastore, DatastoreMemory, DatastoreMySQL)
	}

	// 装配 OpenFGA server；RPC transport 用于把 HTTP 状态码
	// 等 header 从服务实现传递到 gateway 响应（与 openfga 官方 cmd/run 装配一致）。
	srv, err := server.NewServerWithOpts(
		server.WithDatastore(ds),
		server.WithTransport(gateway.NewRPCTransport(logger.NewNoopLogger())),
	)
	if err != nil {
		return nil, fmt.Errorf("构造 OpenFGA server(%s): %w", opt.Datastore, err)
	}

	// grpc-gateway 把 OpenFGA gRPC 服务实现直接挂到 HTTP mux（无独立 gRPC 端口）。
	gwmux := grpc_runtime.NewServeMux()
	if err := openfgav1.RegisterOpenFGAServiceHandlerServer(ctx, gwmux, srv); err != nil {
		return nil, fmt.Errorf("注册 OpenFGA HTTP gateway: %w", err)
	}

	// 自行 net.Listen 以便从监听器取回实际地址（支持 HTTPAddr 传 :0 随机端口）。
	ln, err := net.Listen("tcp", opt.HTTPAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 %s: %w", opt.HTTPAddr, err)
	}
	httpSrv := &http.Server{Handler: gwmux}
	go func() {
		// Serve 在 Close/Shutdown 后返回 http.ErrServerClosed，属正常停止路径，忽略。
		_ = httpSrv.Serve(ln)
	}()

	h := &Handle{
		apiURL:    fmt.Sprintf("http://%s", ln.Addr().String()),
		httpSrv:   httpSrv,
		storeName: opt.StoreName,
		datastore: opt.Datastore,
		mysqlDSN:  opt.MySQLDSN,
	}
	// MySQL datastore 的连接池交给 Stop 统一释放（memory 无资源，为 nil）。
	if opt.Datastore == DatastoreMySQL {
		h.dsClose = ds.Close
	}

	// 等待 HTTP 端口就绪（dial 重试 ≤5s），避免 client 过早请求导致连接拒绝。
	if err := waitPortReady(ctx, h.apiURL, portReadyTimeout); err != nil {
		h.Stop()
		return nil, err
	}

	// 构造 go-sdk client，指向进程内 HTTP server。
	cl, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl: h.apiURL,
	})
	if err != nil {
		h.Stop()
		return nil, fmt.Errorf("构造 go-sdk client: %w", err)
	}
	h.client = cl
	// 引擎装配完成：构造期兜底释放 defer 不再生效（MySQL 连接池改由 Stop 释放）。
	started = true
	return h, nil
}

// newMySQLDatastore 构造 OpenFGA MySQL 数据存储（Datastore=mysql 路径，Task 8）：
//  1. 规范化 DSN：强制 ParseTime=true（OpenFGA 时间列读取依赖；缺省时
//     datastore 查询会因时间反序列化为零值而语义错乱），其余参数原样保留；
//  2. 迁移先行：执行 OpenFGA 自带 schema 迁移（pkg/storage/migrate.RunMigrations，
//     goose 管理 store/authorization_model/tuple/assertion 等表），幂等——
//     goose_db_version 记录已应用版本，重复启动只跳过；必须在引擎对外服务
//     （EnsureModel/Check 等任何 datastore 访问）之前完成；
//  3. mysqlds.New 建连接池（自带 ping 重试；sqlcommon.NewConfig 提供官方默认值）。
func newMySQLDatastore(dsn string) (storage.OpenFGADatastore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("datastore=mysql 需要配置 MySQLDSN")
	}

	cfgDSN, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析 MySQL DSN: %w", err)
	}
	cfgDSN.ParseTime = true
	dsn = cfgDSN.FormatDSN()

	if err := migrate.RunMigrations(migrate.MigrationConfig{
		Engine: "mysql",
		URI:    dsn,
		// 超时必须显式配置：MigrationConfig 零值会让迁移内部的
		// context.WithTimeout(0) 立即过期 + backoff 无限重试（go doc 核对结论，
		// openfga cmd/migrate 由 server config 兜底，程序化调用需自带）。
		Timeout:     30 * time.Second,
		PingTimeout: 5 * time.Second,
		Logger:      logger.NewNoopLogger(),
	}); err != nil {
		return nil, fmt.Errorf("OpenFGA schema 迁移: %w", err)
	}

	ds, err := mysqlds.New(dsn, sqlcommon.NewConfig())
	if err != nil {
		return nil, fmt.Errorf("构造 MySQL datastore: %w", err)
	}
	return ds, nil
}

// Check 判定 user 是否在 object 上拥有 relation 权限（固定使用 EnsureModel 的模型）。
func (h *Handle) Check(ctx context.Context, user, relation, object string) (bool, error) {
	return h.CheckWithContext(ctx, user, relation, object, nil)
}

// CheckWithContext 判定权限并附带 check 侧上下文（ABAC，Phase C 改造③④）：
// evalContext 提供用户侧条件参数（如 level_ok 的 user_level），与元组侧
// condition context（资源属性）在引擎内合并求值；nil/空 = 纯关系判定，
// 与 Check 等价。
func (h *Handle) CheckWithContext(ctx context.Context, user, relation, object string, evalContext map[string]any) (bool, error) {
	if h.client == nil {
		return false, errors.New("engine client 未初始化")
	}
	storeID, modelID := h.boundIDs()
	body := client.ClientCheckRequest{
		User:     user,
		Relation: relation,
		Object:   object,
	}
	if len(evalContext) > 0 {
		body.Context = &evalContext
	}
	resp, err := h.client.Check(ctx).
		Options(client.ClientCheckOptions{
			StoreId:              &storeID,
			AuthorizationModelId: &modelID,
		}).
		Body(body).
		Execute()
	if err != nil {
		return false, fmt.Errorf("Check(%s,%s,%s): %w", user, relation, object, err)
	}
	return resp.GetAllowed(), nil
}

// ListObjects 枚举 user 在指定类型 objects 上拥有 relation 权限的实例列表
// （固定使用 EnsureModel 的模型）。
func (h *Handle) ListObjects(ctx context.Context, user, relation, objType string) ([]string, error) {
	if h.client == nil {
		return nil, errors.New("engine client 未初始化")
	}
	storeID, modelID := h.boundIDs()
	resp, err := h.client.ListObjects(ctx).
		Options(client.ClientListObjectsOptions{
			StoreId:              &storeID,
			AuthorizationModelId: &modelID,
		}).
		Body(client.ClientListObjectsRequest{
			User:     user,
			Relation: relation,
			Type:     objType,
		}).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("ListObjects(%s,%s,%s): %w", user, relation, objType, err)
	}
	return resp.GetObjects(), nil
}

// BatchCheckItem 是 BatchCheck 的单条判定请求（与 OpenFGA BatchCheckItem 同构，
// 但 user 统一由 BatchCheck 顶层参数传入——本系统中同一批次判定始终针对同一用户）。
type BatchCheckItem struct {
	// Object 对象标识（如 kafka_topic:1）。
	Object string
	// Relation 关系/权限（如 can_view）。
	Relation string
}

// BatchCheck 一次判定多个 object/relation 组合，返回与 items 下标一一对应的
// bool 切片。任一 item 明确无权 → 对应位置 false，整体 error 为 nil；
// 引擎/网络故障 → 整体 error 返回（fail-closed）。
// 内部用 correlation_id（下标字符串）做映射，避免 item 级错误丢失。
func (h *Handle) BatchCheck(ctx context.Context, user string, items []BatchCheckItem) ([]bool, error) {
	if h.client == nil {
		return nil, errors.New("engine client 未初始化")
	}
	if len(items) == 0 {
		return []bool{}, nil
	}
	storeID, modelID := h.boundIDs()

	// 构造 OpenFGA BatchCheck 请求：用下标字符串做 correlation_id 做结果映射。
	checks := make([]client.ClientBatchCheckItem, 0, len(items))
	for i, it := range items {
		checks = append(checks, client.ClientBatchCheckItem{
			User:          user,
			Relation:      it.Relation,
			Object:        it.Object,
			CorrelationId: strconv.Itoa(i),
		})
	}

	resp, err := h.client.BatchCheck(ctx).
		Options(client.BatchCheckOptions{
			StoreId:              &storeID,
			AuthorizationModelId: &modelID,
		}).
		Body(client.ClientBatchCheckRequest{Checks: checks}).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("BatchCheck(%d items): %w", len(items), err)
	}

	// 按 correlation_id 映射回原下标。
	results := resp.GetResult()
	out := make([]bool, len(items))
	for i := range items {
		cid := strconv.Itoa(i)
		single, ok := results[cid]
		if !ok {
			// OpenFGA 未返回该 correlation_id → fail-closed。
			out[i] = false
			continue
		}
		if single.HasError() {
			// item 级错误（如模型无此 relation）→ 按无权处理（fail-closed）。
			out[i] = false
			continue
		}
		out[i] = single.GetAllowed()
	}
	return out, nil
}

// waitPortReady 以 TCP dial 轮询等待引擎 HTTP 端口就绪，超过 timeout 返回最后一次错误。
func waitPortReady(ctx context.Context, apiURL string, timeout time.Duration) error {
	// 从 apiURL 剥离 scheme，得到拨号探测用的 host:port。
	target := apiURL
	if len(target) > len("http://") && target[:len("http://")] == "http://" {
		target = target[len("http://"):]
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待端口就绪被取消: %w", ctx.Err())
		default:
		}
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", target)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("等待引擎端口 %s 就绪超时(%s): %w", target, timeout, lastErr)
}
