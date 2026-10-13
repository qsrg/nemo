# 后端基础框架（Phase 0）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建 20~30 个 Go 服务共用的 go-zero 基座（kit 三包 + repo/client 样板 + 标准样板服务 sample），并把 backend-design.md 校准为可执行的接入指南。

**Architecture:** go-zero api 服务形态（无 zRPC/proto）；kit 收缩为 errors/logging/template 三包，v2.1 旧包冻结；repo/client 以 `services/sample` 内的样板代码 + 指南文档形态交付；sample 是新服务接入的对照标准。

**Tech Stack:** Go 1.26 + go-zero（api + goctl）+ database/sql（无 ORM）；测试用标准库 testing + httptest。

**Spec:** `docs/superpowers/specs/2026-09-06-backend-foundation-design.md`（v1.1）

## Global Constraints

- 执行前已 `git init` 并提交基线快照：每任务以 commit 为边界，任务审查基于 git diff；"打标" = 计划勾选 + SDD ledger + TodoWrite 三处同步
- go-zero api 形态，不引入 zRPC / proto（backend-design §1）
- kit 三纪律：不含业务逻辑、不出现业务名词（user/order/sample 等禁止出现在 kit 中）、非强制依赖（backend-design §1）
- 错误体全栈统一：`{"error":{"code","message","request_id"}}`；code 为 `<域>_<原因>` 大写下划线；sample 域前缀 `SAMPLE_`（backend-design §4.2）
- 路由前缀 `/v1/`；`/healthz` `/readyz` 在认证之外（backend-design §4.1）
- 2026-09-06 用户决策（执行中变更）：user/order demo **移出 workspace**（备份 /tmp/nemo-demo-backup）；kit v2.1 冻结约束解除，v2.1 包已删除，kit 收缩为三包终态。原"冻结服务必须持续可编译"约束作废，Makefile 统一 `nemo/...` 通配
- 端口：sample = 8085（backend-design §7）
- AGENTS.md：所有函数/类型必须中文注释；Go 日志用 logx/slog，禁止 fmt.Println 打日志
- 配置一律 etc/<name>.yaml + 环境变量覆盖，所有配置必须有默认值（backend-design §7）

---

### Task 1: 工程基座与工具链（F1）

**Files:**
- Create: `Makefile`（仓库根）
- Create: `README.md`（仓库根）
- Modify: `kit/go.mod`（新增 go-zero 依赖）

**Interfaces:**
- Produces: 根 Makefile 目标 `gen` / `build` / `test` / `lint` / `contract-check`（后续任务与 CI 直接调用）；kit 对 go-zero 的依赖版本（后续所有 kit 代码可用）

- [x] **Step 1: 确认工具链并安装 goctl**

```bash
go version   # 期望 go1.26.x
go install github.com/zeromicro/go-zero/tools/goctl@latest
goctl --version
```

Expected: goctl 打印版本号。若安装失败，改用 `go install github.com/zeromicro/go-zero/tools/goctl@v1.8.4`（与依赖版本一致即可）。

- [x] **Step 2: kit 引入 go-zero 依赖**

```bash
cd kit && go get github.com/zeromicro/go-zero@latest && go mod tidy && go build ./...
```

Expected: kit 编译通过（v2.1 包不受影响）。记录解析到的版本号（下一步写入 README）。

- [x] **Step 3: 写根 Makefile**

```makefile
# 根 Makefile：全仓构建与校验入口（版本与工具链说明见 README.md）
GOZERO_VERSION := $(shell grep 'zeromicro/go-zero v' kit/go.mod | awk '{print $$2}')

.PHONY: gen build test lint contract-check

# gen: 重新生成全部服务的 goctl 代码（新增服务后在此登记）
gen:
	cd services/sample && goctl api go -api sample.api -dir . --home ../../kit/template/goctl

# build: 编译全部 workspace 模块（含冻结的 user/order，防破坏）
build:
	go build ./kit/... ./services/...

# test: 全部测试
test:
	go test ./kit/... ./services/sample/...

# lint: go vet 全模块
lint:
	go vet ./kit/... ./services/sample/...

# contract-check: .api 契约与生成代码一致性（重新生成到临时目录后 diff）
contract-check:
	rm -rf /tmp/contract-check && mkdir -p /tmp/contract-check
	cd services/sample && goctl api go -api sample.api -dir /tmp/contract-check/sample --home ../../kit/template/goctl
	diff -r services/sample/internal/handler /tmp/contract-check/sample/internal/handler
	diff -r services/sample/internal/types /tmp/contract-check/sample/internal/types
	diff -r services/sample/internal/logic /tmp/contract-check/sample/internal/logic
	@echo "contract-check OK"
```

