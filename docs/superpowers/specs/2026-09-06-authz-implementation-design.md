# 权限管理系统实施设计（auth + authz + 管理界面）

> 状态：**已实施（Phase A + Phase B，2026-09-07 M6 联调验收通过收官）**——里程碑打标见 §4，验收取证见 `.superpowers/sdd/2026-09-07-authz-phase-b/task-12-report.md`
> 日期：2026-09-06
> 定位：permission-design.md v1.1 的实施设计——回答"怎么落地、按什么顺序、每步完成标准"。授权模型、API 面、缓存与失效语义以 permission-design v1.1 为准，本文不重复。
> 决策记录：框架按 backend-design.md v2.2 目标态采用 **go-zero**（用户 2026-09-06 确认方案 B；现有 user/order 为 v2.1 标准库骨架，不在本次迁移范围）。
> 前置：后端基础框架 Phase 0（`2026-09-06-backend-foundation-design.md`）——kit 三包、goctl 模板、repo/client 基座由其交付；authz M0 可与其并行，M1 起依赖 F1/F2/F3。

---

## 1. 范围与交付边界

**交付**（全量闭环，登录→授权→判定→界面可用）：

| 组件 | 内容 |
|---|---|
| `services/authz`（:8084） | 管理 API、判定 API（Check / ListObjects / `/me/permissions`）、规则表导出、审计（先行+对账）、资源类型注册、bootstrap |
| `services/auth`（:8083） | CAS 登录（对接本地简易 CAS，地址环境变量化）、用户映射、平台 JWT 签发（claims 含 tenant/roles）、续签接口 |
| `kit/authz` | 判定 client：Checker 接口 + Remote 实现 + Require 中间件 + CheckObject，403/503 语义 |
| `console-frontend/apps/authz-admin` | 管理界面微应用：组织树、成员、平台角色、规则表、共享管理、审计查询六页 + 基座接入 + 权限过滤 |

**不在本次范围**（显式排除，防止蔓延）：

- 网关接入（规则表消费、API key 校验）——网关服务尚未实现
- Redis 判定缓存、规则表 Redis 导出、角色缓存登录兜底——均为消费面/容灾优化，接口留位不接线
- MySQL 生产接线——引擎 datastore 与 authz 业务库的 MySQL 实现预留（见 §3 存储策略），需要环境信息后独立任务
- user/order 迁移 go-zero、kit 定制 goctl 模板制作、CAS SLO、API key 完整生命周期（auth 只做签发最小集，见 §3.2）
- 多租户切换租户流程、API key 管理界面

## 2. 关键落地决策

1. **框架**：go-zero api 服务 + goctl 生成骨架，工程结构按 backend-design §2（`.api` 契约 → handler/logic/svc/repo/client，依赖单向）。生成与 kit 接线使用 Phase 0 基础框架产物（kit/template 定制模板、kit/errors、repo/client 基座样板），见 `2026-09-06-backend-foundation-design.md`。
2. **OpenFGA 接入**：以 Go module 引入（go.mod 锁定 ≥ v1.18.x），authz 进程内启动引擎 server 监听 `127.0.0.1`（端口配置），经**官方 Go client 回环访问**；不 import 引擎内部包——保留抽离为独立容器的可逆性。对外判定 API 是 authz 自有 REST 接口（go-zero handler → logic → 引擎 client），与 permission-design §3"读路径只走判定 API"一致。
3. **引擎 datastore**：dev 用 OpenFGA memory，prod 切 MySQL（`nemo_authz` 独立库）——配置项切换，同一条代码路径（openfga server 启动参数）。memory 模式重启丢 tuple，dev 可接受（bootstrap 幂等重建）。
4. **authz 业务库**（审计表、规则表、注册配置、模型版本）：repo 接口 + 内存实现先行；**2026-09-07 变更：本地 MySQL 已可用（凭据见 .local-dev.md，不进 git），Phase A 的 M2 直接交付 MySQL 实现（自建两库 nemo_authz 引擎存储 / nemo_authz_biz 业务）**，接口不变。
5. **JWT**：HS256 全栈共享 secret 不变（backend-design §6）。Claims 扩展为 `{sub, name, tenant, roles, exp}`（permission-design §5.1 网关本地判定所需；roles 为平台角色名数组）。TTL 1h + 续签接口（permission-design v1.1 §3.1/§5.1）。auth 服务用 go-zero rest jwt 配置做服务内校验。
6. **服务间调用**：demo 无注册中心，authz/auth 地址走环境变量（`AUTHZ_SERVICE_URL`、`AUTH_SERVICE_URL`），与 backend-design §5 一致；注册中心启用后业务代码不变。业务服务调判定 API 透传用户 JWT；资源关系写 API 一期同样以透传 JWT + `X-Service-Name` 头标识来源，落审计（permission-design §6.4 的服务身份凭证细则从简，后续收紧不破坏契约）。
7. **前端**：`apps/authz-admin` 加入 console-frontend monorepo（安全合规域，端口 5276，基座注册表加一条）；权限过滤按 permission-design §5.2（`/me/permissions` 服务级前缀匹配，不可见即隐藏）。
8. **首版模型来源**：`services/authz/` 仓内维护 `model.fga` 源文件（permission-design §6.3 三层存储中的"只读参照"），bootstrap 读取并写入引擎成为模型 v1；后续模型变更经管理 API 走版本化写入。

