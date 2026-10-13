# 后端基础框架实施设计（Phase 0：基座 + 标准样板 + 接入指南校准）

> 状态：实施完成 v1.1（2026-09-06，Phase 0 F1-F5 全部交付；F5 校准与 dry-run 走查见 Task 8）
> 日期：2026-09-06（v1.0 同日；v1.1 变更：user/order demo 废弃不再迁移，样板改为新建 `services/sample`）
> 定位：面向 20~30 个 Go 服务的公共基座——go-zero 服务骨架、kit 三包、repo/client 基座模式、标准样板服务、接入指南（backend-design.md）按实产物校准。
> 依据：architecture.md §3.2（go-zero 选型）、§5（kit 纪律）、§9.2（日志）；backend-design.md v2.3（目标结构，本文使其可执行）。
> 后续：权限管理模块（`2026-09-06-authz-implementation-design.md`）在本框架之上实施，是其第一个"按指南接入"的真实服务。

---

## 1. 背景与目标

**现状**：`services/user`、`services/order` 为 9月2日的微前端/微服务验证 demo（v2.1 标准库骨架）。**2026-09-06 决策（升级）**：两 demo **移出 workspace**（备份 `/tmp/nemo-demo-backup`），kit v2.1 冻结包随之退役删除——kit 直接收缩为 `errors`/`logging`/`template` 三包终态，无历史包袱。backend-design v2.2 描述的 go-zero 目标结构由本文落地。

**目标**：新服务从零到可跑 ≤ 半天；公共约定（错误体、日志、健康检查、repo、client）**代码化**，不靠口头与复制粘贴漂移；产出一个可直接对照的标准样板服务。

**不做**（显式排除）：

- 网关服务（services/gateway）——独立模块，另行设计
- 注册中心接入——env 定址（`XXX_SERVICE_URL`），启用后业务代码不变
- CI 流水线搭建（只交付可被 CI 调用的 Makefile 目标与脚本）
- demo 前端微应用的同步改造（console-frontend 中指向 user/order 的菜单与登录为过渡态，auth 服务上线后切换）

## 2. 交付物

### 2.1 工程基座

- go-zero 版本锁定（根 README/Makefile 记录版本与 goctl 安装方式）
- Makefile（根 + kit）：`gen`（goctl 生成）/ `build` / `test` / `lint` / `contract-check`（`.api` 与生成代码一致性）

> 交付偏差（2026-09-06 终审注记）：kit/Makefile 未单独交付（根 Makefile 覆盖全仓）；clientx 基座以"复制裁剪"形态交付（sample 未挂载调用示例），backend-design §5 为准。
- 新服务骨架 = goctl 产出 + kit 模板接线，目录结构即 backend-design §2

### 2.2 kit 三包（收敛到 backend-design §1 约定）

| 包 | 内容 | 完成标准 |
|---|---|---|
| `kit/errors` | 错误码构造器（BadRequest/Unauthorized/Forbidden/NotFound/Conflict/Internal）+ **go-zero rest 统一错误出口**（输出 `{"error":{code,message,request_id}}`；Internal 不泄原始 err，只进日志）。对 v2.1 API 只加不改，冻结中的 user/order 仍可编译 | 单测覆盖各状态码、错误体格式、request_id 透传 |
| `kit/logging` | logx 字段约定（`method/path/status/duration_ms/trace_id`）+ 轮转初始化（生产 JSON 文件轮转、本地 stdout，架构 §9.2） | 初始化函数 + 字段中间件单测 |
| `kit/template` | goctl 定制模板：handler/logic/svc 骨架自带 kit/errors 接线、repo/client 分层目录与注释 | 模板生成 → contract-check 通过 → 服务可编译可跑 |
| `kit/README.md` | 每包用途、三纪律（无业务逻辑/无业务名词/非强制依赖）、变更评审要求（AGENTS.md 可运维性规范：模块含 README） | — |

### 2.3 repo / client 基座模式（样板 + 文档形态，不做强制框架库）

- **repo**：接口约定 + 内存实现样板 + MySQL 参考实现（`database/sql` 无 ORM，backend-design §1 既定）；`migrations/` 目录 + 极简 runner（sql 按序执行 + 版本表）；集成测试可选（无实例时 skip）
- **client**：`internal/client/` 基座样板——X-Request-ID 与 Bearer 透传、超时 5s、go-zero breaker、非 2xx 解析为 `*kiterrors.Error` 原样带回上游错误码（backend-design §5）；集成测试以 `httptest` server 为对端，不依赖真实服务
- 形态决策：**样板代码 + 指南文档**，不做成 import 即生效的框架库——kit 三纪律（backend-design §1：非强制依赖）

### 2.4 标准样板服务：新建 `services/sample`（:8085）

- 用途：其他模块接入的**对照标准**——不是业务服务，是"按指南从零接入"的活文档；新服务接入时对照它走 checklist
- 契约（`sample.api`）：Item 资源 CRUD（`GET/POST /v1/items`、`GET/PUT/DELETE /v1/items/{id}`），中性无业务含义
- 完整示范：`.api` 契约 → goctl 生成 → svc 装配 → repo 双实现（内存跑单测 / MySQL 参考实现就绪）→ kit/errors 错误出口 → `/healthz` `/readyz` → contract-check 通过 → client 样板挂载示例
- go.work 登记、backend-design §7 端口表登记