注意：`gen`/`contract-check` 依赖 Task 4 的模板目录与 Task 5 的 sample，此处先落文件，目标在后续任务验证。

- [x] **Step 4: 写根 README.md**

内容必须包含：项目一句话简介；Go/go-zero/goctl 版本与安装命令（go-zero 版本用 Step 2 解析到的实际值）；`make build/test/lint/contract-check` 用法；目录结构一段话（kit、services、docs、console-frontend）；指向 backend-design.md（接入指南）与 docs/superpowers/specs/（设计）。

- [x] **Step 5: 验证冻结服务可编译**

```bash
make build
```

Expected: kit、services/user、services/order 全部编译通过（user/order 无 go-zero 依赖也能编——kit 加依赖未改其 API）。

- [x] **Step 6: 打标**

勾选本任务全部步骤，TodoWrite 标记 Task 1 完成。

---

### Task 2: kit/errors——错误构造器 + go-zero 统一错误出口（F2a）

**Files:**
- Modify: `kit/errors/errors.go`（现有 Error struct 与 New 保留不动）
- Create: `kit/errors/constructors.go`
- Create: `kit/errors/gozero.go`
- Create: `kit/errors/ctxid.go`
- Test: `kit/errors/errors_test.go`

**Interfaces:**
- Consumes: go-zero `rest/httpx`（Task 1 引入）
- Produces（后续所有服务与 kit/logging 使用）:
  - `func BadRequest(code, message string) *Error`（及 Unauthorized/Forbidden/NotFound/Conflict/Internal，状态码分别 400/401/403/404/409/500）
  - `func WithRequestID(ctx context.Context, id string) context.Context`
  - `func RequestIDFrom(ctx context.Context) string`
  - `func InstallErrorHandler()`——注册 go-zero 全局错误出口，输出 `{"error":{"code","message","request_id"}}`

- [x] **Step 1: 写失败测试**

```go
package errors

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestConstructors(t *testing.T) {
	cases := []struct {
		name   string
		got    *Error
		status int
		code   string
	}{
		{"BadRequest", BadRequest("SAMPLE_BAD", "参数错误"), 400, "SAMPLE_BAD"},
		{"Unauthorized", Unauthorized("SAMPLE_NO_AUTH", "未登录"), 401, "SAMPLE_NO_AUTH"},
		{"Forbidden", Forbidden("SAMPLE_DENIED", "无权限"), 403, "SAMPLE_DENIED"},
		{"NotFound", NotFound("SAMPLE_NOT_FOUND", "不存在"), 404, "SAMPLE_NOT_FOUND"},
		{"Conflict", Conflict("SAMPLE_DUP", "重复"), 409, "SAMPLE_DUP"},
		{"Internal", Internal("SAMPLE_INTERNAL", "内部错误"), 500, "SAMPLE_INTERNAL"},
	}
	for _, c := range cases {
		if c.got.Status != c.status || c.got.Code != c.code {
			t.Errorf("%s: got (%d,%s), want (%d,%s)", c.name, c.got.Status, c.got.Code, c.status, c.code)
		}
	}
}

func TestRequestIDContext(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-1")
	if RequestIDFrom(ctx) != "req-1" {
		t.Fatal("request id round-trip failed")
	}
	if RequestIDFrom(context.Background()) != "" {
		t.Fatal("empty ctx should return empty id")
	}
}

func TestErrorBody(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-9")
	status, body := errorBody(ctx, NotFound("SAMPLE_NOT_FOUND", "不存在"))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d", status)
	}
	raw, _ := json.Marshal(body)
	want := `{"error":{"code":"SAMPLE_NOT_FOUND","message":"不存在","request_id":"req-9"}}`
	if string(raw) != want {
		t.Fatalf("body = %s, want %s", raw, want)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `cd kit && go test ./errors/ -v`
Expected: FAIL（constructors/WithRequestID 未定义）

- [x] **Step 3: 实现 ctxid.go**

```go
package errors