## 3. 组件设计

### 3.1 services/authz

`.api` 契约三组路由（认证经 rest jwt 配置 + 中间件）：

| 路由组 | 鉴权 | 内容（对应 permission-design §6.1 API 面） |
|---|---|---|
| `/v1/admin/*` | JWT + 平台管理员校验中间件（一期固定校验 claims.roles 含 `super_admin`；后续可切规则表口径，不改契约） | 组织树 CRUD、成员增删移、platform_role 定义与分配、规则表维护（前缀 × 角色分组标签）、资源类型注册、审计查询 |
| `/v1/authz/*` | 透传 JWT（业务服务调用） | `POST /check`、`POST /list-objects`、资源关系写接口（幂等） |
| `/v1/me/permissions` | JWT | 权限集合展开（规则表派生，服务级 + 动作级，permission-design §5.2） |

- `svc.ServiceContext` 装配：引擎 client（回环）、orgRepo/roleRepo/ruleRepo/auditRepo/regRepo（接口 + 内存实现）、logger
- logic 层按领域分文件（org.go / role.go / rule.go / share.go / audit.go / check.go）
- **审计先行 + 对账**：auditRepo 记 pending → 写引擎 → committed；定时任务扫 pending 超时回查修正（permission-design §6.2）
- **bootstrap**：`cmd/bootstrap/main.go` 独立入口（goctl 主入口之外的普通 main），幂等执行：建 Store → 写首版模型 → 根组织/超管（permission-design §6.5）；**偏差注记（2026-09-07）：初始 API key 生成推迟到 M4 随 auth 服务落地（key 生命周期归 auth），bootstrap 步骤 1-4 本期交付**；dev 启动也可选 `BOOTSTRAP=1` 内联执行
- 错误码前缀 `AUTHZ_`（如 `AUTHZ_DENIED`、`AUTHZ_ORG_CYCLE`、`AUTHZ_TYPE_NOT_FOUND`——未注册类型 fail-closed 拒绝）

### 3.2 services/auth

- 公开路由 `/v1/auth/cas/login`（302 → CAS）、`/v1/auth/cas/callback`（ticket → `serviceValidate` → 映射/创建用户 → 调 authz 取角色 → 签发 JWT）
- 业务路由 `/v1/me`（会话恢复）、`/v1/auth/refresh`（续签：旧 token 未过期即换发，roles 实时重取）
- 用户 repo：`id, name, email, idp_type, idp_subject`（一期内存；本地 CAS 用户首次登录自动建）
- API key 签发最小集：`POST /v1/api-keys`（一次性展示原始 key，仅存 SHA-256）——bootstrap 初始 key 依赖此逻辑，管理界面不做（§1 排除项）
- CAS 协议：CAS 3.0 `serviceValidate`（标准库 HTTP 即可，无重依赖；本地简易 CAS 兼容此协议）

### 3.3 kit/authz

- 面向 `net/http` 标准接口（与 go-zero rest 中间件兼容）：`authz.Require(checker, "order:manage")` 路由中间件、`authz.CheckObject(ctx, checker, "order", id, "can_view")`
- Checker 接口单测可 fake；Remote 实现自带最小 client 封装（超时/透传/错误码原样带回，与 Phase 0 clientx 基座同约定）
- 语义：明确无权 403 `AUTHZ_DENIED`；网络/超时/5xx 一律 503 `AUTHZ_SERVICE_UNAVAILABLE`（fail-closed，不降级放行）
- 注意：kit/authz 面向 `net/http` 标准接口，自带最小 client 封装（超时/透传/错误码原样带回，与 Phase 0 client 基座样板同约定）；go-zero rest 与标准库 mux 双兼容

### 3.4 前端 authz-admin

- 六页：组织树（el-tree + 成员管理抽屉）、平台角色（角色表 + 分配）、规则表（前缀 × 角色 × 动作分组标签）、共享管理（资源共享/撤销）、审计查询（时间范围 + 操作类型过滤）、资源类型注册
- 经 `@nemo/http` 调 `/api/v1`；菜单注册表新增服务条目（key: `authz`，安全合规域）；`hasPermission` 按钮级控制

## 4. 里程碑与完成标准（任务完成后逐项打标）