> 交付偏差（2026-09-06 终审注记）：kit/Makefile 未单独交付（根 Makefile 覆盖全仓）；clientx 基座以"复制裁剪"形态交付（sample 未挂载调用示例），backend-design §5 为准。

### 2.5 接入指南校准（backend-design.md 即指南本体，不另写新文档）

- 按实产物校准：模板命令实指、repo/client 用法实指、sample 服务作为对照范例
- 新服务 checklist（§3）补全"从骨架到联调上线"完整步骤（契约 → 生成 → 装配 → 健康检查 → 契约校验 → 登记 go.work/端口表）

## 3. 关键决策

1. go-zero api 形态，不引 zRPC/proto（backend-design 既定，不变）
2. **user/order demo 已移出 workspace**（/tmp/nemo-demo-backup 备份 + git 基线可考）：过渡期前端 SimpleLogin 登录（原指向 user :8081）不可用，auth（CAS）上线后切换；console-frontend 中指向 demo 的菜单为过渡态
3. **kit 收缩为三包终态**：errors（含 request-id context 键与统一错误出口）、logging（logx Init + RequestID/AccessLog 中间件）、template（Task 4）；v2.1 包 auth/config/httpx/middleware/reqid 已删除
4. repo/client 以样板形态交付；MySQL 参考实现随交付，真实接线各服务按需（权限模块在 M2 后段接）
5. 错误码体系沿用 `<域>_<原因>`；sample 用 `SAMPLE_` 前缀示范，权限模块 `AUTHZ_` 前缀为第一个真实消费方；kit 内部兜底码统一 `INTERNAL`

## 4. 里程碑与完成标准（任务完成后逐项打标）

| # | 里程碑 | 完成标准 |
|---|---|---|
| F1 | 工程基座 | goctl 可用；模板产出骨架服务可编译可跑；`/healthz` 返回 ok；Makefile 四目标可用 |
| F2 | kit 三包 | errors/logging 单测过；template 生成代码过 contract-check；kit/README.md 就位 |
| F3 | repo/client 基座 | 内存 + MySQL repo 样板、client 基座样板（httptest 对端）可用 |
| F4 | 标准样板 sample | sample 服务按 checklist 从零建成，CRUD 全通、错误体/健康检查合规、contract-check 过、指南对照走查一遍 |
| F5 | 指南校准 | backend-design.md 校准完（引用 sample 为范例）；按指南从零 dry-run 新建一个空服务，半日内走通全 checklist |

顺序：F1 → F2 → F3 → F4 → F5。与权限模块的衔接：authz M0（OpenFGA PoC）可与 F 阶段**并行**（不依赖基座）；authz M1 起依赖 F1/F2/F3。

## 5. 风险

| 风险 | 影响 | 处理 |
|---|---|---|
| goctl 模板定制复杂度超预期 | F2 工作量 | 降级为"goctl 默认模板生成 + 手调补齐"，如实记录进指南；template 包后置 |
| 无 CI 系统 | contract-check 落地 | 本地 Makefile 目标先跑起来，流水线接入后置 |
| 本地无 MySQL 实例 | F3 集成测试 | MySQL 实现交付但集成测试标注 skip 条件，不影响样板完整性 |
| 前端登录过渡期依赖冻结服务 | demo 体验 | user 服务冻结可手动启动；auth（CAS）上线后切换，Phase 0 不处理前端 |

## 6. 文档影响面

- backend-design.md：迁移说明更新为 v2.3（user/order 废弃决策落档）、端口表标注冻结/新增 sample 8085、本地开发示例改指 sample（本次同步修改）
- 新增：kit/README.md
- 权限模块 spec：M3 冒烟对象由 order 改为 sample（本次同步修改）
- architecture.md：无需改动

---

## 附录：终审遗留清单（2026-09-06 全分支终审 + 修复后，随权限模块迁移处理）

已修复（终审修复波 6eb417e）：README 凭据移出（.local-dev.md，gitignored）、kit go mod tidy（GOWORK=off 可构建）、errorBody/From 识别 %w 包装、repo.Create 注释口径、authz spec 悬空引用、kit/README 示例中性化、交付偏差注记。

随迁清单（按触发时机）：

| 项 | 触发时机 |
|---|---|
| RequestID 入站长度截断 | 权限模块 M0 开工顺手 |
| statusWriter 补 Unwrap/Flusher 透传 | 首个 SSE/流式服务接入前（必须） |
| ReadyHandler 全表 List 探活改最小查询 | 权限模块 M2 接 MySQL 前 |
| NewMySQL Ping 加超时 | M2 接线时 |
| repo 集成测试补时间戳语义断言 | 接真库时 |
| Makefile：Linux mktemp 适配；contract-check 注释与 -x 对齐；GOZERO_VERSION 变量清理 | 跨平台/CI 接入时 |
| clientx：熔断打开错误包装为 kit 语义；upstreamError 空 Code 校验；ctx 取消计入熔断评估 | kit/authz 实装（权限模块 M3）对齐时 |