import "context"

// ctxKey 是 request id 的 context 私有键类型，防止跨包键冲突。
type ctxKey struct{}

// WithRequestID 将请求 id 写入 context（kit/logging 中间件写入，错误出口读取）。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestIDFrom 从 context 取请求 id；不存在返回空串。
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}
```

- [x] **Step 4: 实现 constructors.go**

```go
package errors

// 六个语义构造器：状态码与错误码一一对应，logic 层禁止手工构造非语义化错误。
// BadRequest 返回 400 错误。
func BadRequest(code, message string) *Error { return New(400, code, message) }

// Unauthorized 返回 401 错误。
func Unauthorized(code, message string) *Error { return New(401, code, message) }

// Forbidden 返回 403 错误。
func Forbidden(code, message string) *Error { return New(403, code, message) }

// NotFound 返回 404 错误。
func NotFound(code, message string) *Error { return New(404, code, message) }

// Conflict 返回 409 错误。
func Conflict(code, message string) *Error { return New(409, code, message) }

// Internal 返回 500 错误；原始 err 不进响应体，只进日志。
func Internal(code, message string) *Error { return New(500, code, message) }
```

- [x] **Step 5: 实现 gozero.go（统一错误出口）**

```go
package errors

import (
	"encoding/json"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// body 是错误响应体结构，全栈统一（backend-design §4.2），前端依赖此格式。
type body struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// errorBody 把 error 转为 (HTTP 状态码, 响应体)：
//   - *Error：按其 Status/Code 输出
//   - 其他 error：一律 500 SAMPLE_INTERNAL，原始 err 内容不进响应体（防泄漏）
func errorBody(ctx context.Context, err error) (int, body) {
	var b body
	b.Error.RequestID = RequestIDFrom(ctx)
	if e, ok := err.(*Error); ok {
		b.Error.Code, b.Error.Message = e.Code, e.Message
		return e.Status, b
	}
	b.Error.Code, b.Error.Message = "SAMPLE_INTERNAL", "内部错误"
	return http.StatusInternalServerError, b
}

// InstallErrorHandler 注册 go-zero 全局错误出口。服务 main.go 启动时调用一次。
func InstallErrorHandler() {
	httpx.SetErrorHandler(func(err error) (int, any) {
		status, b := errorBody(context.Background(), err)
		return status, b
	})
}

//Marshal 保留 json 导入引用（body 结构由 httpx 序列化输出）
var _ = json.Marshal
```

注意两处实施要点：① `errorBody` 的 ctx 参数在 `SetErrorHandler` 回调里拿不到——go-zero 回调签名只有 err。核对当前依赖版本的 `httpx` 是否提供带 ctx 的入口（`go doc github.com/zeromicro/go-zero/rest/httpx | grep ErrorHandler`）；若无，`InstallErrorHandler` 改为包装 `httpx.SetOkHandler` 不可用时的替代方案：在 kit/logging 的 AccessLog 中间件里兜底渲染错误响应（logic 返回 error 前写入），实现仍收敛在 kit 内。② 删除 `var _ = json.Marshal` 占位行——它是为了编译通过而留，正式实现（无论走哪条路）应保证 import 干净。

- [x] **Step 6: 运行测试确认通过**

Run: `cd kit && go test ./errors/ -v && go build ./...`
Expected: PASS；kit 全部编译通过（含冻结包）。

- [x] **Step 7: 打标**

勾选全部步骤，TodoWrite 标记 Task 2 完成。

---

### Task 3: kit/logging——日志初始化 + 中间件（F2b）

**Files:**
- Create: `kit/logging/logging.go`
- Create: `kit/logging/middleware.go`
- Test: `kit/logging/logging_test.go`

**Interfaces:**
- Consumes: `kit/errors.WithRequestID / RequestIDFrom`（Task 2）；go-zero `logx`（Task 1）
- Produces:
  - `func Init(serviceName string, prod bool)`——本地 stdout / 生产 JSON 文件轮转（架构 §9.2）
  - `func RequestID(next http.HandlerFunc) http.HandlerFunc`——取 `X-Request-ID`（无则生成），写 context（经 kit/errors）+ 回写响应头
  - `func AccessLog(next http.HandlerFunc) http.HandlerFunc`——输出 `method/path/status/duration_ms/request_id` 字段

- [x] **Step 1: 确认 logx API 表面**

Run: `go doc github.com/zeromicro/go-zero/core/logx LogConf`
Expected: 列出 LogConf 字段。确认 `ServiceName / Mode / Encoding / Path / Level` 存在；若字段名有出入，以当前版本为准调整 Step 3 代码（语义不变：本地 console、生产 file+json）。

- [x] **Step 2: 写失败测试**

```go
package logging

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDGenerated(t *testing.T) {
	var got string
	h := RequestID(func(w http.ResponseWriter, r *http.Request) {
		got = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	if got == "" {
		t.Fatal("expected generated request id in context")
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header")
	}
}

func TestRequestIDPassedThrough(t *testing.T) {
	var got string
	h := RequestID(func(w http.ResponseWriter, r *http.Request) {
		got = RequestIDFrom(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
	req.Header.Set("X-Request-ID", "req-fixed")
	h(httptest.NewRecorder(), req)
	if got != "req-fixed" {
		t.Fatalf("got %q, want req-fixed", got)
	}
}

func TestAccessLog(t *testing.T) {
	var sb strings.Builder
	// 注入测试 writer：logx.SetWriter 由实施时按 Task 3 Step 1 确认的 API 决定注入方式；
	// 若版本不支持便捷注入，则本测试降级为字段构造函数的单测（statusFronemoriter 逻辑）。
	h := AccessLog(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/v1/items", nil))
	_ = sb
	if rec.Code != http.StatusTeapot {
		t.Fatal("next handler status lost")
	}
}
```

- [x] **Step 3: 运行测试确认失败**

Run: `cd kit && go test ./logging/ -v`
Expected: FAIL（包未定义）

- [x] **Step 4: 实现 logging.go**

```go
// Package logging 统一 go-zero logx 的初始化与访问日志字段约定（架构 §9.2、backend-design §8）。
package logging

import (
	"github.com/zeromicro/go-zero/core/logx"
)

// Init 初始化全局日志：本地（prod=false）输出 stdout 文本；生产（prod=true）输出 JSON 文件并按天/按大小轮转（logx 内置轮转）。
// 必须在 main 最先调用。
func Init(serviceName string, prod bool) {
	conf := logx.LogConf{ServiceName: serviceName, Encoding: "json"}
	if prod {
		conf.Mode = "file"
		conf.Path = "logs/" + serviceName
		conf.Level = "info"
	} else {
		conf.Mode = "console"
		conf.Level = "debug"
	}
	logx.Must(logx.SetUp(conf))
}
```

- [x] **Step 5: 实现 middleware.go**

```go
package logging

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	"nemo/kit/errors"
)

// RequestID 中间件：读取入站 X-Request-ID（无则生成 16 字节 hex），
// 写入 context（供错误出口/业务取用）并回写响应头，全链路排障串联。
func RequestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newID()
		}
		w.Header().Set("X-Request-ID", id)
		next(w, r.WithContext(errors.WithRequestID(r.Context(), id)))
	}
}

// newID 生成 16 字节随机 hex 作为请求 id。
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// statusWriter 捕获下游写入的状态码，供访问日志读取。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// AccessLog 中间件：输出 method/path/status/duration_ms/request_id 字段（backend-design §8 字段约定）。
func AccessLog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next(sw, r)
		logx.WithFields(logx.Field{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      sw.status,
			"duration_ms": time.Since(start).Milliseconds(),
			"request_id":  errors.RequestIDFrom(r.Context()),
		}).Info("access")
	}
}
```

- [x] **Step 6: 运行测试确认通过**

Run: `cd kit && go test ./logging/ -v && go build ./... && go vet ./...`
Expected: PASS（TestAccessLog 若因 writer 注入降级，相应断言注释说明）。

- [x] **Step 7: 打标**

勾选全部步骤，TodoWrite 标记 Task 3 完成。

---

### Task 4: kit/template——goctl 定制模板 + kit/README（F2c）

**Files:**
- Create: `kit/template/goctl/`（`goctl template init` 产物 + 修改）
- Create: `kit/README.md`

**Interfaces:**
- Consumes: Task 1 的 goctl；Task 2/3 的 kit/errors、kit/logging
- Produces: `--home ../../kit/template/goctl` 可用的生成模板——生成物自带：main 中调用 `kiterrors.InstallErrorHandler()`、`kitlog.Init`、挂 `RequestID/AccessLog` 中间件；handler 包注释规范；svc 显式装配注释。contract-check 目标（Task 1 Makefile）可运行

- [x] **Step 1: 生成默认模板**

```bash
cd kit && goctl template init --home template/goctl
ls template/goctl/api
```

Expected: 出现 api/handler.tpl、api/logic.tpl、api/main.tpl、api/svc.tpl 等文件。

- [x] **Step 2: 阅读并修改模板**

先 `cat` 全部 api/*.tpl 熟悉变量（goctl 版本间模板变量有差异，以 init 产物为准）。最小修改集：

1. `main.tpl`：在 `server.Start()` 前追加三行接线（import 路径按生成服务的 module 名 `nemo/services/{{.pkgName}}`——若模板变量不足则改为生成后由 checklist 手工补，见 Step 4 降级路径）：
   - `kitlog.Init("{{.pkgName}}", false)`
   - `kiterrors.InstallErrorHandler()`
   - `rest.WithMiddleware(kitlog.RequestID, kitlog.AccessLog)`
2. `handler.tpl`：包注释加一行"handler 只做协议转换，业务规则在 logic（backend-design §2）"
3. `logic.tpl`：包注释加"业务规则层，单测重点；错误用 kit/errors 语义构造器返回"
4. `svc.tpl`：注释加"显式装配依赖，无 DI 容器；repo/client 接口在此注入"

- [x] **Step 3: 验证模板可用**

用临时 .api 试生成（放 /tmp，不进仓库）：

```bash
printf 'type PingReq {}\n\nservice api {\n  @handler PingHandler\n  get /v1/ping returns (string)\n}\n' > /tmp/t.api
# 补全 .api 必需的 syntax/info 块后：
goctl api go -api /tmp/t.api -dir /tmp/t-svc --home kit/template/goctl
cd /tmp/t-svc && go build ./...
```

Expected: 生成物编译通过（编译不过的最常见原因是模板 import 路径写死错误——修模板直到通过）。

- [x] **Step 4: 若模板变量不足以完成 main 接线（降级路径，如实记录）**

保持模板只做注释级修改，main 接线写入 backend-design §3 checklist 成为"生成后手工三行"步骤，并在 kit/README 注明模板能力边界。

- [x] **Step 5: 写 kit/README.md**

内容：三个包各自用途与关键 API（errors 六构造器 + InstallErrorHandler / RequestIDFrom；logging Init + 两中间件；template 用法 `goctl api go -api x.api -dir . --home kit/template/goctl`）；三纪律（无业务逻辑/无业务名词/非强制依赖）；变更评审要求（架构 §5：双人审批）；v2.1 冻结包清单与退役计划（随 user/order 下线退役，不新增依赖方）。

- [x] **Step 6: 打标**

勾选全部步骤，TodoWrite 标记 Task 4 完成。

---

### Task 5: services/sample 骨架生成（F4a）

**Files:**
- Create: `services/sample/sample.api`
- Create: `services/sample/`（goctl 生成物：main.go、etc/sample.yaml、internal/{config,handler,logic,svc,types}）
- Modify: `go.work`（加 `./services/sample`）

**Interfaces:**
- Consumes: kit/template（Task 4）
- Produces（Task 6/7 依赖）:
  - types：`Item{Id string, Name string, Desc string}`、`CreateItemReq{Name string, Desc string}`、`UpdateItemReq{Id string, Name string, Desc string}`、`ItemIdReq{Id string}`、`ListItemsResp{Items []Item, Total int}`
  - 路由：`GET/POST /v1/items`、`GET/PUT/DELETE /v1/items/:id`（goctl path 变量写法 `:id`）
  - config：`config.Config{rest.RestConf, Mysql struct{Dsn string}}`（Dsn 默认空 = 内存模式）

- [x] **Step 1: 写 sample.api 契约**

```
syntax = "v1"

info (
	title:   "Sample Service API"
	desc:    "标准样板服务：新服务接入的对照标准（backend-design §2），中性 Item 资源无业务含义"
	author:  "platform"
	version: "v1"
)

type Item {
	Id   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type CreateItemReq {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type UpdateItemReq {
	Id   string `path:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type ItemIdReq {
	Id string `path:"id"`
}

type ListItemsResp {
	Items []Item `json:"items"`
	Total int    `json:"total"`
}

service sample-api {
	@handler CreateItem
	post /v1/items (CreateItemReq) returns (Item)

	@handler ListItems
	get /v1/items returns (ListItemsResp)

	@handler GetItem
	get /v1/items/:id (ItemIdReq) returns (Item)

	@handler UpdateItem
	put /v1/items/:id (UpdateItemReq) returns (Item)

	@handler DeleteItem
	delete /v1/items/:id (ItemIdReq)
}
```

- [x] **Step 2: 生成骨架**

```bash
cd services/sample && goctl api go -api sample.api -dir . --home ../../kit/template/goctl
```

- [x] **Step 3: 登记 workspace 与端口**

go.work 增 `./services/sample`；etc/sample.yaml：`Port: 8085`、`Mysql: {Dsn: ""}`（所有配置有默认值）。

- [x] **Step 4: 验证骨架可跑**

```bash
cd services/sample && go build ./... && go run . &
curl localhost:8085/v1/items   # handler 未实现时返回逻辑层缺省响应即可，重点是服务起来
```

Expected: 服务启动无错；编译通过。验证后停进程。

- [x] **Step 5: 打标**

勾选全部步骤，TodoWrite 标记 Task 5 完成。

---

### Task 6: sample repo 层——接口 + 内存实现 + MySQL 参考实现（F3a）

**Files:**
- Create: `services/sample/internal/repo/repo.go`
- Create: `services/sample/internal/repo/memory.go`
- Create: `services/sample/internal/repo/mysql.go`
- Create: `services/sample/migrations/001_init.sql`
- Create: `services/sample/cmd/migrate/main.go`
- Test: `services/sample/internal/repo/repo_test.go`

**Interfaces:**
- Consumes: Task 5 types（Item）
- Produces（Task 7 依赖）:
  - `type ItemRepo interface { Create(ctx context.Context, it repo.Item) (repo.Item, error); Get(ctx, id string) (repo.Item, error); List(ctx) ([]repo.Item, error); Update(ctx, it repo.Item) (repo.Item, error); Delete(ctx, id string) error }`
  - `func NewMemory() ItemRepo`；`func NewMySQL(dsn string) (ItemRepo, error)`
  - repo.Item 为存储模型（`ID/Name/Desc/CreatedAt/UpdatedAt`，与 types.Item 分离，不外泄）

- [x] **Step 1: 写失败测试（内存实现）**

```go
package repo

import (
	"context"
	"errors"
	"testing"
)

// TestItemRepo 用同一组断言约束全部实现（内存必跑；MySQL 走 TestMySQLRepo 环境开关）。
func testItemRepo(t *testing.T, r ItemRepo) {
	ctx := context.Background()
	created, err := r.Create(ctx, Item{Name: "a", Desc: "d1"})
	if err != nil || created.ID == "" {
		t.Fatalf("create: %v", err)
	}
	got, err := r.Get(ctx, created.ID)
	if err != nil || got.Name != "a" {
		t.Fatalf("get: %v", err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list: %v", err)
	}
	updated, err := r.Update(ctx, Item{ID: created.ID, Name: "b"})
	if err != nil || updated.Name != "b" {
		t.Fatalf("update: %v", err)
	}
	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestMemoryRepo(t *testing.T) { testItemRepo(t, NewMemory()) }
```

- [x] **Step 2: 运行确认失败**

Run: `cd services/sample && go test ./internal/repo/ -v`
Expected: FAIL（类型未定义）

- [x] **Step 3: 实现 repo.go / memory.go**

repo.go：定义 `Item` 存储模型、`ErrNotFound = errors.New("item not found")`、`ItemRepo` 接口（方法带中文注释）。
memory.go：map + 自增 id（`fmt.Sprintf("it-%d", n)`）+ RWMutex；`Create` 填时间戳；`Get` 未命中返回 `ErrNotFound`。

- [x] **Step 4: 运行测试确认通过**

Run: `cd services/sample && go test ./internal/repo/ -v`
Expected: PASS

- [x] **Step 5: 实现 mysql.go + 迁移**

mysql.go：`database/sql` 直写（无 ORM），五种方法各一条 SQL；`NewMySQL` ping 验证 DSN。
`migrations/001_init.sql`：

```sql
CREATE TABLE IF NOT EXISTS items (
    id         VARCHAR(64) PRIMARY KEY,
    name       VARCHAR(128) NOT NULL,
    descr      VARCHAR(512) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

`cmd/migrate/main.go`：读 `migrations/*.sql` 按文件名排序，跳过 `schema_migrations` 表中已执行项，逐个事务执行并记录；环境变量 `MYSQL_DSN` 必填，缺省报错退出。

- [x] **Step 6: MySQL 集成测试（环境开关）**

```go
func TestMySQLRepo(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN 未设置，跳过 MySQL 集成测试")
	}
	r, err := NewMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	testItemRepo(t, r)
}
```

Run: `cd services/sample && go test ./internal/repo/ -v`（无 DSN）
Expected: MySQL 用例 SKIP，其余 PASS。

- [x] **Step 7: 打标**

勾选全部步骤，TodoWrite 标记 Task 6 完成。

---

### Task 7: sample logic 补全 + client 基座 + 健康检查（F3b/F4b）

**Files:**
- Modify: `services/sample/internal/logic/*`（goctl 生成的 5 个 logic）
- Modify: `services/sample/internal/svc/servicecontext.go`
- Modify: `services/sample/main.go`（健康检查路由 + ready 函数）
- Create: `services/sample/internal/clientx/client.go`（跨服务调用基座样板）
- Test: `services/sample/internal/logic/*_test.go`、`services/sample/internal/clientx/client_test.go`

**Interfaces:**
- Consumes: Task 6 `repo.ItemRepo / NewMemory / ErrNotFound`；Task 2 `kiterrors`；Task 3 `kitlog`
- Produces:
  - logic CRUD：成功返回 Item；不存在返回 `kiterrors.NotFound("SAMPLE_NOT_FOUND", "条目不存在")`；校验失败 `BadRequest("SAMPLE_NAME_REQUIRED", "name 不能为空")`
  - `clientx.Client`：`func New(baseURL string) *Client`、`func (c *Client) Do(ctx context.Context, method, path string, in, out any) error`——自动带 X-Request-ID（取 ctx）与 Bearer（ctx 有则带）、超时 5s、go-zero breaker、非 2xx 解析 `*kiterrors.Error` 原样返回

- [x] **Step 1: 写 logic 失败测试（以 Create/Get 为例，其余同构补全）**

```go
func TestCreateItem(t *testing.T) {
	l := logic.NewCreateItemLogic(context.Background(), svc.NewTestServiceContext())
	item, err := l.CreateItem(&types.CreateItemReq{Name: "a"})
	if err != nil || item.Id == "" || item.Name != "a" {
		t.Fatalf("create: %+v %v", item, err)
	}
}

func TestGetItemNotFound(t *testing.T) {
	l := logic.NewGetItemLogic(context.Background(), svc.NewTestServiceContext())
	_, err := l.GetItem(&types.ItemIdReq{Id: "missing"})
	if kitErr, ok := err.(*kiterrors.Error); !ok || kitErr.Code != "SAMPLE_NOT_FOUND" {
		t.Fatalf("want SAMPLE_NOT_FOUND, got %v", err)
	}
}
```

`svc.NewTestServiceContext()`：svc 包内加的测试构造器（内存 repo + 空 logger），单测专用。

- [x] **Step 2: 运行确认失败** → Run: `cd services/sample && go test ./internal/logic/ -v` → Expected: FAIL

- [x] **Step 3: 实现 svc 装配与 5 个 logic**

ServiceContext：`Config` + `ItemRepo repo.ItemRepo`（`Dsn==""` 时 `repo.NewMemory()`，否则 `repo.NewMySQL` 失败即 panic——启动期错误不吞）。logic 按 Step 1 语义实现，错误一律 kit/errors 构造器。

- [x] **Step 4: 实现健康检查**

main.go 注册（认证之外）：

```go
server.AddRoute(rest.Route{Method: http.MethodGet, Path: "/healthz", Handler: func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}})
server.AddRoute(rest.Route{Method: http.MethodGet, Path: "/readyz", Handler: svcCtx.ReadyHandler})
```

`svc.ReadyHandler`：MySQL 模式 ping 成功才 200；内存模式恒 200。

- [x] **Step 5: 运行 logic 测试确认通过**

Run: `cd services/sample && go test ./internal/logic/ -v` → Expected: PASS

- [x] **Step 6: 写 client 基座（client.go）+ httptest 测试**

client.go 要点（全部中文注释）：`http.Client{Timeout: 5 * time.Second}`；请求前注入 `X-Request-ID`（`kiterrors.RequestIDFrom(ctx)`）与 `Authorization: Bearer`（ctx 注入函数 `WithToken`）；每次调用包 `breaker.DoWithAcceptable`（按主机名分熔断器，HTTP 5xx 计失败）；非 2xx 读 body 按 `{"error":{...}}` 解析为 `kiterrors.New(status, code, message)` 返回。
client_test.go：httptest 对端分别验证——200 正常解码出参、404 错误体解析为 `SAMPLE_NOT_FOUND`、请求头带上了 request id 与 token。

- [x] **Step 7: 全量验证**

Run: `cd services/sample && go test ./... && make lint && make contract-check`
Expected: 全部 PASS / OK。

- [x] **Step 8: 冒烟脚本**

Create `services/sample/scripts/smoke.sh`：起服务 → curl 建/查/改/删一条 Item → 断言各响应（含错误体 404 用例、`X-Request-ID` 回写头）→ 输出 PASS/FAIL。Run: `bash services/sample/scripts/smoke.sh` → Expected: PASS。

- [x] **Step 9: 打标**

勾选全部步骤，TodoWrite 标记 Task 7 完成。

---

### Task 8: 接入指南校准（F5）

**Files:**
- Modify: `docs/backend-design.md`（校准点见 Steps）

**Interfaces:**
- Consumes: Task 1-7 全部实际产物（版本号、模板行为、sample 命令）
- Produces: 与实产物一致的接入指南

- [x] **Step 1: 校准 §1/§2**

技术栈行补 go-zero 实际锁定版本；§2 目录结构补 `cmd/migrate/`、`scripts/`、`clientx` 基座样板说明（含一句"client 基座样板在 sample，复制到新服务按需裁剪"）。

- [x] **Step 2: 校准 §3 checklist**

每一步落到可执行命令/文件：写 `.api` → `goctl api go -api <name>.api -dir . --home ../../kit/template/goctl` → go.work 登记 → 端口顺延登记 §7 → svc 装配 repo/client → main 三行接线（Task 4 模板未覆盖时的手工项，如适用）→ `make lint && make contract-check` → `/healthz` `/readyz` 验证。末尾加一行：对照标准见 `services/sample`。

- [x] **Step 3: 校准 §9 本地开发**

sample 的 run/迁移命令（`go run ./cmd/migrate` 需 `MYSQL_DSN`）写入；冻结服务说明（user/order 端口保留但不再维护）。

- [x] **Step 4: dry-run 走查**

按校准后的 checklist 从零新建 throwaway 服务（/tmp 下，不进仓库），走完全部步骤并跑通 `/healthz`；过程卡点回写指南。走查完成后删除 throwaway。

Expected: 半日内可走通（spec §4 F5 标准）；卡点已回写。

- [x] **Step 5: 打标**

勾选全部步骤，TodoWrite 标记 Task 8 完成；Phase 0 收尾汇报（F1-F5 状态、sample 演示要点、遗留项：MySQL 实例接线、git init 建议、网关注册中心后置）。

---

## Self-Review 记录

- **Spec 覆盖**：F1→Task 1；F2→Task 2/3/4（errors/logging/template+README）；F3→Task 6/7（repo/client 基座）；F4→Task 5/7/8 内 sample 建成与走查；F5→Task 8。排除项（网关/注册中心/Redis/user-order 迁移）无任务，符合 spec。
- **占位符**：Task 2 Step 5 与 Task 3 Step 2 各含一处"go doc 核对当前版本 API"——这是版本表面验证步骤而非需求占位，决策规则已写明；Task 4 Step 4 为模板变量不足时的显式降级路径。
- **类型一致性**：`repo.ItemRepo` 五方法签名在 Task 6 定义、Task 7 消费一致；`kiterrors.RequestIDFrom/WithRequestID` 在 Task 2 定义、Task 3/7 消费一致；types.Item 字段 Task 5 定义、Task 6/7 消费一致；`SAMPLE_` 错误码前缀全局一致。