| # | 里程碑 | 完成标准 |
|---|---|---|
| M0 ✅ | 引擎 PoC | 内嵌 OpenFGA server（memory）回环跑通；permission-design §4.2 模型 DSL 加载成功；Check（层级继承/共享）与 ListObjects 冒烟通过；结论落档 |
| M1 ✅ | authz 骨架 | go-zero 骨架生成、go.work 登记、`.api` 契约初稿；bootstrap 幂等跑通（Store/模型/根组织/超管；初始 key 推迟 M4）；`/healthz` `/readyz` 可用 |
| M2 ✅ | 管理 API | 组织/成员/角色/规则表/注册/审计六模块 logic + 单测；审计先行+对账定时任务；越权用例（非管理员写管理面 → 403） |
| M3 ✅ | 判定面 + kit client | Check/ListObjects/me-permissions 上线；kit/authz 单测全覆盖（403/503、fake 注入）；sample 服务挂 Require 冒烟（判定面接入的首个示范） |
| M4 ✅ | auth 服务 | 本地 CAS 登录全链路（跳转→回调→映射→签发含 tenant/roles 的 JWT）；`/v1/me`、续签；401/503 语义正确（CAS 探测修正：明文 HTTP + `/serviceValidate`，见 §6） |
| M5 ✅ | 管理界面 | 六页可用，基座接入（菜单/页签/权限过滤）；增删改操作真实生效并落审计（API 面全链 curl 取证；基座内嵌 UI 受用户并行容器重构影响，浏览器端六页操作留待人工补测——见 Task 12 报告） |
| M6 ✅ | 联调验收 | 验收清单（§5）全过；模型测试四类场景进 CI；文档回填（本 spec 状态、permission-design 实施备注） |

> 打标（2026-09-07）：M0-M3 为 Phase A 交付、M4-M5 为 Phase B 交付、M6 为 Task 12 联调验收收官。后端全链（CAS 登录→管理操作→派生→fail-closed→共享→审计→重启持久化）以 curl 逐项取证通过；两项 UI 侧保留项（基座登录口令端点 404、微应用容器 token 通道随用户重构中）与一项设计缺口（authz 重启后引擎模型回落基础 model.fga，注册类型未重放）如实记录于 Task 12 报告。

顺序依赖：M0 → M1 → M2 → M3 →（M4、M5 可并行）→ M6。M0 是硬前置（引擎嵌入是最大技术风险，失败即回滚到"独立 OpenFGA 容器"备选并重新评审）。与 Phase 0 的衔接：M0 可与其并行；M1 起依赖 F1/F2/F3 产物。

## 5. 测试策略

- **模型测试**（最重点）：OpenFGA 官方测试框架 `.fga` 表驱动用例，四类场景（permission-design §10）：租户隔离零路径、三层组织继承矩阵、共享/越权转授、平台角色 × 规则表
- **authz 单测**：logic 层（repo fake）；管理 API 越权；`/me/permissions` 派生口径（角色 × 规则表 → 服务×动作）
- **kit/authz 单测**：Checker fake 注入、403/503 区分、超时行为
- **auth 单测**：CAS 回调映射、token 签发/续签、角色获取失败 fail-closed（拒发 token）
- **联调验收清单**（M6）：登录 → 管理界面建组织/角色/规则 → 用户 JWT 带角色 → 网关外业务路由无权限 403 / authz 宕机 503 → 共享资源后对方立即可访问 → 审计记录完整

## 6. 风险与前置条件

| 风险/前置 | 影响 | 处理 |
|---|---|---|
| OpenFGA 进程内嵌入稳定性（官方主推独立 server） | M0 验证 | 失败退路：引擎抽独立容器 + 保留回环 client 代码路径（改 base URL 即切） |
| 本地 MySQL（M2 接线） | — | **2026-09-07 已解除**：用户确认可自建库（root，凭据在 .local-dev.md）；两库 nemo_authz / nemo_authz_biz 由迁移任务创建 |
| 本地简易 CAS 地址与协议版本（M4） | 阻塞 M4 联调 | **2026-09-07 已解除，M6 探测修正**：docker 启动（.local-dev.md）；实测容器为**明文 HTTP**（python http.server，无 TLS 包装，https 握手失败）且仅实现 CAS 2.0 `/serviceValidate`（`/cas/p3/serviceValidate` 不存在）——auth etc 已按探测修正（BaseURL `http://localhost:8443` + ValidatePath `/serviceValidate`） |
| kit/template 模板质量（Phase 0 产物） | M1 生成代码可用性 | Phase 0 F2 已定降级路径（默认模板 + 手调）；authz 接入发现缺口回写指南 |
| kit/authz 与 go-zero rest 中间件兼容性 | M3 冒烟 | 面向 net/http 标准接口设计；sample 服务先行验证 |

## 7. 文档影响面

- 本次已改：backend-design.md（kit 表增 `authz/`、端口表登记 authz 8084）
- 前置产物：Phase 0 基础框架（`2026-09-06-backend-foundation-design.md`）——authz 是其指南校准后第一个按流程接入的服务
- M6 回填：permission-design.md 附实施备注（go-zero 已按 backend-design v2.2 落地，无语义变更）；architecture.md 无需再改（authz 已回填）
- 实施计划：由 writing-plans 产出细粒度任务清单（含打标机制），本文档只锁定里程碑边界
